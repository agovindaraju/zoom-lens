package plan

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/zoomlens/zoomlens/internal/knowledge"
)

type fakeTree struct{ refs []knowledge.MetricRef }

func (f fakeTree) Expand(string, knowledge.EntityKind, int) ([]knowledge.MetricRef, error) {
	return f.refs, nil
}

type fakeGraph struct{ related []knowledge.Entity }

func (f fakeGraph) Related(context.Context, knowledge.Entity, int) ([]knowledge.Entity, error) {
	return f.related, nil
}

func TestPlanGroupsByNode(t *testing.T) {
	k := knowledge.Knowledge{
		Tree: fakeTree{refs: []knowledge.MetricRef{
			{Source: knowledge.SourceCAdvisor, Name: "container_cpu_cfs_throttled_periods_total"},
			{Source: knowledge.SourceNodeExporter, Name: "node_pressure_cpu_waiting_seconds_total"},
			{Source: knowledge.SourceHostMetrics, Name: "paging"},
			{Source: knowledge.SourceEBPF, Name: "biolatency"},
		}},
		Graph: fakeGraph{related: []knowledge.Entity{
			{Kind: knowledge.KindPod, Name: "app", Namespace: "demo", Node: "n1"},
			{Kind: knowledge.KindNode, Name: "n1", Node: "n1"},
			{Kind: knowledge.KindPod, Name: "other", Namespace: "demo", Node: "n2"},
		}},
	}
	scope, err := NewPlanner(k).Plan(context.Background(), Request{
		IncidentID: "i1", Root: "r", Entity: knowledge.Entity{Kind: knowledge.KindPod, Name: "app", Namespace: "demo"},
		Level: knowledge.Level{Index: 1, Depth: 1, Hops: 1, ScrapeInterval: 10 * time.Second, EnableEBPF: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := scope.Nodes(); !reflect.DeepEqual(got, []string{"n1", "n2"}) {
		t.Fatalf("nodes = %v", got)
	}
	n1 := scope["n1"]
	if len(n1.Pods) != 1 || n1.Pods[0].Name != "app" {
		t.Errorf("n1 pods = %v", n1.Pods)
	}
	if len(n1.EBPFPrograms) != 0 {
		t.Errorf("eBPF must stay off when the level disables it, got %v", n1.EBPFPrograms)
	}
	if !reflect.DeepEqual(n1.HostScrapers, []string{"paging"}) {
		t.Errorf("host scrapers = %v", n1.HostScrapers)
	}
}

func TestPlanLevelZeroIsEmpty(t *testing.T) {
	scope, err := NewPlanner(knowledge.Knowledge{}).Plan(context.Background(), Request{})
	if err != nil || len(scope) != 0 {
		t.Fatalf("level 0 should plan nothing, got %v, %v", scope, err)
	}
}

func TestMergeUnionsAndTakesShortestInterval(t *testing.T) {
	a := Scope{"n1": {Node: "n1", Interval: 10 * time.Second, Incidents: []string{"a"},
		Metrics: map[knowledge.Source][]string{knowledge.SourceCAdvisor: {"m1"}}, Pods: []PodRef{{"demo", "app"}}}}
	b := Scope{"n1": {Node: "n1", Interval: 5 * time.Second, Incidents: []string{"b"},
		Metrics: map[knowledge.Source][]string{knowledge.SourceCAdvisor: {"m1", "m2"}}, Pods: []PodRef{{"demo", "app"}},
		EBPFPrograms: []string{"biolatency"}}}
	got := Merge(a, b)["n1"]
	if got.Interval != 5*time.Second {
		t.Errorf("interval = %v, want 5s", got.Interval)
	}
	if !reflect.DeepEqual(got.Metrics[knowledge.SourceCAdvisor], []string{"m1", "m2"}) {
		t.Errorf("metrics = %v", got.Metrics)
	}
	if len(got.Pods) != 1 || !reflect.DeepEqual(got.Incidents, []string{"a", "b"}) || len(got.EBPFPrograms) != 1 {
		t.Errorf("merged overlay = %+v", got)
	}
}
