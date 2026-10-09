package incident

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/zoomlens/zoomlens/internal/alerts"
	"github.com/zoomlens/zoomlens/internal/budget"
	"github.com/zoomlens/zoomlens/internal/ebpf"
	"github.com/zoomlens/zoomlens/internal/knowledge"
	"github.com/zoomlens/zoomlens/internal/metrics"
	"github.com/zoomlens/zoomlens/internal/plan"
	"github.com/zoomlens/zoomlens/internal/render"
)

// ladder: L1 at 0, L2 at 5m (eBPF), clearFor 5m.
type fakePolicy struct{}

func (fakePolicy) LevelFor(d time.Duration) knowledge.Level {
	if d >= 5*time.Minute {
		return knowledge.Level{Index: 2, Depth: 2, Hops: 1, ScrapeInterval: 5 * time.Second, EnableEBPF: true}
	}
	return knowledge.Level{Index: 1, Depth: 1, Hops: 1, ScrapeInterval: 10 * time.Second}
}
func (fakePolicy) ClearFor() time.Duration { return 5 * time.Minute }

type fakePlanner struct{ calls []int }

func (f *fakePlanner) Plan(_ context.Context, r plan.Request) (plan.Scope, error) {
	f.calls = append(f.calls, r.Level.Index)
	o := plan.NodeOverlay{Node: "n1", Interval: r.Level.ScrapeInterval, Incidents: []string{r.IncidentID},
		Metrics: map[knowledge.Source][]string{knowledge.SourceCAdvisor: {"container_memory_rss"}}}
	if r.Level.EnableEBPF {
		o.EBPFPrograms = []string{"biolatency"}
	}
	return plan.Scope{"n1": o}, nil
}

type fakeBudget struct {
	allowed bool
	err     error
}

func (f fakeBudget) Check(context.Context) (budget.Result, error) {
	return budget.Result{Allowed: f.allowed}, f.err
}

type fakeConfig struct{ last map[string][]byte }

func (f *fakeConfig) SetDesired(b map[string][]byte) { f.last = b }

type fakeEBPF struct{ last map[string][]string }

func (f *fakeEBPF) Sync(_ context.Context, d map[string][]string) ([]ebpf.PodInfo, error) {
	f.last = d
	return nil, nil
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func setup(b BudgetChecker) (*Manager, *clock, *fakePlanner, *fakeConfig, *fakeEBPF) {
	c := &clock{t: time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)}
	p, cfg, e := &fakePlanner{}, &fakeConfig{}, &fakeEBPF{}
	m := NewManager(slog.New(slog.NewTextHandler(io.Discard, nil)), metrics.New(), fakePolicy{}, p, b, cfg, e,
		Options{Render: render.DefaultOptions()}, c.now)
	return m, c, p, cfg, e
}

func firing(start time.Time) alerts.Signal {
	return alerts.Signal{Fingerprint: "fp0123456789", AlertName: "A", Firing: true, StartsAt: start, Root: "r",
		Entity: knowledge.Entity{Kind: knowledge.KindPod, Name: "app", Namespace: "demo"}}
}

func TestLadderEscalatesCoolsDownAndReverts(t *testing.T) {
	m, c, p, cfg, e := setup(fakeBudget{allowed: true})
	ctx := context.Background()

	m.Observe([]alerts.Signal{firing(c.t)})
	m.Reconcile(ctx)
	if len(p.calls) != 1 || p.calls[0] != 1 {
		t.Fatalf("want L1 planned, got %v", p.calls)
	}
	if !strings.Contains(string(cfg.last["n1"]), "zl-cadvisor") || len(e.last) != 0 {
		t.Fatalf("L1 should push an overlay without eBPF: cfg=%q ebpf=%v", cfg.last["n1"], e.last)
	}

	c.t = c.t.Add(6 * time.Minute)
	m.Reconcile(ctx)
	if len(p.calls) != 2 || p.calls[1] != 2 || len(e.last["n1"]) != 1 {
		t.Fatalf("want L2 with eBPF after 6m, calls=%v ebpf=%v", p.calls, e.last)
	}

	resolved := firing(time.Time{})
	resolved.Firing = false
	m.Observe([]alerts.Signal{resolved})
	c.t = c.t.Add(2 * time.Minute)
	m.Reconcile(ctx)
	if snap := m.Snapshot(); len(snap.Incidents) != 1 || snap.Incidents[0].State() != "cooldown" || len(e.last["n1"]) != 1 {
		t.Fatalf("cool-down must keep the level and eBPF: %+v", snap.Incidents)
	}

	c.t = c.t.Add(4 * time.Minute) // 6m after resolve > clearFor
	m.Reconcile(ctx)
	if snap := m.Snapshot(); len(snap.Incidents) != 0 {
		t.Fatalf("incident should be closed, got %+v", snap.Incidents)
	}
	if len(cfg.last) != 0 || len(e.last) != 0 {
		t.Fatalf("revert must clear overlays and eBPF: cfg=%v ebpf=%v", cfg.last, e.last)
	}
}

func TestRefireDuringCooldownResumes(t *testing.T) {
	m, c, p, _, _ := setup(fakeBudget{allowed: true})
	ctx := context.Background()
	m.Observe([]alerts.Signal{firing(c.t)})
	m.Reconcile(ctx)
	res := firing(time.Time{})
	res.Firing = false
	m.Observe([]alerts.Signal{res})
	c.t = c.t.Add(time.Minute)
	m.Observe([]alerts.Signal{firing(c.t)})
	m.Reconcile(ctx)
	snap := m.Snapshot()
	if len(snap.Incidents) != 1 || snap.Incidents[0].State() != "firing" || snap.Incidents[0].Level.Index != 1 || len(p.calls) != 1 {
		t.Fatalf("re-fire should resume at the same level without re-planning: %+v calls=%v", snap.Incidents, p.calls)
	}
}

func TestBudgetBlocksEscalation(t *testing.T) {
	for _, b := range []fakeBudget{{allowed: false}, {err: errors.New("down")}} {
		m, c, p, cfg, _ := setup(b)
		m.Observe([]alerts.Signal{firing(c.t)})
		m.Reconcile(context.Background())
		snap := m.Snapshot()
		if len(p.calls) != 0 || snap.Incidents[0].Level.Index != 0 || snap.Incidents[0].LastError == "" || len(cfg.last) != 0 {
			t.Fatalf("budget %+v should hold the incident at L0: %+v", b, snap.Incidents)
		}
	}
}

func TestStartupGraceDefersRevert(t *testing.T) {
	c := &clock{t: time.Now()}
	cfg := &fakeConfig{last: map[string][]byte{"sentinel": nil}}
	m := NewManager(slog.New(slog.NewTextHandler(io.Discard, nil)), metrics.New(), fakePolicy{}, &fakePlanner{},
		fakeBudget{allowed: true}, cfg, &fakeEBPF{}, Options{StartupGrace: time.Minute, Render: render.DefaultOptions()}, c.now)
	m.Reconcile(context.Background())
	if _, ok := cfg.last["sentinel"]; !ok {
		t.Fatal("nothing may be applied during the startup grace period")
	}
	c.t = c.t.Add(2 * time.Minute)
	m.Reconcile(context.Background())
	if _, ok := cfg.last["sentinel"]; ok {
		t.Fatal("after the grace period the (empty) desired state must be applied")
	}
}
