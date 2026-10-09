// Package metrics holds the operator's own Prometheus metrics (zoomlens_*).
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics are the operator's self-observability metrics.
type Metrics struct {
	Registry *prometheus.Registry

	AlertsReceived  *prometheus.CounterVec
	AlertsSkipped   prometheus.Counter
	Incidents       *prometheus.GaugeVec
	IncidentLevel   *prometheus.GaugeVec
	Escalations     *prometheus.CounterVec
	BudgetBlocked   prometheus.Counter
	AgentsConnected prometheus.Gauge
	ConfigPushes    *prometheus.CounterVec
	RemoteConfig    *prometheus.GaugeVec
	EBPFPods        prometheus.Gauge
	OverlayNodes    prometheus.Gauge
	ReconcileErrors *prometheus.CounterVec
}

// New registers and returns all metrics.
func New() *Metrics {
	m := &Metrics{
		Registry: prometheus.NewRegistry(),
		AlertsReceived: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "zoomlens_alerts_received_total", Help: "Zoom-Lens alerts received from Alertmanager, by status.",
		}, []string{"status"}),
		AlertsSkipped: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "zoomlens_alerts_skipped_total", Help: "Alerts ignored because they lack Zoom-Lens labels.",
		}),
		Incidents: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "zoomlens_incidents", Help: "Tracked incidents, by state (firing, cooldown).",
		}, []string{"state"}),
		IncidentLevel: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "zoomlens_incident_level", Help: "Current dwell-ladder level of each incident (0 = baseline).",
		}, []string{"incident", "alertname", "root", "entity"}),
		Escalations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "zoomlens_escalations_total", Help: "Dwell-ladder escalations, by target level.",
		}, []string{"level"}),
		BudgetBlocked: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "zoomlens_budget_blocked_total", Help: "Escalations held back by the series budget.",
		}),
		AgentsConnected: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "zoomlens_opamp_agents_connected", Help: "Collectors connected over OpAMP.",
		}),
		ConfigPushes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "zoomlens_config_pushes_total", Help: "Remote configs sent to collectors, by node and result.",
		}, []string{"node", "result"}),
		RemoteConfig: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "zoomlens_remote_config_status", Help: "Last remote config status reported by each agent (1 = current status).",
		}, []string{"node", "status"}),
		EBPFPods: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "zoomlens_ebpf_pods", Help: "ebpf_exporter pods currently managed by Zoom-Lens.",
		}),
		OverlayNodes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "zoomlens_overlay_nodes", Help: "Nodes that currently have a non-empty collection overlay.",
		}),
		ReconcileErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "zoomlens_reconcile_errors_total", Help: "Reconcile errors, by stage.",
		}, []string{"stage"}),
	}
	m.Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.AlertsReceived, m.AlertsSkipped, m.Incidents, m.IncidentLevel, m.Escalations, m.BudgetBlocked,
		m.AgentsConnected, m.ConfigPushes, m.RemoteConfig, m.EBPFPods, m.OverlayNodes, m.ReconcileErrors,
	)
	return m
}
