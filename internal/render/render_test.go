package render

import (
	"strings"
	"testing"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/zoomlens/zoomlens/internal/knowledge"
	"github.com/zoomlens/zoomlens/internal/plan"
)

func TestRenderEmptyIsNoop(t *testing.T) {
	body, err := Render(plan.NodeOverlay{Node: "n1"}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != NoopBody {
		t.Fatalf("got %q", body)
	}
	var m map[string]any
	if err := yaml.Unmarshal(body, &m); err != nil || len(m) != 0 {
		t.Fatalf("noop body must be an empty YAML map, got %v, %v", m, err)
	}
}

func TestRenderFullOverlay(t *testing.T) {
	o := plan.NodeOverlay{
		Node:     "kind-worker",
		Interval: 5 * time.Second,
		Metrics: map[knowledge.Source][]string{
			knowledge.SourceCAdvisor:     {"container_cpu_cfs_throttled_periods_total", "container_memory_rss"},
			knowledge.SourceNodeExporter: {"node_pressure_cpu_waiting_seconds_total"},
			knowledge.SourceApp:          {"demo_orders_total"},
		},
		Pods:         []plan.PodRef{{Namespace: "zoomlens-demo", Name: "demo-app-7d9f.x"}},
		HostScrapers: []string{"paging"},
		EBPFPrograms: []string{"biolatency"},
		Incidents:    []string{"abc"},
	}
	body, err := Render(o, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Receivers map[string]struct {
			Config struct {
				ScrapeConfigs []map[string]any `json:"scrape_configs"`
			} `json:"config"`
			CollectionInterval string         `json:"collection_interval"`
			Scrapers           map[string]any `json:"scrapers"`
		} `json:"receivers"`
		Service struct {
			Pipelines map[string]struct {
				Receivers []string `json:"receivers"`
				Exporters []string `json:"exporters"`
			} `json:"pipelines"`
		} `json:"service"`
	}
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("rendered YAML does not parse: %v\n%s", err, body)
	}
	jobs := doc.Receivers[PrometheusReceiver].Config.ScrapeConfigs
	if len(jobs) != 4 {
		t.Fatalf("want 4 scrape jobs (cadvisor, node-exporter, app, ebpf), got %d\n%s", len(jobs), body)
	}
	for _, j := range jobs {
		if j["scrape_interval"] != "5s" || j["scrape_timeout"] != "5s" {
			t.Errorf("job %v: interval/timeout = %v/%v", j["job_name"], j["scrape_interval"], j["scrape_timeout"])
		}
	}
	if hm := doc.Receivers[HostMetricsReceiver]; hm.CollectionInterval != "5s" || hm.Scrapers["paging"] == nil {
		t.Errorf("hostmetrics receiver = %+v", hm)
	}
	p := doc.Service.Pipelines[Pipeline]
	if len(p.Receivers) != 1 || p.Receivers[0] != PrometheusReceiver || p.Exporters[0] != "prometheusremotewrite" {
		t.Errorf("scrape pipeline = %+v", p)
	}
	hp := doc.Service.Pipelines[HostPipeline]
	if len(hp.Receivers) != 1 || hp.Receivers[0] != HostMetricsReceiver {
		t.Errorf("host pipeline = %+v", hp)
	}
	s := string(body)
	for _, want := range []string{`zoomlens-demo;demo-app-7d9f\.x`, "metadata.name=kind-worker", "spec.nodeName=kind-worker", "zl-ebpf"} {
		if !strings.Contains(s, want) {
			t.Errorf("rendered config missing %q\n%s", want, s)
		}
	}
	if strings.Contains(s, "$") {
		t.Errorf("rendered config must not contain '$' (collector env expansion)\n%s", s)
	}
}

func TestRenderRejectsBadInput(t *testing.T) {
	bad := plan.NodeOverlay{Node: "n1", Metrics: map[knowledge.Source][]string{knowledge.SourceCAdvisor: {"bad name"}}}
	if _, err := Render(bad, DefaultOptions()); err == nil {
		t.Fatal("expected error for invalid metric name")
	}
	badNode := plan.NodeOverlay{Node: "N_1", HostScrapers: []string{"paging"}}
	if _, err := Render(badNode, DefaultOptions()); err == nil {
		t.Fatal("expected error for invalid node name")
	}
}
