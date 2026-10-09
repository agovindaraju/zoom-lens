// Package config loads operator settings from flags, with environment
// variables (ZOOMLENS_<FLAG_NAME>) as defaults.
package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all operator settings.
type Config struct {
	HTTPAddr  string
	OpAMPAddr string
	// LadderFile holds the dwell ladder (escalation timings).
	LadderFile string
	// KnowledgeRefresh is how often the zoomlens.dev knowledge resources are re-listed.
	KnowledgeRefresh time.Duration
	Kubeconfig       string

	PrometheusURL string
	MaxHeadSeries float64

	ReconcileInterval time.Duration
	StartupGrace      time.Duration

	EBPFNamespace string
	EBPFImage     string
	EBPFPort      int
	EBPFDeadline  time.Duration
	EBPFConfigDir string

	CollectorExporter       string
	CollectorProcessors     []string
	CollectorHostProcessors []string
	CollectorHostRootPath   string

	LogLevel  string
	LogFormat string
}

// Load parses args (without the program name).
func Load(args []string) (*Config, error) {
	fs := flag.NewFlagSet("zoomlens", flag.ContinueOnError)
	c := &Config{}
	var processors, hostProcessors string

	fs.StringVar(&c.HTTPAddr, "http-addr", env("HTTP_ADDR", ":8080"), "listen address for the Alertmanager webhook, /metrics, /healthz and /debug/state")
	fs.StringVar(&c.OpAMPAddr, "opamp-addr", env("OPAMP_ADDR", ":4320"), "listen address of the OpAMP server (path /v1/opamp)")
	fs.StringVar(&c.LadderFile, "ladder-file", env("LADDER_FILE", "/etc/zoomlens/ladder.yaml"), "dwell ladder file (escalation timings)")
	fs.DurationVar(&c.KnowledgeRefresh, "knowledge-refresh", envDuration("KNOWLEDGE_REFRESH", time.Minute), "how often the zoomlens.dev knowledge resources (MetricTreePack, MetricRelations, EntityGraph) are re-listed")
	fs.StringVar(&c.Kubeconfig, "kubeconfig", env("KUBECONFIG", ""), "kubeconfig path; empty means in-cluster config")
	fs.StringVar(&c.PrometheusURL, "prometheus-url", env("PROMETHEUS_URL", "http://prometheus-operated.monitoring.svc:9090"), "Prometheus base URL for the series budget")
	fs.Float64Var(&c.MaxHeadSeries, "max-head-series", envFloat("MAX_HEAD_SERIES", 200000), "refuse escalations above this many Prometheus head series (0 disables)")
	fs.DurationVar(&c.ReconcileInterval, "reconcile-interval", envDuration("RECONCILE_INTERVAL", 15*time.Second), "how often the dwell ladder is evaluated")
	fs.DurationVar(&c.StartupGrace, "startup-grace", envDuration("STARTUP_GRACE", 90*time.Second), "after start, wait this long for Alertmanager to re-send firing alerts before reverting anything")
	fs.StringVar(&c.EBPFNamespace, "ebpf-namespace", env("EBPF_NAMESPACE", "zoomlens"), "namespace for ebpf_exporter pods")
	fs.StringVar(&c.EBPFImage, "ebpf-image", env("EBPF_IMAGE", "ghcr.io/cloudflare/ebpf_exporter:v2.5.1"), "ebpf_exporter image (must contain /examples)")
	fs.IntVar(&c.EBPFPort, "ebpf-port", envInt("EBPF_PORT", 9435), "ebpf_exporter listen port")
	fs.DurationVar(&c.EBPFDeadline, "ebpf-deadline", envDuration("EBPF_DEADLINE", 30*time.Minute), "activeDeadlineSeconds safety net for ebpf_exporter pods")
	fs.StringVar(&c.EBPFConfigDir, "ebpf-config-dir", env("EBPF_CONFIG_DIR", "/examples"), "config directory inside the ebpf_exporter image")
	fs.StringVar(&c.CollectorExporter, "collector-exporter", env("COLLECTOR_EXPORTER", "prometheusremotewrite"), "baseline exporter name the overlay pipeline uses")
	fs.StringVar(&processors, "collector-processors", env("COLLECTOR_PROCESSORS", "memory_limiter,batch"), "comma-separated baseline processors the overlay scrape pipeline uses")
	fs.StringVar(&hostProcessors, "collector-host-processors", env("COLLECTOR_HOST_PROCESSORS", "memory_limiter,resource/host,batch"), "comma-separated baseline processors the overlay hostmetrics pipeline uses")
	fs.StringVar(&c.CollectorHostRootPath, "collector-host-root-path", env("COLLECTOR_HOST_ROOT_PATH", "/hostfs"), "hostmetrics root_path used by the baseline (empty when the collector is not containerised)")
	fs.StringVar(&c.LogLevel, "log-level", env("LOG_LEVEL", "info"), "debug, info, warn or error")
	fs.StringVar(&c.LogFormat, "log-format", env("LOG_FORMAT", "json"), "json or text")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	c.CollectorProcessors = splitList(processors)
	c.CollectorHostProcessors = splitList(hostProcessors)
	return c, c.validate()
}

func (c *Config) validate() error {
	var errs []error
	if c.ReconcileInterval < time.Second {
		errs = append(errs, errors.New("reconcile-interval must be >= 1s"))
	}
	if c.EBPFDeadline < time.Minute {
		errs = append(errs, errors.New("ebpf-deadline must be >= 1m"))
	}
	if c.EBPFPort <= 0 || c.EBPFPort > 65535 {
		errs = append(errs, fmt.Errorf("ebpf-port %d is invalid", c.EBPFPort))
	}
	if c.CollectorExporter == "" {
		errs = append(errs, errors.New("collector-exporter must be set"))
	}
	if c.KnowledgeRefresh < time.Second {
		errs = append(errs, errors.New("knowledge-refresh must be >= 1s"))
	}
	switch c.LogFormat {
	case "json", "text":
	default:
		errs = append(errs, fmt.Errorf("log-format %q must be json or text", c.LogFormat))
	}
	return errors.Join(errs...)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func env(key, def string) string {
	if v, ok := os.LookupEnv("ZOOMLENS_" + key); ok {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(env(key, "")); err == nil {
		return d
	}
	return def
}

func envInt(key string, def int) int {
	if i, err := strconv.Atoi(env(key, "")); err == nil {
		return i
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if f, err := strconv.ParseFloat(env(key, ""), 64); err == nil {
		return f
	}
	return def
}
