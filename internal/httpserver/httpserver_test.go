package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zoomlens/zoomlens/internal/alerts"
	"github.com/zoomlens/zoomlens/internal/incident"
	"github.com/zoomlens/zoomlens/internal/metrics"
	"github.com/zoomlens/zoomlens/internal/opamp"
)

type fakeObs struct{ got []alerts.Signal }

func (f *fakeObs) Observe(s []alerts.Signal)   { f.got = append(f.got, s...) }
func (f *fakeObs) Snapshot() incident.Snapshot { return incident.Snapshot{} }

type fakeAgents struct{}

func (fakeAgents) Agents() []opamp.AgentInfo { return []opamp.AgentInfo{{Node: "n1"}} }

func TestRoutes(t *testing.T) {
	obs := &fakeObs{}
	s := New(slog.New(slog.NewTextHandler(io.Discard, nil)), metrics.New(), obs, fakeAgents{})
	h := s.Handler()

	body := `{"alerts":[{"status":"firing","fingerprint":"f1","labels":{"zoomlens_root":"r","node":"n1"}}]}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/alerts", strings.NewReader(body)))
	if rec.Code != http.StatusAccepted || len(obs.got) != 1 {
		t.Fatalf("POST /alerts: code=%d signals=%d", rec.Code, len(obs.got))
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/alerts", strings.NewReader("nope")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad payload: code=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz before ready: %d", rec.Code)
	}
	s.SetReady(true)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("readyz after ready: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/state", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"node": "n1"`) {
		t.Fatalf("debug/state: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "zoomlens_alerts_skipped_total") {
		t.Fatalf("metrics: %d", rec.Code)
	}
}
