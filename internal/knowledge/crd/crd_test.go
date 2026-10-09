package crd

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/zoomlens/zoomlens/internal/knowledge"
)

// The curated knowledge in the repository plus the demo app pack.
func loadRepo(t *testing.T) *Snapshot {
	t.Helper()
	sn, err := LoadFiles("../../../knowledge/packs", "../../../knowledge/relations.yaml",
		"../../../knowledge/entity-graph.yaml", "../../../deploy/demo/knowledge-pack.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return sn
}

func names(refs []knowledge.MetricRef, src knowledge.Source) []string {
	var out []string
	for _, r := range refs {
		if r.Source == src {
			out = append(out, r.Name)
		}
	}
	return out
}

func has(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func TestLoadRepoKnowledge(t *testing.T) {
	sn := loadRepo(t)
	if sn.Stats.Packs < 30 || sn.Stats.Roots < 100 || sn.Stats.EntityEdges < 30 {
		t.Fatalf("unexpectedly small knowledge: %+v", sn.Stats)
	}
}

func TestExpandNodeDisk(t *testing.T) {
	sn := loadRepo(t)
	root := "node_disk_io_time_seconds_total"

	d1, _ := sn.Expand(root, knowledge.KindNode, 1)
	ne := names(d1, knowledge.SourceNodeExporter)
	if !has(ne, "node_disk_write_time_seconds_total") || !has(ne, "node_pressure_io_waiting_seconds_total") {
		t.Errorf("depth 1 misses direct children: %v", ne)
	}
	if has(ne, "node_pressure_io_stalled_seconds_total") {
		t.Errorf("depth 1 contains a depth-2 child: %v", ne)
	}
	if has(ne, root) {
		t.Errorf("Expand returned the root itself")
	}
	if !has(names(d1, knowledge.SourceHostMetrics), "filesystem") {
		t.Errorf("depth 1 misses the hostmetrics sibling's children: %v", d1)
	}
	if len(names(d1, knowledge.SourceEBPF)) != 0 {
		t.Errorf("eBPF programs at depth 1: %v", d1)
	}
	for _, r := range d1 {
		if r.Source == "kubelet" {
			t.Errorf("unsupported source returned: %v", r)
		}
	}

	d2, _ := sn.Expand(root, knowledge.KindNode, 2)
	if !has(names(d2, knowledge.SourceNodeExporter), "node_pressure_io_stalled_seconds_total") {
		t.Errorf("depth 2 misses grandchildren")
	}
	if got := names(d2, knowledge.SourceEBPF); len(got) != 1 || got[0] != "biolatency" {
		t.Errorf("depth 2 eBPF = %v, want [biolatency]", got)
	}
	d3, _ := sn.Expand(root, knowledge.KindNode, 3)
	if got := names(d3, knowledge.SourceEBPF); !has(got, "ext4dist") {
		t.Errorf("depth 3 eBPF = %v, want ext4dist too", got)
	}
}

func TestExpandNeverReturnsAlwaysOnMetrics(t *testing.T) {
	sn := loadRepo(t)
	for _, root := range []string{"node_cpu_seconds_total", "node_load1", "node_memory_MemAvailable_bytes", "node_disk_io_time_seconds_total"} {
		refs, _ := sn.Expand(root, knowledge.KindNode, 3)
		for _, r := range refs {
			if sn.alwaysOn[string(r.Source)][r.Name] {
				t.Errorf("%s: returned always-on metric %v", root, r)
			}
		}
	}
}

func TestExpandContainerRootActsOnPod(t *testing.T) {
	sn := loadRepo(t)
	// cadvisor-container is entity Container, operatorKind Pod.
	refs, _ := sn.Expand("container_cpu_usage_seconds_total", knowledge.KindPod, 1)
	if !has(names(refs, knowledge.SourceCAdvisor), "container_cpu_user_seconds_total") {
		t.Errorf("container root not expanded for Pod: %v", refs)
	}
	if !has(names(refs, knowledge.SourceNodeExporter), "node_pressure_cpu_waiting_seconds_total") {
		t.Errorf("explains target missing: %v", refs)
	}
	if refs, _ := sn.Expand("container_cpu_usage_seconds_total", knowledge.KindNode, 1); len(refs) != 0 {
		t.Errorf("wrong entity kind expanded: %v", refs)
	}
}

func TestExpandDemoApp(t *testing.T) {
	sn := loadRepo(t)
	root := "http_server_request_duration_seconds"
	d1, _ := sn.Expand(root, knowledge.KindPod, 1)
	if !has(names(d1, knowledge.SourceApp), "demo_inflight_requests") || !has(names(d1, knowledge.SourceCAdvisor), "container_memory_rss") {
		t.Errorf("demo depth 1 = %v", d1)
	}
	d2, _ := sn.Expand(root, knowledge.KindPod, 2)
	if !has(names(d2, knowledge.SourceEBPF), "biolatency") || !has(names(d2, knowledge.SourceHostMetrics), "paging") {
		t.Errorf("demo depth 2 = %v", d2)
	}
	if refs, _ := sn.Expand("no_such_root", knowledge.KindPod, 3); len(refs) != 0 {
		t.Errorf("unknown root expanded: %v", refs)
	}
}

type fakeGraph struct{ gotHops int }

func (f *fakeGraph) Related(_ context.Context, e knowledge.Entity, hops int) ([]knowledge.Entity, error) {
	f.gotHops = hops
	return []knowledge.Entity{e}, nil
}

func TestGraphDomainGating(t *testing.T) {
	st := &Store{}
	st.Set(loadRepo(t))
	cases := []struct {
		root string
		kind knowledge.EntityKind
		hops int
		want int
	}{
		{"container_cpu_usage_seconds_total", knowledge.KindPod, 2, 2}, // cpu: runs-on both ways
		{"node_disk_io_time_seconds_total", knowledge.KindNode, 2, 1},  // Node -> Pods is the only hop
		{"node_timex_offset_seconds", knowledge.KindNode, 1, 1},        // lifecycle is on runs-on
		{"unknown_root", knowledge.KindPod, 2, 2},                      // not gated
		{"container_cpu_usage_seconds_total", knowledge.KindPod, 0, 0},
	}
	for _, c := range cases {
		f := &fakeGraph{}
		g := Graph{Store: st, Inner: f}
		if _, err := g.RelatedForRoot(context.Background(), c.root, knowledge.Entity{Kind: c.kind, Name: "x", Node: "n"}, c.hops); err != nil {
			t.Fatal(err)
		}
		if f.gotHops != c.want {
			t.Errorf("%s from %s hops %d: got %d, want %d", c.root, c.kind, c.hops, f.gotHops, c.want)
		}
	}
}

// The collector baseline must collect every root and companion of the
// node-exporter and cadvisor packs, so each of their alert rules can be
// evaluated in steady state.
func TestBaselineCollectsAlwaysOnMetrics(t *testing.T) {
	sn := loadRepo(t)
	raw, err := os.ReadFile("../../../deploy/collector/base-config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	jobRE := map[string]string{"node-exporter": "base-node-exporter", "cadvisor": "base-cadvisor"}
	for src, job := range jobRE {
		keep := keepRegex(t, string(raw), job)
		re := regexp.MustCompile("^(?:" + keep + ")$")
		for _, m := range sn.AlwaysOn(src) {
			if !re.MatchString(m) {
				t.Errorf("%s: baseline job %s does not keep %s", src, job, m)
			}
		}
	}
}

// keepRegex returns the __name__ keep regex of one baseline scrape job.
func keepRegex(t *testing.T, cfg, job string) string {
	t.Helper()
	i := strings.Index(cfg, "job_name: "+job)
	if i < 0 {
		t.Fatalf("job %s not in baseline", job)
	}
	rest := cfg[i:]
	j := strings.Index(rest, "source_labels: [__name__]")
	if j < 0 {
		t.Fatalf("job %s has no __name__ filter", job)
	}
	m := regexp.MustCompile(`regex: (\S+)`).FindStringSubmatch(rest[j:])
	if m == nil {
		t.Fatalf("job %s: no keep regex", job)
	}
	return m[1]
}
