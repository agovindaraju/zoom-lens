// Package plan turns "incident X is at level L" into concrete per-node
// collection overlays, and merges the overlays of all active incidents.
package plan

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/zoomlens/zoomlens/internal/knowledge"
)

// PodRef identifies a pod whose container-level metrics should be collected.
type PodRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// NodeOverlay is everything Zoom-Lens adds on top of the baseline on one node.
type NodeOverlay struct {
	Node string `json:"node"`
	// Interval is the scrape interval for everything in this overlay.
	Interval time.Duration `json:"interval"`
	// Metrics maps each scrape-based source to the metric names to keep.
	Metrics map[knowledge.Source][]string `json:"metrics,omitempty"`
	// Pods restricts pod-scoped sources (cadvisor, app) to these pods.
	Pods []PodRef `json:"pods,omitempty"`
	// HostScrapers are extra OpenTelemetry hostmetrics scrapers to enable.
	HostScrapers []string `json:"hostScrapers,omitempty"`
	// EBPFPrograms are ebpf_exporter config names to run on this node.
	EBPFPrograms []string `json:"ebpfPrograms,omitempty"`
	// Incidents lists the incident IDs that contributed to this overlay.
	Incidents []string `json:"incidents"`
}

// Empty reports whether the overlay adds nothing.
func (o NodeOverlay) Empty() bool {
	for _, m := range o.Metrics {
		if len(m) > 0 {
			return false
		}
	}
	return len(o.HostScrapers) == 0 && len(o.EBPFPrograms) == 0
}

// Scope is the per-node result of planning one incident at one level.
type Scope map[string]NodeOverlay

// Nodes returns the node names in the scope, sorted.
func (s Scope) Nodes() []string {
	out := make([]string, 0, len(s))
	for n := range s {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Request is the input for planning one incident.
type Request struct {
	IncidentID string
	Root       string
	Entity     knowledge.Entity
	Level      knowledge.Level
}

// Planner computes scopes using the Knowledge layer.
type Planner struct {
	k knowledge.Knowledge
}

// NewPlanner returns a Planner.
func NewPlanner(k knowledge.Knowledge) *Planner { return &Planner{k: k} }

// Plan walks the metric tree DOWN and the dependency graph OUT.
func (p *Planner) Plan(ctx context.Context, r Request) (Scope, error) {
	if r.Level.Index == 0 {
		return Scope{}, nil
	}
	refs, err := p.k.Tree.Expand(r.Root, r.Entity.Kind, r.Level.Depth)
	if err != nil {
		return nil, fmt.Errorf("expand metric tree for %s: %w", r.Root, err)
	}
	var related []knowledge.Entity
	if g, ok := p.k.Graph.(knowledge.RootAwareGraph); ok {
		related, err = g.RelatedForRoot(ctx, r.Root, r.Entity, r.Level.Hops)
	} else {
		related, err = p.k.Graph.Related(ctx, r.Entity, r.Level.Hops)
	}
	if err != nil {
		return nil, fmt.Errorf("walk dependency graph from %s/%s: %w", r.Entity.Kind, r.Entity.Name, err)
	}

	scope := Scope{}
	for _, e := range related {
		if e.Node == "" {
			continue
		}
		o, ok := scope[e.Node]
		if !ok {
			o = newOverlay(e.Node, r.Level.ScrapeInterval, r.IncidentID, refs, r.Level.EnableEBPF)
		}
		if e.Kind == knowledge.KindPod {
			o.Pods = append(o.Pods, PodRef{Namespace: e.Namespace, Name: e.Name})
		}
		scope[e.Node] = o
	}
	for n, o := range scope {
		o.Pods = dedupePods(o.Pods)
		if o.Empty() {
			delete(scope, n)
			continue
		}
		scope[n] = o
	}
	return scope, nil
}

func newOverlay(node string, interval time.Duration, id string, refs []knowledge.MetricRef, ebpf bool) NodeOverlay {
	o := NodeOverlay{Node: node, Interval: interval, Metrics: map[knowledge.Source][]string{}, Incidents: []string{id}}
	for _, m := range refs {
		switch m.Source {
		case knowledge.SourceHostMetrics:
			o.HostScrapers = append(o.HostScrapers, m.Name)
		case knowledge.SourceEBPF:
			if ebpf {
				o.EBPFPrograms = append(o.EBPFPrograms, m.Name)
			}
		default:
			o.Metrics[m.Source] = append(o.Metrics[m.Source], m.Name)
		}
	}
	for s, names := range o.Metrics {
		o.Metrics[s] = dedupeStrings(names)
	}
	o.HostScrapers = dedupeStrings(o.HostScrapers)
	o.EBPFPrograms = dedupeStrings(o.EBPFPrograms)
	return o
}

// Merge combines the scopes of all active incidents into one overlay per node.
// Metrics, pods, scrapers and programs are unioned; the shortest interval wins.
func Merge(scopes ...Scope) Scope {
	out := Scope{}
	for _, s := range scopes {
		for node, o := range s {
			cur, ok := out[node]
			if !ok {
				cur = NodeOverlay{Node: node, Interval: o.Interval, Metrics: map[knowledge.Source][]string{}}
			}
			if o.Interval > 0 && (cur.Interval == 0 || o.Interval < cur.Interval) {
				cur.Interval = o.Interval
			}
			for src, names := range o.Metrics {
				cur.Metrics[src] = append(cur.Metrics[src], names...)
			}
			cur.Pods = append(cur.Pods, o.Pods...)
			cur.HostScrapers = append(cur.HostScrapers, o.HostScrapers...)
			cur.EBPFPrograms = append(cur.EBPFPrograms, o.EBPFPrograms...)
			cur.Incidents = append(cur.Incidents, o.Incidents...)
			out[node] = cur
		}
	}
	for node, o := range out {
		for src, names := range o.Metrics {
			o.Metrics[src] = dedupeStrings(names)
		}
		o.Pods = dedupePods(o.Pods)
		o.HostScrapers = dedupeStrings(o.HostScrapers)
		o.EBPFPrograms = dedupeStrings(o.EBPFPrograms)
		o.Incidents = dedupeStrings(o.Incidents)
		out[node] = o
	}
	return out
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func dedupePods(in []PodRef) []PodRef {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[PodRef]bool, len(in))
	out := make([]PodRef, 0, len(in))
	for _, p := range in {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}
