// Package crd implements the Knowledge layer's metric tree and dependency
// graph on top of the zoomlens.dev/v1alpha1 resources (MetricTreePack,
// MetricRelations, EntityGraph) curated in knowledge/.
//
// Scope of this implementation:
//   - DOWN: children of the firing root up to the ladder depth, depth-1
//     children of its siblings, explains targets and eBPF programs, exactly as
//     described in knowledge/README.md ("How an expansion works").
//   - Only sources the collector renderer supports are returned (cadvisor,
//     node-exporter, app, hostmetrics, ebpf); the rest are counted as skipped.
//   - OUT: the Pod <-> Node edges of the EntityGraph, gated by the root's
//     domain. Other entity classes (PVC, Service, ...) are not actuated yet.
//   - The escalation policy is not part of the CRDs and still comes from the
//     knowledge file.
package crd

import (
	"fmt"
	"sort"

	"github.com/zoomlens/zoomlens/api/v1alpha1"
	"github.com/zoomlens/zoomlens/internal/knowledge"
)

// Snapshot is an immutable, indexed view of the knowledge resources.
type Snapshot struct {
	packs     map[packKey]*v1alpha1.MetricTreePackSpec
	kindOf    map[string]knowledge.EntityKind // entity class -> operator kind
	rootsOf   map[rootKey][]packKey           // (root, operator kind) -> packs that have it as a root
	domainOf  map[string]v1alpha1.Domain      // root -> domain
	alwaysOn  map[string]map[string]bool      // source -> roots and companions (collected at L0)
	siblings  map[refKey][]v1alpha1.MetricRef // root -> other members of its sibling groups
	explains  map[refKey][]v1alpha1.DepthRef
	ebpf      map[refKey][]v1alpha1.ProgramRef
	edges     []edge
	supported map[string]knowledge.Source

	Stats Stats
}

// Stats summarises a snapshot for logs and /debug/state.
type Stats struct {
	Packs          int `json:"packs"`
	Roots          int `json:"roots"`
	SupportedRoots int `json:"supportedRoots"`
	Relations      int `json:"relations"`
	EntityEdges    int `json:"entityEdges"`
}

type packKey struct{ source, entity string }

type rootKey struct {
	root string
	kind knowledge.EntityKind
}

type refKey struct{ source, entity, metric string }

// edge is an EntityGraph edge between operator kinds.
type edge struct {
	from, to      knowledge.EntityKind
	bidirectional bool
	domains       map[v1alpha1.Domain]bool
}

// Build indexes the given resources. Duplicate packs (same source and entity)
// are an error; references to unknown packs are ignored.
func Build(packs []v1alpha1.MetricTreePack, rels []v1alpha1.MetricRelations, graphs []v1alpha1.EntityGraph) (*Snapshot, error) {
	s := &Snapshot{
		packs:     map[packKey]*v1alpha1.MetricTreePackSpec{},
		kindOf:    map[string]knowledge.EntityKind{},
		rootsOf:   map[rootKey][]packKey{},
		domainOf:  map[string]v1alpha1.Domain{},
		alwaysOn:  map[string]map[string]bool{},
		siblings:  map[refKey][]v1alpha1.MetricRef{},
		explains:  map[refKey][]v1alpha1.DepthRef{},
		ebpf:      map[refKey][]v1alpha1.ProgramRef{},
		supported: map[string]knowledge.Source{},
	}
	for _, src := range knowledge.KnownSources {
		s.supported[string(src)] = src
	}
	for _, g := range graphs {
		for _, c := range g.Spec.Entities {
			s.kindOf[c.Name] = knowledge.EntityKind(c.OperatorKindOf())
		}
	}
	for _, g := range graphs {
		for _, e := range g.Spec.Edges {
			ed := edge{from: s.kind(e.From), to: s.kind(e.To), bidirectional: e.Bidirectional == nil || *e.Bidirectional, domains: map[v1alpha1.Domain]bool{}}
			for _, d := range e.Domains {
				ed.domains[d] = true
			}
			s.edges = append(s.edges, ed)
		}
	}

	for i := range packs {
		p := &packs[i].Spec
		k := packKey{p.Source.Name, p.Entity}
		if _, dup := s.packs[k]; dup {
			return nil, fmt.Errorf("knowledge: two MetricTreePacks for source %q entity %q (second: %s)", k.source, k.entity, packs[i].Name)
		}
		s.packs[k] = p
		on := s.alwaysOn[k.source]
		if on == nil {
			on = map[string]bool{}
			s.alwaysOn[k.source] = on
		}
		for _, r := range p.Roots {
			rk := rootKey{r.Name, s.kind(p.Entity)}
			s.rootsOf[rk] = append(s.rootsOf[rk], k)
			if _, ok := s.domainOf[r.Name]; !ok {
				s.domainOf[r.Name] = r.Domain
			}
			on[r.Name] = true
			for _, c := range r.Companions {
				on[c] = true
			}
			s.Stats.Roots++
			if _, ok := s.supported[k.source]; ok {
				s.Stats.SupportedRoots++
			}
		}
	}
	s.Stats.Packs = len(s.packs)

	for _, r := range rels {
		s.Stats.Relations++
		for _, g := range r.Spec.Siblings {
			for _, m := range g.Members {
				for _, o := range g.Members {
					if o != m {
						s.siblings[ref(m)] = append(s.siblings[ref(m)], o)
					}
				}
			}
		}
		for _, x := range r.Spec.Explains {
			s.explains[ref(x.Root)] = append(s.explains[ref(x.Root)], x.Targets...)
		}
		for _, x := range r.Spec.EBPF {
			s.ebpf[ref(x.Root)] = append(s.ebpf[ref(x.Root)], x.Programs...)
		}
	}
	s.Stats.EntityEdges = len(s.edges)
	return s, nil
}

func ref(m v1alpha1.MetricRef) refKey { return refKey{m.Source, m.Entity, m.Metric} }

// kind maps an entity class to the operator's entity kind.
func (s *Snapshot) kind(class string) knowledge.EntityKind {
	if k, ok := s.kindOf[class]; ok {
		return k
	}
	return knowledge.EntityKind(class)
}

// Expand implements knowledge.MetricTree.
func (s *Snapshot) Expand(root string, kind knowledge.EntityKind, depth int) ([]knowledge.MetricRef, error) {
	if depth < 1 {
		return nil, nil
	}
	set := map[knowledge.MetricRef]bool{}
	add := func(source, name string) {
		src, ok := s.supported[source]
		if !ok || name == "" || s.alwaysOn[source][name] {
			return
		}
		set[knowledge.MetricRef{Source: src, Name: name}] = true
	}

	for _, pk := range s.rootsOf[rootKey{root, kind}] {
		p := s.packs[pk]
		// DOWN the pack's own tree, level by level.
		frontier := []string{root}
		for d := 1; d <= depth && len(frontier) > 0; d++ {
			var next []string
			for _, m := range frontier {
				for _, c := range p.Children[m] {
					add(pk.source, c)
					next = append(next, c)
				}
			}
			frontier = next
		}
		rk := refKey{pk.source, pk.entity, root}
		// Siblings: the same condition seen by another source, depth 1 only.
		for _, o := range s.siblings[rk] {
			if op, ok := s.packs[packKey{o.Source, o.Entity}]; ok {
				for _, c := range op.Children[o.Metric] {
					add(o.Source, c)
				}
			}
		}
		for _, t := range s.explains[rk] {
			if int(t.Depth) <= depth {
				add(t.Source, t.Metric)
			}
		}
		for _, prog := range s.ebpf[rk] {
			if int(prog.Depth) <= depth {
				add(string(knowledge.SourceEBPF), prog.Name)
			}
		}
	}

	out := make([]knowledge.MetricRef, 0, len(set))
	for m := range set {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Domain returns the domain of a root metric, if any pack declares it.
func (s *Snapshot) Domain(root string) (v1alpha1.Domain, bool) {
	d, ok := s.domainOf[root]
	return d, ok
}

// linked reports whether an edge from a to b admits domain d.
func (s *Snapshot) linked(a, b knowledge.EntityKind, d v1alpha1.Domain) bool {
	for _, e := range s.edges {
		if !e.domains[d] {
			continue
		}
		if (e.from == a && e.to == b) || (e.bidirectional && e.from == b && e.to == a) {
			return true
		}
	}
	return false
}

// AlwaysOn returns the roots and companions of one source: the metrics the
// baseline must collect so every alert on that source can be evaluated.
func (s *Snapshot) AlwaysOn(source string) []string {
	out := make([]string, 0, len(s.alwaysOn[source]))
	for m := range s.alwaysOn[source] {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}
