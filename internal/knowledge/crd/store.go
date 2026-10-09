package crd

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/zoomlens/zoomlens/internal/knowledge"
)

// Store holds the current Snapshot. Tree and Graph read from it, so a refresh
// takes effect on the next reconcile without restarting the operator.
type Store struct {
	cur atomic.Pointer[Snapshot]
}

// Set replaces the current snapshot.
func (s *Store) Set(sn *Snapshot) { s.cur.Store(sn) }

// Get returns the current snapshot, or nil before the first load.
func (s *Store) Get() *Snapshot { return s.cur.Load() }

var errNotLoaded = errors.New("knowledge resources not loaded yet")

// Tree is a knowledge.MetricTree backed by the Store.
type Tree struct{ Store *Store }

var _ knowledge.MetricTree = Tree{}

// Expand implements knowledge.MetricTree.
func (t Tree) Expand(root string, kind knowledge.EntityKind, depth int) ([]knowledge.MetricRef, error) {
	sn := t.Store.Get()
	if sn == nil {
		return nil, errNotLoaded
	}
	return sn.Expand(root, kind, depth)
}

// Graph gates an inner Pod/Node graph with the EntityGraph edges: it walks
// OUT only as far as the edges allow for the domain of the firing root.
// Roots unknown to the packs (e.g. application roots without a pack) are not
// gated.
type Graph struct {
	Store *Store
	// Inner resolves the Pod -> Node -> co-located Pods walk.
	Inner knowledge.DependencyGraph
}

var (
	_ knowledge.DependencyGraph = Graph{}
	_ knowledge.RootAwareGraph  = Graph{}
)

// Related implements knowledge.DependencyGraph without domain gating.
func (g Graph) Related(ctx context.Context, e knowledge.Entity, hops int) ([]knowledge.Entity, error) {
	return g.Inner.Related(ctx, e, hops)
}

// RelatedForRoot implements knowledge.RootAwareGraph.
func (g Graph) RelatedForRoot(ctx context.Context, root string, e knowledge.Entity, hops int) ([]knowledge.Entity, error) {
	return g.Inner.Related(ctx, e, g.allowedHops(root, e.Kind, hops))
}

// allowedHops is how many of the requested hops the EntityGraph permits.
// From a Pod: hop 1 is its Node, hop 2 the Pods on that Node. From a Node:
// hop 1 is the Pods on it.
func (g Graph) allowedHops(root string, kind knowledge.EntityKind, hops int) int {
	sn := g.Store.Get()
	if sn == nil {
		return hops
	}
	d, ok := sn.Domain(root)
	if !ok {
		return hops
	}
	path := []knowledge.EntityKind{knowledge.KindPod, knowledge.KindNode, knowledge.KindPod}
	if kind == knowledge.KindNode {
		path = path[1:]
	} else if kind != knowledge.KindPod {
		return 0
	}
	allowed := 0
	for allowed < hops && allowed+1 < len(path) && sn.linked(path[allowed], path[allowed+1], d) {
		allowed++
	}
	return allowed
}
