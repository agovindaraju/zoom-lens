package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Domain is the canonical problem domain of a root metric. It decides which
// entity-graph edges an alert on that root may follow.
// +kubebuilder:validation:Enum=cpu;memory;storage;network;dns;lifecycle;control-plane;scheduling
type Domain string

// Domains in the order they are documented.
var Domains = []Domain{"cpu", "memory", "storage", "network", "dns", "lifecycle", "control-plane", "scheduling"}

// Scope says which collector scrapes a metric source.
// +kubebuilder:validation:Enum=per-node;cluster;control-plane
type Scope string

// NameKind says what the names in a MetricTreePack are.
// +kubebuilder:validation:Enum=metric;scraper;ebpf-program
type NameKind string

const (
	// NameKindMetric names are Prometheus exposition names. Histograms are
	// listed by base name (hydrate to name_bucket, name_sum, name_count).
	NameKindMetric NameKind = "metric"
	// NameKindScraper names are OpenTelemetry hostmetrics scraper names.
	NameKindScraper NameKind = "scraper"
	// NameKindEBPFProgram names are ebpf_exporter example config names.
	NameKindEBPFProgram NameKind = "ebpf-program"
)

// ---------------------------------------------------------------------------
// MetricTreePack
// ---------------------------------------------------------------------------

// MetricSource describes where the metrics of a pack come from.
type MetricSource struct {
	// Name is the source id used by the operator's scrape templates and in the
	// zoomlens_source alert label, e.g. node-exporter, cadvisor, kubelet.
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
	Name string `json:"name"`
	// Component is the upstream project and version the names were taken from.
	Component string `json:"component"`
	// Job is the Prometheus job name the source is scraped under.
	// +optional
	Job string `json:"job,omitempty"`
	// Endpoint is the port and path that serve the metrics.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// Scope says which collector scrapes the source.
	Scope Scope `json:"scope"`
	// NameKind says what the names in this pack are. Defaults to metric.
	// +kubebuilder:default=metric
	// +optional
	NameKind NameKind `json:"nameKind,omitempty"`
}

// RootMetric is always collected (L0). Its alert rule starts an expansion.
type RootMetric struct {
	// Name of the root metric.
	Name string `json:"name"`
	// Domain of the root; gates which entity-graph edges are followed.
	Domain Domain `json:"domain"`
	// Companions are collected together with the root at L0 because the
	// root's alert expression needs them (denominators and guards). They are
	// never expanded.
	// +optional
	Companions []string `json:"companions,omitempty"`
}

// EBPFProgram is an ebpf_exporter program that can be started on a node.
type EBPFProgram struct {
	// Name is the ebpf_exporter example config name (--config.names).
	Name string `json:"name"`
	// Domain the program gives kernel-level detail for.
	Domain Domain `json:"domain"`
	// Metrics are the Prometheus metrics the program exposes.
	Metrics []string `json:"metrics"`
	// Cardinality is a rough guide to the number of series it produces.
	// +optional
	Cardinality string `json:"cardinality,omitempty"`
	// Kernel lists kernel requirements beyond the ebpf_exporter baseline.
	// +optional
	Kernel string `json:"kernel,omitempty"`
}

// MetricTreePackSpec is the metric tree of one source for one entity kind.
type MetricTreePackSpec struct {
	// Source of the metrics in this pack.
	Source MetricSource `json:"source"`
	// Entity is the Kubernetes entity class the metrics describe (see
	// EntityGraph.spec.entities), e.g. Node, Pod, Container.
	Entity string `json:"entity"`
	// IdentityLabels identify the entity on every series of this pack.
	// +optional
	IdentityLabels []string `json:"identityLabels,omitempty"`
	// Optional packs belong to add-ons that only some clusters run.
	// +optional
	Optional bool `json:"optional,omitempty"`
	// Roots are always collected.
	// +optional
	Roots []RootMetric `json:"roots,omitempty"`
	// Children maps a metric to the finer metrics collected one ladder level
	// deeper (depth 1 = L1, 2 = L2, 3 = L3).
	// +optional
	Children map[string][]string `json:"children,omitempty"`
	// EBPFPrograms is only set on the ebpf pack. Programs are never roots;
	// they are reached through MetricRelations.spec.ebpf.
	// +optional
	EBPFPrograms []EBPFProgram `json:"ebpfPrograms,omitempty"`
	// Description holds caveats: required flags, feature gates, deprecations.
	// +optional
	Description string `json:"description,omitempty"`
}

// MetricTreePack is the curated metric tree of one metric source for one
// Kubernetes entity kind.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=mtp
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.source.name`
// +kubebuilder:printcolumn:name="Entity",type=string,JSONPath=`.spec.entity`
// +kubebuilder:printcolumn:name="Optional",type=boolean,JSONPath=`.spec.optional`
type MetricTreePack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MetricTreePackSpec `json:"spec"`
}

// MetricTreePackList is a list of MetricTreePack.
//
// +kubebuilder:object:root=true
type MetricTreePackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MetricTreePack `json:"items"`
}

// ---------------------------------------------------------------------------
// MetricRelations
// ---------------------------------------------------------------------------

// MetricRef points at one metric of one pack.
type MetricRef struct {
	// Source is MetricTreePack.spec.source.name.
	Source string `json:"source"`
	// Entity is MetricTreePack.spec.entity.
	Entity string `json:"entity"`
	// Metric is the metric name inside that pack.
	Metric string `json:"metric"`
}

// SiblingGroup is a set of roots in different packs that report the same
// condition. When one fires, the others' depth-1 children are collected too.
type SiblingGroup struct {
	// Name of the group, for humans.
	Name string `json:"name"`
	// Domain of the condition.
	Domain Domain `json:"domain"`
	// Members are roots.
	// +kubebuilder:validation:MinItems=2
	Members []MetricRef `json:"members"`
}

// DepthRef is a metric collected once the ladder reaches Depth.
type DepthRef struct {
	MetricRef `json:",inline"`
	// Depth is the ladder level (1..3) from which the metric is collected.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3
	Depth int32 `json:"depth"`
}

// ExplainsLink lists specific child metrics of another entity that explain a
// root. It is a targeted, cheaper alternative to collecting every depth-1
// child of the related entity.
type ExplainsLink struct {
	// Root that fires.
	Root MetricRef `json:"root"`
	// Targets are child metrics (not roots) of other packs.
	Targets []DepthRef `json:"targets"`
}

// ProgramRef is an eBPF program started once the ladder reaches Depth.
type ProgramRef struct {
	// Name is an EBPFProgram name.
	Name string `json:"name"`
	// Depth is the ladder level (2..3) from which the program runs.
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=3
	Depth int32 `json:"depth"`
}

// EBPFLink lists the eBPF programs started on the affected node for a root.
type EBPFLink struct {
	// Root that fires.
	Root MetricRef `json:"root"`
	// Programs to start on the node, only on levels that enable eBPF.
	Programs []ProgramRef `json:"programs"`
}

// MetricRelationsSpec links metric trees across sources.
type MetricRelationsSpec struct {
	// Siblings are roots in different sources that report the same condition.
	// +optional
	Siblings []SiblingGroup `json:"siblings,omitempty"`
	// Explains are targeted cross-layer children.
	// +optional
	Explains []ExplainsLink `json:"explains,omitempty"`
	// EBPF are the deepest, on-demand kernel-level children.
	// +optional
	EBPF []EBPFLink `json:"ebpf,omitempty"`
}

// MetricRelations links the MetricTreePacks of different sources.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=mrel
type MetricRelations struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MetricRelationsSpec `json:"spec"`
}

// MetricRelationsList is a list of MetricRelations.
//
// +kubebuilder:object:root=true
type MetricRelationsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MetricRelations `json:"items"`
}

// ---------------------------------------------------------------------------
// EntityGraph
// ---------------------------------------------------------------------------

// EntityClass is one kind of node in the entity graph.
type EntityClass struct {
	// Name of the class, e.g. Node, Pod, PersistentVolumeClaim.
	Name string `json:"name"`
	// OperatorKind is the value of the zoomlens_entity alert label for
	// entities of this class. Defaults to Name.
	// +optional
	OperatorKind string `json:"operatorKind,omitempty"`
	// JoinLabels join series of this entity across sources.
	// +optional
	JoinLabels []string `json:"joinLabels,omitempty"`
	// Description says how to join when labels alone are not enough.
	// +optional
	Description string `json:"description,omitempty"`
}

// Discovery says how the operator builds an edge.
// +kubebuilder:validation:Enum=informer;rule;annotation;metrics
type Discovery string

// EntityEdge is a typed dependency between two entity classes.
type EntityEdge struct {
	// From entity class.
	From string `json:"from"`
	// Relation name, e.g. runs-on, mounts, owned-by.
	Relation string `json:"relation"`
	// To entity class.
	To string `json:"to"`
	// Discovery says how the edge is built.
	Discovery Discovery `json:"discovery"`
	// DefinedBy is the field, object or rule that defines the edge.
	DefinedBy string `json:"definedBy"`
	// Domains gate traversal: the edge is followed only for roots in one of
	// these domains.
	// +kubebuilder:validation:MinItems=1
	Domains []Domain `json:"domains"`
	// Bidirectional edges are traversed both ways. Defaults to true.
	// +kubebuilder:default=true
	// +optional
	Bidirectional *bool `json:"bidirectional,omitempty"`
	// Description adds context.
	// +optional
	Description string `json:"description,omitempty"`
}

// EntityGraphSpec is the Kubernetes entity dependency graph.
type EntityGraphSpec struct {
	// Entities are the entity classes.
	Entities []EntityClass `json:"entities"`
	// Edges between entity classes.
	Edges []EntityEdge `json:"edges"`
}

// EntityGraph is the typed Kubernetes entity dependency graph used for the
// OUT axis of an expansion.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=eg
type EntityGraph struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec EntityGraphSpec `json:"spec"`
}

// EntityGraphList is a list of EntityGraph.
//
// +kubebuilder:object:root=true
type EntityGraphList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EntityGraph `json:"items"`
}

// OperatorKindOf returns the zoomlens_entity value for an entity class.
func (c EntityClass) OperatorKindOf() string {
	if c.OperatorKind != "" {
		return c.OperatorKind
	}
	return c.Name
}
