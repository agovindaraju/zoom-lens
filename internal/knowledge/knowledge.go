// Package knowledge defines the contract between the Zoom-Lens operator and the
// "Knowledge" layer: the metric tree, the dependency graph and the escalation
// policy.
//
// The operator only depends on the interfaces in this file. The implementations are:
//
//	MetricTree        package crd      (MetricTreePack and MetricRelations resources)
//	DependencyGraph   package crd      (EntityGraph resource), with package topology resolving
//	                                   Pod -> Node -> co-located Pods from the Kubernetes API
//	EscalationPolicy  package policy   (the ladder file)
//
// See docs/KNOWLEDGE-INTERFACES.md. To use a different knowledge source, implement these interfaces.
package knowledge

import (
	"context"
	"time"
)

// Source identifies where a metric comes from. It decides which scrape
// template the renderer uses for it.
type Source string

const (
	// SourceCAdvisor is the kubelet's cAdvisor endpoint (/metrics/cadvisor).
	SourceCAdvisor Source = "cadvisor"
	// SourceNodeExporter is the Prometheus node-exporter DaemonSet.
	SourceNodeExporter Source = "node-exporter"
	// SourceApp is any application pod annotated with zoomlens.dev/scrape=true.
	SourceApp Source = "app"
	// SourceHostMetrics is an OpenTelemetry hostmetrics scraper. Name is the
	// scraper name, e.g. "paging" or "processes".
	SourceHostMetrics Source = "hostmetrics"
	// SourceEBPF is an ebpf_exporter program. Name is the example config
	// name, e.g. "biolatency".
	SourceEBPF Source = "ebpf"
)

// KnownSources lists every Source the renderer can handle.
var KnownSources = []Source{SourceCAdvisor, SourceNodeExporter, SourceApp, SourceHostMetrics, SourceEBPF}

// MetricRef names one metric (or scraper / eBPF program) and its source.
type MetricRef struct {
	Source Source `json:"source"`
	Name   string `json:"name"`
}

// EntityKind is the type of an entity in the dependency graph.
type EntityKind string

const (
	KindPod  EntityKind = "Pod"
	KindNode EntityKind = "Node"
)

// Entity is one node of the dependency graph.
type Entity struct {
	Kind      EntityKind `json:"kind"`
	Name      string     `json:"name"`
	Namespace string     `json:"namespace,omitempty"`
	// Node is the Kubernetes node the entity lives on. For a Node entity it
	// equals Name. It is empty for cluster-scoped entities that have no node.
	Node string `json:"node,omitempty"`
}

// Level is one rung of the dwell ladder.
type Level struct {
	// Index is 1 for L1, 2 for L2, ... Index 0 means "no escalation".
	Index int `json:"index"`
	// Depth is how far DOWN the metric tree to expand (1 = direct children).
	Depth int `json:"depth"`
	// Hops is how far OUT the dependency graph to expand.
	Hops int `json:"hops"`
	// ScrapeInterval is used for all metrics added at this level.
	ScrapeInterval time.Duration `json:"scrapeInterval"`
	// EnableEBPF allows eBPF programs from the tree to be started.
	EnableEBPF bool `json:"enableEBPF"`
}

// MetricTree answers "which finer metrics sit below this root?".
type MetricTree interface {
	// Expand returns every metric below root for the given entity kind, down
	// to depth (1 = direct children). The root itself is not included. An
	// unknown root returns an empty slice and no error.
	Expand(root string, kind EntityKind, depth int) ([]MetricRef, error)
}

// DependencyGraph answers "which entities are related to this one?".
type DependencyGraph interface {
	// Related returns the entity itself plus every entity up to hops away.
	// Implementations must fill Entity.Node for node-bound entities, because
	// the operator actuates collection per node.
	Related(ctx context.Context, e Entity, hops int) ([]Entity, error)
}

// RootAwareGraph is an optional extension of DependencyGraph for graphs whose
// edges are gated by the problem domain of the firing root. When the Graph
// implements it, the planner calls RelatedForRoot instead of Related.
type RootAwareGraph interface {
	RelatedForRoot(ctx context.Context, root string, e Entity, hops int) ([]Entity, error)
}

// EscalationPolicy defines the dwell ladder.
type EscalationPolicy interface {
	// LevelFor returns the level for an alert that has been firing for dwell.
	LevelFor(dwell time.Duration) Level
	// ClearFor is how long an alert must stay resolved before collection
	// reverts to the baseline.
	ClearFor() time.Duration
}

// Knowledge bundles the three parts the operator needs.
type Knowledge struct {
	Tree   MetricTree
	Graph  DependencyGraph
	Policy EscalationPolicy
}
