// Package httpserver exposes the Alertmanager webhook, health, metrics and
// debug endpoints.
package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/zoomlens/zoomlens/internal/alerts"
	"github.com/zoomlens/zoomlens/internal/incident"
	"github.com/zoomlens/zoomlens/internal/metrics"
	"github.com/zoomlens/zoomlens/internal/opamp"
)

// Observer consumes alert signals.
type Observer interface {
	Observe([]alerts.Signal)
	Snapshot() incident.Snapshot
}

// AgentLister lists OpAMP agents.
type AgentLister interface {
	Agents() []opamp.AgentInfo
}

// DebugState is returned by GET /debug/state.
type DebugState struct {
	incident.Snapshot
	Agents []opamp.AgentInfo `json:"agents"`
}

// Server wires the HTTP routes.
type Server struct {
	log    *slog.Logger
	m      *metrics.Metrics
	obs    Observer
	agents AgentLister
	ready  atomic.Bool
}

// New returns a Server.
func New(log *slog.Logger, m *metrics.Metrics, obs Observer, agents AgentLister) *Server {
	return &Server{log: log, m: m, obs: obs, agents: agents}
}

// SetReady flips /readyz.
func (s *Server) SetReady(v bool) { s.ready.Store(v) }

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /alerts", s.handleAlerts)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("GET /debug/state", s.handleState)
	mux.Handle("GET /metrics", promhttp.HandlerFor(s.m.Registry, promhttp.HandlerOpts{}))
	return mux
}

// NewHTTPServer returns an http.Server with sane timeouts.
func (s *Server) NewHTTPServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	payload, err := alerts.Decode(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	signals, skipped := payload.Signals()
	s.m.AlertsSkipped.Add(float64(skipped))
	s.log.Debug("alertmanager webhook", "status", payload.Status, "alerts", len(payload.Alerts), "zoomlens", len(signals), "skipped", skipped)
	s.obs.Observe(signals)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	if !s.ready.Load() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ready\n"))
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(DebugState{Snapshot: s.obs.Snapshot(), Agents: s.agents.Agents()})
}
