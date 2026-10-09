// Command zoomlens is the Zoom-Lens operator: it receives Alertmanager alerts,
// walks the dwell ladder, and expands/reverts metric collection by pushing
// OpAMP remote configs to collectors and starting eBPF exporters per node.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/zoomlens/zoomlens/internal/budget"
	"github.com/zoomlens/zoomlens/internal/config"
	"github.com/zoomlens/zoomlens/internal/ebpf"
	"github.com/zoomlens/zoomlens/internal/httpserver"
	"github.com/zoomlens/zoomlens/internal/incident"
	"github.com/zoomlens/zoomlens/internal/knowledge"
	"github.com/zoomlens/zoomlens/internal/knowledge/crd"
	"github.com/zoomlens/zoomlens/internal/knowledge/policy"
	"github.com/zoomlens/zoomlens/internal/knowledge/topology"
	"github.com/zoomlens/zoomlens/internal/metrics"
	"github.com/zoomlens/zoomlens/internal/opamp"
	"github.com/zoomlens/zoomlens/internal/plan"
	"github.com/zoomlens/zoomlens/internal/render"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "zoomlens:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return err
	}
	log := newLogger(cfg)
	log.Info("starting zoomlens", "version", version)

	rc, err := restConfig(cfg.Kubeconfig)
	if err != nil {
		return err
	}
	client, err := kubernetes.NewForConfig(rc)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Knowledge layer. The metric trees, relations and entity graph come from the zoomlens.dev
	// resources (knowledge/); the dwell ladder comes from the ladder file; the Pod -> Node ->
	// co-located Pods edges come from the Kubernetes API. See docs/KNOWLEDGE-INTERFACES.md.
	pol, err := policy.Load(cfg.LadderFile)
	if err != nil {
		return err
	}
	dc, err := dynamic.NewForConfig(rc)
	if err != nil {
		return err
	}
	loader := crd.APILoader{Client: dc}
	sn, err := loader.Load(ctx)
	if err != nil {
		return fmt.Errorf("load knowledge resources (are the zoomlens.dev CRDs and knowledge/ applied?): %w", err)
	}
	store := &crd.Store{}
	store.Set(sn)
	log.Info("knowledge loaded", "stats", sn.Stats, "ladder", cfg.LadderFile)
	go crd.Run(ctx, loader, store, cfg.KnowledgeRefresh, log.With("component", "knowledge"))
	k := knowledge.Knowledge{
		Tree:   crd.Tree{Store: store},
		Graph:  crd.Graph{Store: store, Inner: topology.NewGraph(client)},
		Policy: pol,
	}

	m := metrics.New()
	ropt := render.DefaultOptions()
	ropt.Exporter, ropt.Processors, ropt.HostProcessors = cfg.CollectorExporter, cfg.CollectorProcessors, cfg.CollectorHostProcessors
	ropt.EBPFNamespace, ropt.EBPFPort = cfg.EBPFNamespace, cfg.EBPFPort
	ropt.HostRootPath = cfg.CollectorHostRootPath

	opampSrv := opamp.New(log.With("component", "opamp"), m, []byte(render.NoopBody))
	actuator := ebpf.New(client, ebpf.Options{
		Namespace: cfg.EBPFNamespace, Image: cfg.EBPFImage, Port: int32(cfg.EBPFPort),
		DeadlineSeconds: int64(cfg.EBPFDeadline.Seconds()), ConfigDir: cfg.EBPFConfigDir,
	}, log.With("component", "ebpf"))
	mgr := incident.NewManager(log.With("component", "incident"), m, k.Policy, plan.NewPlanner(k),
		budget.New(cfg.PrometheusURL, cfg.MaxHeadSeries), opampSrv, actuator,
		incident.Options{StartupGrace: cfg.StartupGrace, Render: ropt}, nil)

	if err := opampSrv.Start(cfg.OpAMPAddr); err != nil {
		return fmt.Errorf("start opamp server: %w", err)
	}
	log.Info("opamp server listening", "addr", cfg.OpAMPAddr, "path", "/v1/opamp")

	hs := httpserver.New(log.With("component", "http"), m, mgr, opampSrv)
	srv := hs.NewHTTPServer(cfg.HTTPAddr)
	srvErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErr <- err
		}
	}()

	go mgr.Run(ctx, cfg.ReconcileInterval)
	hs.SetReady(true)

	select {
	case <-ctx.Done():
	case err := <-srvErr:
		return fmt.Errorf("http server: %w", err)
	}

	log.Info("shutting down")
	hs.SetReady(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Collection overlays and eBPF pods are left in place on shutdown: the
	// next operator instance adopts them after the startup grace period, and
	// eBPF pods have their own deadline.
	return errors.Join(srv.Shutdown(shutdownCtx), opampSrv.Stop(shutdownCtx))
}

func newLogger(cfg *config.Config) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	if cfg.LogFormat == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

func restConfig(kubeconfig string) (*rest.Config, error) {
	var (
		rc  *rest.Config
		err error
	)
	if kubeconfig != "" {
		rc, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		rc, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("kubernetes client config: %w", err)
	}
	// The operator makes a handful of small API calls; JSON keeps them
	// readable in API server audit logs and debugging proxies.
	rc.ContentType = "application/json"
	rc.UserAgent = "zoomlens/" + version
	return rc, nil
}
