// Package incident owns the dwell-ladder state machine: it tracks incidents
// from Alertmanager signals, decides when to escalate, and reconciles the
// merged desired collection state into the actuators.
package incident

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/zoomlens/zoomlens/internal/alerts"
	"github.com/zoomlens/zoomlens/internal/budget"
	"github.com/zoomlens/zoomlens/internal/ebpf"
	"github.com/zoomlens/zoomlens/internal/knowledge"
	"github.com/zoomlens/zoomlens/internal/metrics"
	"github.com/zoomlens/zoomlens/internal/plan"
	"github.com/zoomlens/zoomlens/internal/render"
)

// Planner computes the scope of one incident at one level.
type Planner interface {
	Plan(ctx context.Context, r plan.Request) (plan.Scope, error)
}

// BudgetChecker guards escalations.
type BudgetChecker interface {
	Check(ctx context.Context) (budget.Result, error)
}

// ConfigActuator pushes per-node collector config overlays (OpAMP).
type ConfigActuator interface {
	SetDesired(bodies map[string][]byte)
}

// EBPFActuator reconciles eBPF exporter pods.
type EBPFActuator interface {
	Sync(ctx context.Context, desired map[string][]string) ([]ebpf.PodInfo, error)
}

// Incident is one alert being tracked through the ladder.
type Incident struct {
	ID          string           `json:"id"`
	Fingerprint string           `json:"fingerprint"`
	AlertName   string           `json:"alertName"`
	Root        string           `json:"root"`
	Entity      knowledge.Entity `json:"entity"`
	StartsAt    time.Time        `json:"startsAt"`
	ResolvedAt  *time.Time       `json:"resolvedAt,omitempty"`
	Level       knowledge.Level  `json:"level"`
	Scope       plan.Scope       `json:"scope,omitempty"`
	LastError   string           `json:"lastError,omitempty"`
}

// State returns "firing" or "cooldown".
func (i *Incident) State() string {
	if i.ResolvedAt != nil {
		return "cooldown"
	}
	return "firing"
}

// Snapshot is the manager's state for the debug endpoint.
type Snapshot struct {
	Time       time.Time         `json:"time"`
	Incidents  []Incident        `json:"incidents"`
	Overlays   plan.Scope        `json:"overlays"`
	EBPFPods   []ebpf.PodInfo    `json:"ebpfPods"`
	LastApply  time.Time         `json:"lastApply,omitempty"`
	LastErrors map[string]string `json:"lastErrors,omitempty"`
}

// Options configure the manager.
type Options struct {
	// StartupGrace delays the first revert after a restart so incidents that
	// Alertmanager will re-send are not reverted and re-expanded.
	StartupGrace time.Duration
	Render       render.Options
}

// Manager is safe for concurrent use.
type Manager struct {
	log     *slog.Logger
	m       *metrics.Metrics
	policy  knowledge.EscalationPolicy
	planner Planner
	budget  BudgetChecker
	config  ConfigActuator
	ebpf    EBPFActuator
	opt     Options
	now     func() time.Time
	started time.Time
	kick    chan struct{}

	mu        sync.Mutex
	incidents map[string]*Incident
	overlays  plan.Scope
	pods      []ebpf.PodInfo
	lastApply time.Time
	errs      map[string]string
}

// NewManager wires a Manager. now may be nil (time.Now).
func NewManager(log *slog.Logger, m *metrics.Metrics, policy knowledge.EscalationPolicy, planner Planner,
	b BudgetChecker, cfg ConfigActuator, e EBPFActuator, opt Options, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	return &Manager{
		log: log, m: m, policy: policy, planner: planner, budget: b, config: cfg, ebpf: e, opt: opt,
		now: now, started: now(), kick: make(chan struct{}, 1),
		incidents: map[string]*Incident{}, overlays: plan.Scope{}, errs: map[string]string{},
	}
}

// Observe records alert signals and triggers a reconcile.
func (mg *Manager) Observe(signals []alerts.Signal) {
	mg.mu.Lock()
	now := mg.now()
	for _, s := range signals {
		inc, ok := mg.incidents[s.Fingerprint]
		switch {
		case s.Firing && !ok:
			start := s.StartsAt
			if start.IsZero() || start.After(now) {
				start = now
			}
			inc = &Incident{
				ID: shortID(s.Fingerprint), Fingerprint: s.Fingerprint, AlertName: s.AlertName,
				Root: s.Root, Entity: s.Entity, StartsAt: start,
			}
			mg.incidents[s.Fingerprint] = inc
			mg.log.Info("incident opened", "incident", inc.ID, "alert", s.AlertName, "root", s.Root,
				"entity", fmt.Sprintf("%s/%s/%s", s.Entity.Kind, s.Entity.Namespace, s.Entity.Name), "startsAt", start)
		case s.Firing && ok && inc.ResolvedAt != nil:
			inc.ResolvedAt = nil
			mg.log.Info("incident re-fired during cool-down, keeping its level", "incident", inc.ID, "level", inc.Level.Index)
		case !s.Firing && ok && inc.ResolvedAt == nil:
			t := now
			inc.ResolvedAt = &t
			mg.log.Info("incident resolved, cool-down started", "incident", inc.ID, "clearFor", mg.policy.ClearFor())
		}
		if s.Firing {
			mg.m.AlertsReceived.WithLabelValues(alerts.StatusFiring).Inc()
		} else {
			mg.m.AlertsReceived.WithLabelValues(alerts.StatusResolved).Inc()
		}
	}
	mg.mu.Unlock()
	select {
	case mg.kick <- struct{}{}:
	default:
	}
}

// Run reconciles every interval and whenever Observe is called.
func (mg *Manager) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		mg.Reconcile(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-mg.kick:
		}
	}
}

// Reconcile advances the ladder and applies the merged desired state.
func (mg *Manager) Reconcile(ctx context.Context) {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	now := mg.now()

	for fp, inc := range mg.incidents {
		if inc.ResolvedAt != nil {
			if now.Sub(*inc.ResolvedAt) >= mg.policy.ClearFor() {
				mg.log.Info("incident closed, reverting its collection", "incident", inc.ID, "level", inc.Level.Index)
				delete(mg.incidents, fp)
			}
			continue
		}
		mg.advance(ctx, inc, now)
	}

	if len(mg.incidents) == 0 && now.Sub(mg.started) < mg.opt.StartupGrace {
		// Just restarted and nothing re-reported yet: do not revert anything
		// collectors may still be running for ongoing incidents.
		mg.updateGauges()
		return
	}
	mg.apply(ctx, now)
	mg.updateGauges()
}

func (mg *Manager) advance(ctx context.Context, inc *Incident, now time.Time) {
	target := mg.policy.LevelFor(now.Sub(inc.StartsAt))
	if target.Index <= inc.Level.Index {
		return
	}
	res, err := mg.budget.Check(ctx)
	if err != nil || !res.Allowed {
		msg := fmt.Sprintf("escalation to L%d held by series budget (series=%.0f limit=%.0f)", target.Index, res.Series, res.Limit)
		if err != nil {
			msg = fmt.Sprintf("escalation to L%d held: budget check failed: %v", target.Index, err)
		}
		if inc.LastError != msg {
			mg.log.Warn(msg, "incident", inc.ID)
		}
		inc.LastError = msg
		mg.m.BudgetBlocked.Inc()
		return
	}
	scope, err := mg.planner.Plan(ctx, plan.Request{IncidentID: inc.ID, Root: inc.Root, Entity: inc.Entity, Level: target})
	if err != nil {
		inc.LastError = err.Error()
		mg.m.ReconcileErrors.WithLabelValues("plan").Inc()
		mg.log.Error("planning failed", "incident", inc.ID, "level", target.Index, "err", err)
		return
	}
	mg.log.Info("incident escalated", "incident", inc.ID, "from", inc.Level.Index, "to", target.Index,
		"depth", target.Depth, "hops", target.Hops, "interval", target.ScrapeInterval, "ebpf", target.EnableEBPF,
		"nodes", scope.Nodes())
	inc.Level, inc.Scope, inc.LastError = target, scope, ""
	mg.m.Escalations.WithLabelValues(fmt.Sprintf("L%d", target.Index)).Inc()
}

func (mg *Manager) apply(ctx context.Context, now time.Time) {
	scopes := make([]plan.Scope, 0, len(mg.incidents))
	for _, inc := range mg.incidents {
		scopes = append(scopes, inc.Scope)
	}
	merged := plan.Merge(scopes...)

	bodies := make(map[string][]byte, len(merged))
	programs := make(map[string][]string, len(merged))
	delete(mg.errs, "render")
	for node, o := range merged {
		b, err := render.Render(o, mg.opt.Render)
		if err != nil {
			mg.errs["render"] = err.Error()
			mg.m.ReconcileErrors.WithLabelValues("render").Inc()
			mg.log.Error("render failed, node keeps its baseline", "node", node, "err", err)
			continue
		}
		bodies[node] = b
		if len(o.EBPFPrograms) > 0 {
			programs[node] = o.EBPFPrograms
		}
	}
	mg.config.SetDesired(bodies)

	pods, err := mg.ebpf.Sync(ctx, programs)
	if err != nil {
		mg.errs["ebpf"] = err.Error()
		mg.m.ReconcileErrors.WithLabelValues("ebpf").Inc()
		mg.log.Error("ebpf sync failed", "err", err)
	} else {
		delete(mg.errs, "ebpf")
	}
	mg.overlays, mg.pods, mg.lastApply = merged, pods, now
}

func (mg *Manager) updateGauges() {
	counts := map[string]int{"firing": 0, "cooldown": 0}
	mg.m.IncidentLevel.Reset()
	for _, inc := range mg.incidents {
		counts[inc.State()]++
		mg.m.IncidentLevel.WithLabelValues(inc.ID, inc.AlertName, inc.Root,
			fmt.Sprintf("%s/%s", inc.Entity.Kind, inc.Entity.Name)).Set(float64(inc.Level.Index))
	}
	for state, n := range counts {
		mg.m.Incidents.WithLabelValues(state).Set(float64(n))
	}
	nonEmpty := 0
	for _, o := range mg.overlays {
		if !o.Empty() {
			nonEmpty++
		}
	}
	mg.m.OverlayNodes.Set(float64(nonEmpty))
	mg.m.EBPFPods.Set(float64(len(mg.pods)))
}

// Snapshot returns a copy of the current state.
func (mg *Manager) Snapshot() Snapshot {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	s := Snapshot{Time: mg.now(), Overlays: mg.overlays, EBPFPods: mg.pods, LastApply: mg.lastApply, LastErrors: map[string]string{}}
	for k, v := range mg.errs {
		s.LastErrors[k] = v
	}
	for _, inc := range mg.incidents {
		s.Incidents = append(s.Incidents, *inc)
	}
	sort.Slice(s.Incidents, func(i, j int) bool { return s.Incidents[i].StartsAt.Before(s.Incidents[j].StartsAt) })
	return s
}

func shortID(fp string) string {
	if len(fp) > 8 {
		return fp[:8]
	}
	return fp
}
