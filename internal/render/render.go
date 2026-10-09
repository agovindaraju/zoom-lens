// Package render converts a plan.NodeOverlay into an OpenTelemetry Collector
// configuration fragment.
//
// The OpAMP Supervisor merges this fragment on top of the collector's local
// baseline config (deploy/collector/base-config.yaml). Maps are merged deeply,
// so the fragment only adds its own receivers and its own pipeline; it
// references the exporter and processors defined by the baseline by name.
package render

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/zoomlens/zoomlens/internal/knowledge"
	"github.com/zoomlens/zoomlens/internal/plan"
)

// Component names used in the rendered fragment.
const (
	PrometheusReceiver  = "prometheus/zoomlens"
	HostMetricsReceiver = "hostmetrics/zoomlens"
	Pipeline            = "metrics/zoomlens"
	HostPipeline        = "metrics/zoomlens-host"
)

// NoopBody is sent when a node has no active incident. It must not be empty:
// the Supervisor stops the collector when it receives an empty config map.
const NoopBody = "# zoomlens: no active incident on this node, baseline only\n{}\n"

// Options must match the baseline collector config.
type Options struct {
	// Exporter is the baseline exporter the Zoom-Lens pipeline sends to.
	Exporter string
	// Processors are baseline processors the Zoom-Lens pipeline uses.
	Processors []string
	// HostProcessors are baseline processors for the hostmetrics pipeline.
	// They typically include a resource processor that sets service.name and
	// service.instance.id, which become the job and instance labels.
	HostProcessors []string
	// HostRootPath must equal the baseline hostmetrics root_path. Empty
	// omits root_path (collector not running in a container).
	HostRootPath string
	// ServiceAccountTokenFile authenticates to the kubelet.
	ServiceAccountTokenFile string
	// NodeExporterPort identifies node-exporter pods.
	NodeExporterPort int
	// NodeExporterSelector is the label selector for node-exporter pods.
	NodeExporterSelector string
	// AppScrapeAnnotation is the pod annotation (as a Prometheus meta label
	// suffix) that marks application pods for scraping.
	AppScrapeAnnotation string
	// AppPortName is the container port name exposing app metrics.
	AppPortName string
	// EBPFNamespace is where the operator creates ebpf_exporter pods.
	EBPFNamespace string
	// EBPFSelector is the label selector of ebpf_exporter pods.
	EBPFSelector string
	// EBPFPort is the ebpf_exporter metrics port.
	EBPFPort int
}

// DefaultOptions returns options matching the manifests in deploy/.
func DefaultOptions() Options {
	return Options{
		Exporter:                "prometheusremotewrite",
		Processors:              []string{"memory_limiter", "batch"},
		HostProcessors:          []string{"memory_limiter", "resource/host", "batch"},
		HostRootPath:            "/hostfs",
		ServiceAccountTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
		NodeExporterPort:        9100,
		NodeExporterSelector:    "app.kubernetes.io/name=prometheus-node-exporter",
		AppScrapeAnnotation:     "zoomlens_dev_scrape",
		AppPortName:             "metrics",
		EBPFNamespace:           "zoomlens",
		EBPFSelector:            "app.kubernetes.io/component=ebpf-exporter",
		EBPFPort:                9435,
	}
}

var (
	metricNameRE = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	dnsNameRE    = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)
	scraperRE    = regexp.MustCompile(`^[a-z_]+$`)
)

// Render returns the collector config fragment for one node.
func Render(o plan.NodeOverlay, opt Options) ([]byte, error) {
	if o.Empty() {
		return []byte(NoopBody), nil
	}
	if !dnsNameRE.MatchString(o.Node) {
		return nil, fmt.Errorf("render: invalid node name %q", o.Node)
	}
	interval := o.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}

	var jobs []any
	for _, src := range []knowledge.Source{knowledge.SourceCAdvisor, knowledge.SourceNodeExporter, knowledge.SourceApp} {
		names := o.Metrics[src]
		if len(names) == 0 {
			continue
		}
		keep, err := alternation(names, metricNameRE, "metric name")
		if err != nil {
			return nil, err
		}
		job, err := scrapeJob(src, o, interval, keep, opt)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if len(o.EBPFPrograms) > 0 {
		jobs = append(jobs, ebpfJob(o.Node, interval, opt))
	}

	receivers := map[string]any{}
	pipelines := map[string]any{}
	if len(jobs) > 0 {
		receivers[PrometheusReceiver] = map[string]any{"config": map[string]any{"scrape_configs": jobs}}
		pipelines[Pipeline] = pipeline([]string{PrometheusReceiver}, opt.Processors, opt.Exporter)
	}
	if len(o.HostScrapers) > 0 {
		scrapers := map[string]any{}
		for _, s := range o.HostScrapers {
			if !scraperRE.MatchString(s) {
				return nil, fmt.Errorf("render: invalid hostmetrics scraper %q", s)
			}
			scrapers[s] = map[string]any{}
		}
		hm := map[string]any{"collection_interval": interval.String(), "scrapers": scrapers}
		if opt.HostRootPath != "" {
			hm["root_path"] = opt.HostRootPath
		}
		receivers[HostMetricsReceiver] = hm
		pipelines[HostPipeline] = pipeline([]string{HostMetricsReceiver}, opt.HostProcessors, opt.Exporter)
	}
	if len(pipelines) == 0 {
		return []byte(NoopBody), nil
	}

	doc := map[string]any{
		"receivers": receivers,
		"service":   map[string]any{"pipelines": pipelines},
	}
	body, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("render: marshal: %w", err)
	}
	header := fmt.Sprintf("# zoomlens overlay for node %s, incidents: %s\n", o.Node, strings.Join(o.Incidents, ","))
	return append([]byte(header), body...), nil
}

func pipeline(receivers, processors []string, exporter string) map[string]any {
	p := map[string]any{"receivers": receivers, "exporters": []string{exporter}}
	if len(processors) > 0 {
		p["processors"] = processors
	}
	return p
}

func scrapeJob(src knowledge.Source, o plan.NodeOverlay, interval time.Duration, keep string, opt Options) (map[string]any, error) {
	job := map[string]any{
		"job_name":        "zl-" + string(src),
		"scrape_interval": interval.String(),
		"scrape_timeout":  interval.String(),
		"metric_relabel_configs": []any{
			map[string]any{"source_labels": []string{"__name__"}, "regex": keep, "action": "keep"},
		},
	}
	podFilter, err := podAlternation(o.Pods)
	if err != nil {
		return nil, err
	}
	nodeLabel := map[string]any{"source_labels": []string{"__meta_kubernetes_pod_node_name"}, "target_label": "node"}

	switch src {
	case knowledge.SourceCAdvisor:
		job["scheme"] = "https"
		job["metrics_path"] = "/metrics/cadvisor"
		job["tls_config"] = map[string]any{"insecure_skip_verify": true}
		job["authorization"] = map[string]any{"credentials_file": opt.ServiceAccountTokenFile}
		job["kubernetes_sd_configs"] = []any{map[string]any{
			"role":      "node",
			"selectors": []any{map[string]any{"role": "node", "field": "metadata.name=" + o.Node}},
		}}
		job["relabel_configs"] = []any{
			map[string]any{"source_labels": []string{"__meta_kubernetes_node_name"}, "target_label": "node"},
		}
		if podFilter != "" {
			job["metric_relabel_configs"] = append(job["metric_relabel_configs"].([]any), map[string]any{
				"source_labels": []string{"namespace", "pod"}, "separator": ";", "regex": podFilter, "action": "keep",
			})
		}
	case knowledge.SourceNodeExporter:
		job["kubernetes_sd_configs"] = []any{podSD(o.Node, opt.NodeExporterSelector, "")}
		job["relabel_configs"] = []any{
			map[string]any{"source_labels": []string{"__meta_kubernetes_pod_container_port_number"}, "regex": fmt.Sprint(opt.NodeExporterPort), "action": "keep"},
			nodeLabel,
		}
	case knowledge.SourceApp:
		relabel := []any{
			map[string]any{"source_labels": []string{"__meta_kubernetes_pod_annotation_" + opt.AppScrapeAnnotation}, "regex": "true", "action": "keep"},
			map[string]any{"source_labels": []string{"__meta_kubernetes_pod_container_port_name"}, "regex": opt.AppPortName, "action": "keep"},
		}
		if podFilter != "" {
			relabel = append(relabel, map[string]any{
				"source_labels": []string{"__meta_kubernetes_namespace", "__meta_kubernetes_pod_name"}, "separator": ";", "regex": podFilter, "action": "keep",
			})
		}
		relabel = append(relabel,
			map[string]any{"source_labels": []string{"__meta_kubernetes_namespace"}, "target_label": "namespace"},
			map[string]any{"source_labels": []string{"__meta_kubernetes_pod_name"}, "target_label": "pod"},
			nodeLabel,
		)
		job["kubernetes_sd_configs"] = []any{podSD(o.Node, "", "")}
		job["relabel_configs"] = relabel
	default:
		return nil, fmt.Errorf("render: source %q is not scrape-based", src)
	}
	return job, nil
}

func ebpfJob(node string, interval time.Duration, opt Options) map[string]any {
	return map[string]any{
		"job_name":              "zl-ebpf",
		"scrape_interval":       interval.String(),
		"scrape_timeout":        interval.String(),
		"kubernetes_sd_configs": []any{podSD(node, opt.EBPFSelector, opt.EBPFNamespace)},
		"relabel_configs": []any{
			map[string]any{"source_labels": []string{"__meta_kubernetes_pod_container_port_number"}, "regex": fmt.Sprint(opt.EBPFPort), "action": "keep"},
			map[string]any{"source_labels": []string{"__meta_kubernetes_pod_node_name"}, "target_label": "node"},
		},
	}
}

func podSD(node, labelSelector, namespace string) map[string]any {
	sel := map[string]any{"role": "pod", "field": "spec.nodeName=" + node}
	if labelSelector != "" {
		sel["label"] = labelSelector
	}
	sd := map[string]any{"role": "pod", "selectors": []any{sel}}
	if namespace != "" {
		sd["namespaces"] = map[string]any{"names": []string{namespace}}
	}
	return sd
}

func alternation(values []string, valid *regexp.Regexp, what string) (string, error) {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	quoted := make([]string, 0, len(sorted))
	for _, v := range sorted {
		if !valid.MatchString(v) {
			return "", fmt.Errorf("render: invalid %s %q", what, v)
		}
		quoted = append(quoted, regexp.QuoteMeta(v))
	}
	return strings.Join(quoted, "|"), nil
}

func podAlternation(pods []plan.PodRef) (string, error) {
	if len(pods) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(pods))
	for _, p := range pods {
		if !dnsNameRE.MatchString(p.Namespace) || !dnsNameRE.MatchString(p.Name) {
			return "", fmt.Errorf("render: invalid pod reference %s/%s", p.Namespace, p.Name)
		}
		parts = append(parts, regexp.QuoteMeta(p.Namespace)+";"+regexp.QuoteMeta(p.Name))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|"), nil
}
