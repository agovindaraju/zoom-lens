// Command demo-app is the Zoom-Lens demo workload. One binary, four modes:
//
//	server       HTTP app with a CPU-bound and a disk-bound endpoint, exposing
//	             Prometheus metrics on /metrics
//	load         constant-rate load generator against the server
//	stress-cpu   noisy neighbour that burns CPU (Scenario A)
//	stress-disk  noisy neighbour that writes and fsyncs to disk (Scenario B)
//
// Shipping every role in one image keeps the demo free of third-party images.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	mode := flag.String("mode", "server", "server | load | stress-cpu | stress-disk")
	addr := flag.String("addr", ":8080", "server: listen address")
	dataDir := flag.String("data-dir", "/data", "server and stress-disk: directory to write to")
	target := flag.String("target", "http://demo-app:8080", "load: base URL of the server")
	rps := flag.Int("rps", 20, "load: requests per second per route")
	workers := flag.Int("workers", 2, "stress-*: parallel workers")
	duration := flag.Duration("duration", 0, "stress-*: stop after this long (0 = until killed)")
	cpuWork := flag.Int("cpu-work", 20000, "server: sha256 rounds per /work/cpu request")
	diskKiB := flag.Int("disk-kib", 256, "server and stress-disk: KiB written (and fsynced) per operation")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if *duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *duration)
		defer cancel()
	}

	var err error
	switch *mode {
	case "server":
		err = serve(ctx, log, *addr, *dataDir, *cpuWork, *diskKiB)
	case "load":
		err = load(ctx, log, *target, *rps)
	case "stress-cpu":
		stressCPU(ctx, log, *workers)
	case "stress-disk":
		err = stressDisk(ctx, log, *dataDir, *workers, *diskKiB)
	default:
		err = fmt.Errorf("unknown mode %q", *mode)
	}
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		log.Error("exiting", "err", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------- server

var (
	reqDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_server_request_duration_seconds",
		Help:    "Duration of HTTP server requests.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"route", "code"})
	orders = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "demo_orders_total", Help: "Demo business metric: orders processed, by status.",
	}, []string{"status"})
	diskBytes = promauto.NewCounter(prometheus.CounterOpts{
		Name: "demo_disk_bytes_written_total", Help: "Bytes written and fsynced by /work/disk.",
	})
	inflight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "demo_inflight_requests", Help: "Requests currently being served.",
	})
)

func serve(ctx context.Context, log *slog.Logger, addr, dataDir string, cpuWork, diskKiB int) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	buf := make([]byte, diskKiB*1024)
	_, _ = rand.Read(buf)

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.Handle("GET /work/cpu", instrument("/work/cpu", func(w http.ResponseWriter, r *http.Request) error {
		n := cpuWork
		if v, err := strconv.Atoi(r.URL.Query().Get("n")); err == nil && v > 0 {
			n = v
		}
		sum := sha256.Sum256([]byte(r.URL.String()))
		for i := 0; i < n; i++ {
			sum = sha256.Sum256(sum[:])
		}
		_, err := fmt.Fprintf(w, "%x\n", sum[:4])
		return err
	}))
	mux.Handle("GET /work/disk", instrument("/work/disk", func(w http.ResponseWriter, _ *http.Request) error {
		if err := writeSync(filepath.Join(dataDir, "app.dat"), buf); err != nil {
			return err
		}
		diskBytes.Add(float64(len(buf)))
		_, err := w.Write([]byte("written\n"))
		return err
	}))

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info("demo app listening", "addr", addr, "dataDir", dataDir)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func instrument(route string, h func(http.ResponseWriter, *http.Request) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inflight.Inc()
		defer inflight.Dec()
		start := time.Now()
		code := http.StatusOK
		if err := h(w, r); err != nil {
			code = http.StatusInternalServerError
			http.Error(w, err.Error(), code)
			orders.WithLabelValues("failed").Inc()
		} else {
			orders.WithLabelValues("ok").Inc()
		}
		reqDuration.WithLabelValues(route, strconv.Itoa(code)).Observe(time.Since(start).Seconds())
	})
}

func writeSync(path string, buf []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// ---------------------------------------------------------------- load

func load(ctx context.Context, log *slog.Logger, target string, rps int) error {
	if rps <= 0 {
		return errors.New("rps must be > 0")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	routes := []string{"/work/cpu", "/work/disk"}
	log.Info("load generator started", "target", target, "rpsPerRoute", rps, "routes", routes)
	var wg sync.WaitGroup
	for _, route := range routes {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			t := time.NewTicker(time.Second / time.Duration(rps))
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					go hit(ctx, client, url)
				}
			}
		}(target + route)
	}
	wg.Wait()
	return ctx.Err()
}

func hit(ctx context.Context, c *http.Client, url string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	resp, err := c.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// ---------------------------------------------------------------- stress

func stressCPU(ctx context.Context, log *slog.Logger, workers int) {
	log.Info("cpu stress started", "workers", workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sum := sha256.Sum256([]byte("zoomlens"))
			for ctx.Err() == nil {
				for j := 0; j < 10000; j++ {
					sum = sha256.Sum256(sum[:])
				}
			}
		}()
	}
	wg.Wait()
	log.Info("cpu stress stopped")
}

func stressDisk(ctx context.Context, log *slog.Logger, dir string, workers, kib int) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	buf := make([]byte, kib*1024)
	_, _ = rand.Read(buf)
	log.Info("disk stress started", "dir", dir, "workers", workers, "kibPerWrite", kib)
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := filepath.Join(dir, fmt.Sprintf("stress-%d.dat", i))
			for ctx.Err() == nil {
				if err := writeSync(path, buf); err != nil {
					errs <- err
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	log.Info("disk stress stopped")
	return <-errs
}
