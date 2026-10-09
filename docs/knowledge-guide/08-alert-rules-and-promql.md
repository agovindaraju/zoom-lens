# 8. Alert rules and the PromQL they use

The alert rules are the **trigger** of every expansion. Each rule watches **one root**. When it
fires, Alertmanager delivers it to the operator, and the operator expands **that** root.

Files: [`knowledge/rules/<source>.rules.yml`](../../knowledge/rules). There are 14 files and 113
rules; hostmetrics and ebpf have none. These are standard Prometheus rule files.
`knowledge/generated/prometheusrule.yaml` wraps the same rules for the Prometheus Operator.

---

## 8.1 Anatomy of one rule

```yaml
groups:
  - name: zoomlens.node-exporter          # one group per source
    interval: 30s                         # roots are scraped every 30 s
    rules:
      - alert: ZLNodeMemoryHighUtilization
        expr: |-
          (1 - (max by (node) (node_memory_MemAvailable_bytes) / max by (node) (node_memory_MemTotal_bytes))) > 0.9
        #  └──────────────── value expression ─────────────────┘                          └ operator + threshold
        for: 15m                          # the condition must hold for 15 min
        labels:
          severity: warning
          zoomlens_source: node-exporter   # which pack
          zoomlens_root: node_memory_MemAvailable_bytes   # which root to expand
          zoomlens_entity: Node            # operator kind of the entity
        annotations:
          summary: "Memory on {{ $labels.node }} is {{ $value | humanizePercentage }} used"
          description: "Source: node-mixin NodeMemoryHighUtilization."
```

Things to notice:
- **`by (node)`** keeps the identity label. The alert that reaches the operator carries `node=…`,
  so the operator knows *which* node.
- **The companion** `node_memory_MemTotal_bytes` is in the expression. That is why it is collected
  at L0 together with the root.
- **The `description`** records where the rule comes from.

---

## 8.2 The label contract with the operator

The operator (`internal/alerts`) reads these labels from each alert:

| Label | Required | Used for |
|---|---|---|
| `zoomlens_root` | **yes**. Alerts without it are ignored and counted in `zoomlens_alerts_skipped_total` | The root whose tree is expanded |
| `zoomlens_entity` | recommended | `Pod` or `Node` today. If absent, it is inferred from the `pod` or `node` label. Other values (`APIServer`, `Deployment`, …) are skipped until the operator supports them |
| `pod`, `namespace` | for `Pod` | The pod. `node` is optional; the operator looks the node up |
| `node` | for `Node` | The node |
| `zoomlens_source` | yes, in the curated rules | Disambiguates roots that exist in several sources. **The operator does not read it today**: it matches roots by name and entity kind |
| `severity` | yes, in the curated rules | For humans and routing |

The validator (`hack/mkg`) checks that:
- every curated rule has all four `zoomlens_*`/`severity` labels;
- `(zoomlens_source, zoomlens_root)` is a real root;
- `zoomlens_entity` equals the operator kind of that pack's entity.

---

## 8.3 PromQL patterns used in the rules (with real examples)

### P1. Error ratio from counters (RED "errors")

```promql
sum by (node) (rate(apiserver_request_total{code=~"5.."}[5m]))
  / sum by (node) (rate(apiserver_request_total[5m]))                 > 0.05
```
- `rate(x[5m])` turns an ever-growing counter into a per-second rate over 5 minutes.
- Dividing two aggregated rates gives the **share of 5xx responses**.
- Also used for kubelet runtime errors, rest-client 5xx, etcd gRPC failures and CoreDNS SERVFAIL.

### P2. Utilisation from a mode-labelled counter

```promql
1 - avg by (node) (rate(node_cpu_seconds_total{mode=~"idle|iowait"}[5m]))    > 0.9
```
- `node_cpu_seconds_total` counts seconds per CPU and per mode.
- The rate of idle+iowait seconds is the idle share. One minus it is the busy share.

### P3. Gauge ratio with a companion

```promql
max by (node) (node_filefd_allocated / node_filefd_maximum)     > 0.7
```
- This is the reason companions exist.
- Also used for conntrack (entries / limit), memory, filesystem space and inodes, PVC space and
  inodes, and etcd DB / quota.

### P4. Latency percentile from a histogram

```promql
histogram_quantile(0.99,
  sum by (node, le) (rate(etcd_disk_wal_fsync_duration_seconds_bucket[5m])))   > 0.5
```
- The `_bucket` series carry an `le` ("less or equal") label.
- Aggregating the bucket **rates** and keeping `le` lets `histogram_quantile` estimate the p99.
- Also used for API server, kubelet PLEG and pod start, scheduler, kube-proxy, CoreDNS,
  admission webhooks and containerd.
- This is why histogram roots are listed by base name and scraped as `_bucket|_sum|_count`.

### P5. Rare events: `increase(...) > 0`

```promql
sum by (node, namespace, pod, container) (increase(container_oom_events_total{container!=""}[10m]))   > 0
```
- The rule fires when at least one event happened in the window.
- Also used for evictions, CoreDNS panics, node-controller evictions and pod restarts (`> 2`).

### P6. Throttling share: `increase` over `increase`

```promql
sum by (node, namespace, pod, container) (increase(container_cpu_cfs_throttled_periods_total{container!=""}[5m]))
  / sum by (node, namespace, pod, container) (increase(container_cpu_cfs_periods_total{container!=""}[5m]))   > 0.25
```
This is the kubernetes-mixin `CPUThrottlingHigh`: the share of CFS periods in which the container
was throttled.

### P7. Filtering inside an expression

```promql
max by (node, namespace, pod, container) (
  container_memory_working_set_bytes{container!="",image!=""}
    / (container_spec_memory_limit_bytes{container!="",image!=""} > 0))             > 0.9
```
- `(limit > 0)` keeps only containers **with** a limit. A comparison without `bool` filters
  series out rather than returning 0/1.
- The same trick is used for `node_network_speed_bytes > 0` (virtual NICs report 0) and
  `kube_resourcequota{type="hard"} > 0`.

### P8. Guards with `and` / `unless` (and operator precedence)

```promql
min by (node, device, mountpoint) (
  node_filesystem_avail_bytes{fstype!=""} / node_filesystem_size_bytes{fstype!=""}
    and node_filesystem_readonly{fstype!=""} == 0)                                  < 0.05
```
- Comparison operators bind tighter than `and`. This reads as
  `(avail/size) and (readonly == 0)`: keep the ratio only for read-write filesystems.
- Similar guards appear in the etcd fragmentation rule (only above 100 MiB in use), the PDB rule
  (only when `expected_pods > 0`) and the HPA "maxed out" rule (only when max ≠ min).

### P9. Joining two sources with `on (…)`

```promql
max by (node) (kubelet_running_pods) / on (node) max by (node) (kube_node_status_capacity{resource="pods"})   > 0.95
```
- `on (node)` matches series from **different jobs** (kubelet and kube-state-metrics) by the
  `node` label only.
- The KSM companion is collected at L0 by the `kube-state-metrics-cluster` pack. The validator
  accepts cross-source use as long as the metric is always-on somewhere.

### P10. `group_left` for many-to-one joins

```promql
sum(kube_pod_container_resource_requests{resource="cpu"}
    * on (namespace, pod) group_left () max by (namespace, pod) (kube_pod_status_phase{phase=~"Pending|Running"} == 1))
  / sum(kube_node_status_allocatable{resource="cpu"})                                 > 1
```
- Many containers (left) match one pod phase (right). `group_left` allows that.
- The phase filter counts only requests of pods that are pending or running.
- This is the kubernetes-mixin `KubeCPUOvercommit`, in its non-HA form.

### P11. State ("enum") gauges from kube-state-metrics

```promql
max by (namespace, pod) (kube_pod_status_phase{phase=~"Pending|Unknown|Failed"})     == 1
max by (node, condition) (kube_node_status_condition{condition=~"MemoryPressure|DiskPressure|PIDPressure",status="true"}) == 1
```
- KSM exposes one series per possible state, with value 1 for the current state and 0 for the
  others.

### P12. Arithmetic instead of logic

```promql
# NotReady and not cordoned: Ready(=1 when ready) + unschedulable(=1 when cordoned) == 0
min by (node) (kube_node_status_condition{condition="Ready",status="true"} + on (node) kube_node_spec_unschedulable)   == 0

# Running but not Ready: Running(1) - Ready(0) == 1
max by (namespace, pod) (kube_pod_status_phase{phase="Running"}) - min by (namespace, pod) (kube_pod_status_ready{condition="true"})   == 1

# Missing replicas: desired - available > 0
max by (namespace, deployment) (kube_deployment_spec_replicas - kube_deployment_status_replicas_available)   > 0
```
These are short ways to combine 0/1 gauges.

### P13. Ages from timestamp gauges

```promql
time() - max by (namespace, lease) (kube_lease_renew_time)                            > 60
time() - max by (namespace, job_name) (kube_job_status_start_time and on (namespace, job_name) kube_job_status_active > 0)   > 43200
```
Some metrics are Unix timestamps. `time() -` turns them into an age in seconds.

### P14. Flapping and "recently"

```promql
sum by (node) (changes(kube_node_status_condition{condition="Ready",status="true"}[15m]))   > 2
max by (namespace, pod, container) (max_over_time(kube_pod_container_status_waiting_reason{reason="CrashLoopBackOff"}[5m])) >= 1
max by (node) (abs(node_timex_offset_seconds))                                          > 0.05
```
- `changes()` counts value changes in the window, which detects flapping.
- `max_over_time` catches a state that was present at any point in the window.
- `abs` handles offsets in both directions.

---

## 8.4 Where the thresholds come from

| Origin (in `description`) | Rules | Meaning |
|---|---|---|
| node-mixin, kubernetes-mixin, etcd-mixin, coredns-mixin | 59 | The upstream expression and threshold, sometimes simplified (for example no `changes()` guard) or with a shorter `for:`. The description says so |
| USE / RED | 19 | The standard method where no mixin rule exists (container CPU near limit, disk busy, network saturation, error ratios) |
| operational | 20 | Common Kubernetes failure modes (namespace stuck terminating, lease not renewed, PVC not bound) |
| other | 15 | Kubernetes SLO/SLIs (API latency, network programming ≤ 30 s), component `/readyz` and healthz checks, APF and inflight saturation, exporter self-health, eviction early warning |

Six rules are marked **"tune"**, because the right value depends on the hardware:
- container disk writes above 50 MiB/s;
- workqueue depth above 100;
- API server inflight above 320;
- scheduling SLI above 30 s;
- Cilium drops above 10/s;
- CronJob success lag above 1 h.

**Demo override.** The upstream `for:` durations are 5 minutes to 1 hour, which is too slow for a
live demo. Step 03 of the deploy scripts rewrites every `for:` to `ZL_KNOWLEDGE_RULE_FOR` (default
`1m`) when it applies `prometheusrule.yaml`. Set it to empty to keep the upstream values.

**Demo rules** (`deploy/monitoring/alert-rules.yaml`, not part of `knowledge/`) use `[2m]` rate
windows and `keep_firing_for: 1m`. The collector restarts briefly when it applies an overlay, and
these settings stop that short gap from resolving the alert.

---

## 8.5 Testing a rule

```bash
# Syntax and semantics, exactly as Prometheus loads it (needs promtool):
promtool check rules knowledge/rules/*.rules.yml

# The repository's own check (also verifies labels and that every metric used is always-on):
make knowledge

# Is it loaded and healthy in the cluster?
./scripts/promq.sh 'ALERTS{alertname=~"ZL.*"}'
kubectl -n monitoring port-forward svc/prometheus-operated 19091:9090 &
curl -s localhost:19091/api/v1/rules | head -c 2000

# What are its inputs right now? (example: the CPU rule)
./scripts/promq.sh '1 - avg by (node) (rate(node_cpu_seconds_total{mode=~"idle|iowait"}[5m]))'
```

For behaviour tests (feed samples, expect alerts), use Prometheus's rule unit tests:
https://prometheus.io/docs/prometheus/latest/configuration/unit_testing_rules/

---

## 8.6 All 113 rules

Generated from `knowledge/rules/*.rules.yml`. "Fires when" is the full expression, with the
operator and threshold at the end.

#### cadvisor

| Alert | Root (`zoomlens_root`) | Entity | `for` | Severity | Fires when | Origin |
|---|---|---|---|---|---|---|
| `ZLContainerCPUNearLimit` | `container_cpu_usage_seconds_total` | Pod | 15m | warning | `(sum by (node, namespace, pod, container) (rate(container_cpu_usage_seconds_total{container!="",image!=""}[5m])) / sum by (node, namespace, pod, container) (container_spec_cpu_quota{container!="",image!=""} / container_spec_cpu_period{container!="",image!=""})) > 0.9` | USE utilisation vs CPU limit |
| `ZLCPUThrottlingHigh` | `container_cpu_cfs_throttled_periods_total` | Pod | 15m | info | `(sum by (node, namespace, pod, container) (increase(container_cpu_cfs_throttled_periods_total{container!=""}[5m])) / sum by (node, namespace, pod, container) (increase(container_cpu_cfs_periods_total{container!=""}[5m]))) > 0.25` | kubernetes-mixin CPUThrottlingHigh |
| `ZLContainerMemoryNearLimit` | `container_memory_working_set_bytes` | Pod | 15m | warning | `(max by (node, namespace, pod, container) (container_memory_working_set_bytes{container!="",image!=""} / (container_spec_memory_limit_bytes{container!="",image!=""} > 0))) > 0.9` | USE saturation vs memory limit |
| `ZLContainerOOMEvents` | `container_oom_events_total` | Pod | — | warning | `(sum by (node, namespace, pod, container) (increase(container_oom_events_total{container!=""}[10m]))) > 0` | USE errors |
| `ZLContainerHeavyDiskWrites` | `container_fs_writes_bytes_total` | Pod | 10m | info | `(sum by (node, namespace, pod) (rate(container_fs_writes_bytes_total{container!=""}[5m]))) > 52428800` | USE utilisation (tune per cluster) |
| `ZLPodNetworkDrops` | `container_network_receive_packets_dropped_total` | Pod | 10m | warning | `(sum by (node, namespace, pod) (rate(container_network_receive_packets_dropped_total[5m]) + rate(container_network_transmit_packets_dropped_total[5m]))) > 1` | USE errors |

#### calico

| Alert | Root (`zoomlens_root`) | Entity | `for` | Severity | Fires when | Origin |
|---|---|---|---|---|---|---|
| `ZLCalicoDataplaneFailures` | `felix_int_dataplane_failures` | Node | 10m | warning | `(sum by (node) (rate(felix_int_dataplane_failures[5m]))) > 0` | RED errors |
| `ZLCalicoNotInSync` | `felix_resync_state` | Node | 10m | warning | `(max by (node) (felix_resync_state)) != 3` | operational (3 = in sync) |

#### cilium

| Alert | Root (`zoomlens_root`) | Entity | `for` | Severity | Fires when | Origin |
|---|---|---|---|---|---|---|
| `ZLCiliumDrops` | `cilium_drop_count_total` | Node | 10m | warning | `(sum by (node, reason) (rate(cilium_drop_count_total[5m]))) > 10` | USE errors (tune) |
| `ZLCiliumBPFMapPressure` | `cilium_bpf_map_pressure` | Node | 10m | warning | `(max by (node, map_name) (cilium_bpf_map_pressure)) > 0.9` | USE saturation |
| `ZLCiliumUnreachableNodes` | `cilium_unreachable_nodes` | Node | 10m | warning | `(max by (node) (cilium_unreachable_nodes)) > 0` | operational |
| `ZLCiliumEndpointRegenerationFailures` | `cilium_endpoint_regenerations_total` | Node | 10m | warning | `(sum by (node) (rate(cilium_endpoint_regenerations_total{outcome="fail"}[5m]))) > 0` | RED errors |

#### containerd

| Alert | Root (`zoomlens_root`) | Entity | `for` | Severity | Fires when | Origin |
|---|---|---|---|---|---|---|
| `ZLContainerdPodNetworkSetupSlow` | `containerd_cri_sandbox_create_network_seconds` | Node | 10m | warning | `(histogram_quantile(0.99, sum by (node, le) (rate(containerd_cri_sandbox_create_network_seconds_bucket[5m])))) > 10` | operational (metric name UNVERIFIED) |

#### coredns

| Alert | Root (`zoomlens_root`) | Entity | `for` | Severity | Fires when | Origin |
|---|---|---|---|---|---|---|
| `ZLCoreDNSLatencyHigh` | `coredns_dns_request_duration_seconds` | Pod | 10m | warning | `(histogram_quantile(0.99, sum by (namespace, pod, server, zone, le) (rate(coredns_dns_request_duration_seconds_bucket[5m])))) > 1` | coredns-mixin CoreDNSLatencyHigh (mixin critical = 4s; lower here to zoom in earlier) |
| `ZLCoreDNSErrorsHigh` | `coredns_dns_responses_total` | Pod | 10m | warning | `(sum by (namespace, pod) (rate(coredns_dns_responses_total{rcode="SERVFAIL"}[5m])) / sum by (namespace, pod) (rate(coredns_dns_responses_total[5m]))) > 0.01` | coredns-mixin CoreDNSErrorsHigh (warning) |
| `ZLCoreDNSForwardHealthcheckBroken` | `coredns_forward_healthcheck_broken_total` | Pod | 10m | warning | `(sum by (namespace, pod) (rate(coredns_forward_healthcheck_broken_total[5m]))) > 0` | coredns-mixin CoreDNSForwardHealthcheckBrokenCount |
| `ZLCoreDNSOverloaded` | `coredns_health_request_duration_seconds` | Pod | 5m | warning | `(histogram_quantile(0.99, sum by (namespace, pod, le) (rate(coredns_health_request_duration_seconds_bucket[5m])))) > 1` | health plugin overload signal |
| `ZLCoreDNSPanics` | `coredns_panics_total` | Pod | — | warning | `(sum by (namespace, pod) (increase(coredns_panics_total[10m]))) > 0` | RED errors |

#### csi

| Alert | Root (`zoomlens_root`) | Entity | `for` | Severity | Fires when | Origin |
|---|---|---|---|---|---|---|
| `ZLCSIOperationErrors` | `csi_sidecar_operations_seconds` | CSIDriver | 10m | warning | `(sum by (driver_name, method_name) (rate(csi_sidecar_operations_seconds_count{grpc_status_code!="OK"}[5m]))) > 0` | RED errors |

#### etcd

| Alert | Root (`zoomlens_root`) | Entity | `for` | Severity | Fires when | Origin |
|---|---|---|---|---|---|---|
| `ZLEtcdNoLeader` | `etcd_server_has_leader` | EtcdMember | 1m | critical | `(min by (node) (etcd_server_has_leader)) == 0` | etcd-mixin etcdNoLeader |
| `ZLEtcdHighNumberOfLeaderChanges` | `etcd_server_leader_changes_seen_total` | EtcdMember | 5m | warning | `(max by (node) (increase(etcd_server_leader_changes_seen_total[15m]))) >= 4` | etcd-mixin etcdHighNumberOfLeaderChanges |
| `ZLEtcdHighNumberOfFailedProposals` | `etcd_server_proposals_failed_total` | EtcdMember | 15m | warning | `(max by (node) (rate(etcd_server_proposals_failed_total[15m]))) > 5` | etcd-mixin etcdHighNumberOfFailedProposals |
| `ZLEtcdHighFsyncDurations` | `etcd_disk_wal_fsync_duration_seconds` | EtcdMember | 10m | warning | `(histogram_quantile(0.99, sum by (node, le) (rate(etcd_disk_wal_fsync_duration_seconds_bucket[5m])))) > 0.5` | etcd-mixin etcdHighFsyncDurations |
| `ZLEtcdHighCommitDurations` | `etcd_disk_backend_commit_duration_seconds` | EtcdMember | 10m | warning | `(histogram_quantile(0.99, sum by (node, le) (rate(etcd_disk_backend_commit_duration_seconds_bucket[5m])))) > 0.25` | etcd-mixin etcdHighCommitDurations |
| `ZLEtcdMemberCommunicationSlow` | `etcd_network_peer_round_trip_time_seconds` | EtcdMember | 10m | warning | `(histogram_quantile(0.99, sum by (node, To, le) (rate(etcd_network_peer_round_trip_time_seconds_bucket[5m])))) > 0.15` | etcd-mixin etcdMemberCommunicationSlow |
| `ZLEtcdDatabaseQuotaLowSpace` | `etcd_mvcc_db_total_size_in_bytes` | EtcdMember | 10m | critical | `(max by (node) (etcd_mvcc_db_total_size_in_bytes / etcd_server_quota_backend_bytes)) > 0.95` | etcd-mixin etcdDatabaseQuotaLowSpace |
| `ZLEtcdDatabaseHighFragmentationRatio` | `etcd_mvcc_db_total_size_in_bytes` | EtcdMember | 10m | warning | `(max by (node) ((etcd_mvcc_db_total_size_in_use_in_bytes / etcd_mvcc_db_total_size_in_bytes) and etcd_mvcc_db_total_size_in_use_in_bytes > 104857600)) < 0.5` | etcd-mixin etcdDatabaseHighFragmentationRatio |
| `ZLEtcdHighNumberOfFailedGRPCRequests` | `grpc_server_handled_total` | EtcdMember | 10m | warning | `(sum by (node, grpc_service, grpc_method) (rate(grpc_server_handled_total{grpc_code=~"Unknown\|FailedPrecondition\|ResourceExhausted\|Internal\|Unavailable\|DataLoss\|DeadlineExceeded"}[5m])) / sum by (node, grpc_service, grpc_method) (rate(grpc_server_handled_total[5m]))) > 0.01` | etcd-mixin etcdHighNumberOfFailedGRPCRequests |

#### kube-apiserver

| Alert | Root (`zoomlens_root`) | Entity | `for` | Severity | Fires when | Origin |
|---|---|---|---|---|---|---|
| `ZLKubeAPIErrorsHigh` | `apiserver_request_total` | APIServer | 10m | warning | `(sum by (node) (rate(apiserver_request_total{code=~"5.."}[5m])) / sum by (node) (rate(apiserver_request_total[5m]))) > 0.05` | kubernetes-mixin KubeAPIErrorBudgetBurn (simplified to a ratio) |
| `ZLKubeAPILatencyHigh` | `apiserver_request_sli_duration_seconds` | APIServer | 10m | warning | `(histogram_quantile(0.99, sum by (node, verb, le) (rate(apiserver_request_sli_duration_seconds_bucket{verb!~"WATCH\|WATCHLIST\|CONNECT"}[5m])))) > 1` | SLO: non-streaming request latency |
| `ZLKubeAPIPriorityAndFairnessRejecting` | `apiserver_flowcontrol_rejected_requests_total` | APIServer | 5m | warning | `(sum by (node, priority_level) (rate(apiserver_flowcontrol_rejected_requests_total[5m]))) > 0` | APF saturation |
| `ZLKubeAPIInflightHigh` | `apiserver_current_inflight_requests` | APIServer | 5m | info | `(max by (node, request_kind) (apiserver_current_inflight_requests)) > 320` | saturation (80% of the default --max-requests-inflight=400; tune) |
| `ZLKubeAPIEtcdLatencyHigh` | `etcd_request_duration_seconds` | APIServer | 10m | warning | `(histogram_quantile(0.99, sum by (node, operation, le) (rate(etcd_request_duration_seconds_bucket[5m])))) > 0.5` | storage latency seen by the apiserver |
| `ZLKubeAdmissionWebhookSlow` | `apiserver_admission_webhook_admission_duration_seconds` | APIServer | 10m | warning | `(histogram_quantile(0.99, sum by (node, name, le) (rate(apiserver_admission_webhook_admission_duration_seconds_bucket[5m])))) > 1` | operational |
| `ZLKubeAggregatedAPIDown` | `aggregator_unavailable_apiservice` | APIServer | 10m | warning | `(max by (name) (aggregator_unavailable_apiservice)) == 1` | kubernetes-mixin KubeAggregatedAPIDown |
| `ZLKubeClientCertificateExpiration` | `apiserver_client_certificate_expiration_seconds` | APIServer | 5m | warning | `(histogram_quantile(0.01, sum by (node, le) (rate(apiserver_client_certificate_expiration_seconds_bucket[5m])))) < 604800` | kubernetes-mixin KubeClientCertificateExpiration |
| `ZLKubeAPIServerHealthcheckFailing` | `kubernetes_healthcheck` | APIServer | 5m | critical | `(min by (node, name) (kubernetes_healthcheck{type="readyz"})) == 0` | component /readyz check |

#### kube-controller-manager

| Alert | Root (`zoomlens_root`) | Entity | `for` | Severity | Fires when | Origin |
|---|---|---|---|---|---|---|
| `ZLControllerWorkqueueBacklog` | `workqueue_depth` | ControllerManager | 15m | warning | `(max by (node, name) (workqueue_depth)) > 100` | saturation (tune) |
| `ZLUnhealthyNodesInZone` | `node_collector_unhealthy_nodes_in_zone` | ControllerManager | 5m | warning | `(max by (zone) (node_collector_unhealthy_nodes_in_zone)) > 0` | operational |
| `ZLNodeControllerEvicting` | `node_collector_evictions_total` | ControllerManager | — | warning | `(sum by (zone) (increase(node_collector_evictions_total[15m]))) > 0` | operational |
| `ZLVolumeOperationErrors` | `volume_operation_errors_total` | ControllerManager | 10m | warning | `(sum by (operation_name, plugin_name) (rate(volume_operation_errors_total[5m]))) > 0` | RED errors |
| `ZLControllerManagerNoLeader` | `leader_election_master_status` | ControllerManager | 5m | critical | `(sum(leader_election_master_status{name="kube-controller-manager"})) == 0` | operational |
| `ZLControllerManagerHealthcheckFailing` | `kubernetes_healthcheck` | ControllerManager | 5m | warning | `(min by (node, name) (kubernetes_healthcheck)) == 0` | component health check |
| `ZLControllerManagerClientErrors` | `rest_client_requests_total` | ControllerManager | 15m | warning | `(sum by (node) (rate(rest_client_requests_total{code=~"5.."}[5m])) / sum by (node) (rate(rest_client_requests_total[5m]))) > 0.01` | kubernetes-mixin KubeClientErrors |

#### kube-proxy

| Alert | Root (`zoomlens_root`) | Entity | `for` | Severity | Fires when | Origin |
|---|---|---|---|---|---|---|
| `ZLKubeProxySyncSlow` | `kubeproxy_sync_proxy_rules_duration_seconds` | Node | 15m | warning | `(histogram_quantile(0.99, sum by (node, le) (rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[5m])))) > 1` | operational |
| `ZLKubeProxyNetworkProgrammingSlow` | `kubeproxy_network_programming_duration_seconds` | Node | 15m | warning | `(histogram_quantile(0.99, sum by (node, le) (rate(kubeproxy_network_programming_duration_seconds_bucket[5m])))) > 30` | Kubernetes SLO: in-cluster network programming latency p99 <= 30s |
| `ZLKubeProxyRulesStale` | `kubeproxy_sync_proxy_rules_last_timestamp_seconds` | Node | 5m | warning | `(max by (node) (kubeproxy_sync_proxy_rules_last_queued_timestamp_seconds - kubeproxy_sync_proxy_rules_last_timestamp_seconds)) > 60` | kube-proxy healthz logic |
| `ZLKubeProxyIptablesRestoreFailures` | `kubeproxy_sync_proxy_rules_iptables_restore_failures_total` | Node | 5m | warning | `(sum by (node) (rate(kubeproxy_sync_proxy_rules_iptables_restore_failures_total[5m]))) > 0` | RED errors |
| `ZLKubeProxyNftablesSyncFailures` | `kubeproxy_sync_proxy_rules_nftables_sync_failures_total` | Node | 5m | warning | `(sum by (node) (rate(kubeproxy_sync_proxy_rules_nftables_sync_failures_total[5m]))) > 0` | RED errors |
| `ZLKubeProxyUnhealthy` | `kubeproxy_proxy_healthz_total` | Node | 5m | warning | `(sum by (node) (rate(kubeproxy_proxy_healthz_total{code!="200"}[5m]))) > 0` | component health check |
| `ZLKubeProxyClientErrors` | `rest_client_requests_total` | Node | 15m | warning | `(sum by (node) (rate(rest_client_requests_total{code=~"5.."}[5m])) / sum by (node) (rate(rest_client_requests_total[5m]))) > 0.01` | kubernetes-mixin KubeClientErrors |

#### kube-scheduler

| Alert | Root (`zoomlens_root`) | Entity | `for` | Severity | Fires when | Origin |
|---|---|---|---|---|---|---|
| `ZLSchedulerUnschedulablePods` | `scheduler_pending_pods` | Scheduler | 30m | warning | `(sum by (node) (scheduler_pending_pods{queue="unschedulable"})) > 0` | operational |
| `ZLSchedulerErrors` | `scheduler_schedule_attempts_total` | Scheduler | 10m | warning | `(sum by (node) (rate(scheduler_schedule_attempts_total{result="error"}[5m]))) > 0` | RED errors |
| `ZLSchedulerLatencyHigh` | `scheduler_pod_scheduling_sli_duration_seconds` | Scheduler | 15m | warning | `(histogram_quantile(0.99, sum by (node, le) (rate(scheduler_pod_scheduling_sli_duration_seconds_bucket[5m])))) > 30` | SLI (tune) |
| `ZLSchedulerNoLeader` | `leader_election_master_status` | Scheduler | 5m | critical | `(sum(leader_election_master_status{name="kube-scheduler"})) == 0` | operational |
| `ZLSchedulerHealthcheckFailing` | `kubernetes_healthcheck` | Scheduler | 5m | warning | `(min by (node, name) (kubernetes_healthcheck)) == 0` | component health check |
| `ZLSchedulerClientErrors` | `rest_client_requests_total` | Scheduler | 15m | warning | `(sum by (node) (rate(rest_client_requests_total{code=~"5.."}[5m])) / sum by (node) (rate(rest_client_requests_total[5m]))) > 0.01` | kubernetes-mixin KubeClientErrors |

#### kube-state-metrics

| Alert | Root (`zoomlens_root`) | Entity | `for` | Severity | Fires when | Origin |
|---|---|---|---|---|---|---|
| `ZLKubeCPUOvercommit` | `kube_node_status_allocatable` | Cluster | 10m | warning | `(sum(kube_pod_container_resource_requests{resource="cpu"} * on (namespace, pod) group_left () max by (namespace, pod) (kube_pod_status_phase{phase=~"Pending\|Running"} == 1)) / sum(kube_node_status_allocatable{resource="cpu"})) > 1` | kubernetes-mixin KubeCPUOvercommit (non-HA form) |
| `ZLKubeMemoryOvercommit` | `kube_node_status_allocatable` | Cluster | 10m | warning | `(sum(kube_pod_container_resource_requests{resource="memory"} * on (namespace, pod) group_left () max by (namespace, pod) (kube_pod_status_phase{phase=~"Pending\|Running"} == 1)) / sum(kube_node_status_allocatable{resource="memory"})) > 1` | kubernetes-mixin KubeMemoryOvercommit (non-HA form) |
| `ZLKubeNodeNotReady` | `kube_node_status_condition` | Node | 15m | warning | `(min by (node) (kube_node_status_condition{condition="Ready",status="true"} + on (node) kube_node_spec_unschedulable)) == 0` | kubernetes-mixin KubeNodeNotReady (value 0 = not Ready and not cordoned) |
| `ZLKubeNodePressure` | `kube_node_status_condition` | Node | 10m | info | `(max by (node, condition) (kube_node_status_condition{condition=~"MemoryPressure\|DiskPressure\|PIDPressure",status="true"})) == 1` | kubernetes-mixin KubeNodePressure |
| `ZLKubeNodeReadinessFlapping` | `kube_node_status_condition` | Node | 15m | warning | `(sum by (node) (changes(kube_node_status_condition{condition="Ready",status="true"}[15m]))) > 2` | kubernetes-mixin KubeNodeReadinessFlapping |
| `ZLKubeNodeUnreachable` | `kube_node_spec_taint` | Node | 15m | warning | `(max by (node) (kube_node_spec_taint{key="node.kubernetes.io/unreachable",effect="NoSchedule"})) == 1` | kubernetes-mixin KubeNodeUnreachable |
| `ZLKubeQuotaAlmostFull` | `kube_resourcequota` | Namespace | 15m | info | `(max by (namespace, resourcequota, resource) (kube_resourcequota{type="used"} / on (namespace, resourcequota, resource) (kube_resourcequota{type="hard"} > 0))) > 0.9` | kubernetes-mixin KubeQuotaAlmostFull |
| `ZLKubeNamespaceStuckTerminating` | `kube_namespace_status_phase` | Namespace | 1h | warning | `(max by (namespace) (kube_namespace_status_phase{phase="Terminating"})) == 1` | operational |
| `ZLKubePodNotRunning` | `kube_pod_status_phase` | Pod | 15m | warning | `(max by (namespace, pod) (kube_pod_status_phase{phase=~"Pending\|Unknown\|Failed"})) == 1` | kubernetes-mixin KubePodNotReady (phase part) |
| `ZLKubePodNotReady` | `kube_pod_status_ready` | Pod | 15m | warning | `(max by (namespace, pod) (kube_pod_status_phase{phase="Running"}) - min by (namespace, pod) (kube_pod_status_ready{condition="true"})) == 1` | kubernetes-mixin KubePodNotReady (ready part; 1 = Running but not Ready) |
| `ZLKubePodRestarting` | `kube_pod_container_status_restarts_total` | Pod | — | warning | `(max by (namespace, pod, container) (increase(kube_pod_container_status_restarts_total[15m]))) > 2` | operational |
| `ZLKubePodCrashLooping` | `kube_pod_container_status_waiting_reason` | Pod | 15m | warning | `(max by (namespace, pod, container) (max_over_time(kube_pod_container_status_waiting_reason{reason="CrashLoopBackOff"}[5m]))) >= 1` | kubernetes-mixin KubePodCrashLooping |
| `ZLKubeContainerWaiting` | `kube_pod_container_status_waiting_reason` | Pod | 1h | warning | `(max by (namespace, pod, container, reason) (kube_pod_container_status_waiting_reason{reason!="CrashLoopBackOff"})) > 0` | kubernetes-mixin KubeContainerWaiting |
| `ZLKubeDeploymentReplicasMismatch` | `kube_deployment_status_replicas_available` | Deployment | 15m | warning | `(max by (namespace, deployment) (kube_deployment_spec_replicas - kube_deployment_status_replicas_available)) > 0` | kubernetes-mixin KubeDeploymentReplicasMismatch (without the changes() guard) |
| `ZLKubeReplicaSetReplicasMismatch` | `kube_replicaset_status_ready_replicas` | ReplicaSet | 15m | info | `(max by (namespace, replicaset) (kube_replicaset_spec_replicas - kube_replicaset_status_ready_replicas)) > 0` | operational |
| `ZLKubeStatefulSetReplicasMismatch` | `kube_statefulset_status_replicas_ready` | StatefulSet | 15m | warning | `(max by (namespace, statefulset) (kube_statefulset_replicas - kube_statefulset_status_replicas_ready)) > 0` | kubernetes-mixin KubeStatefulSetReplicasMismatch (without the changes() guard) |
| `ZLKubeDaemonSetNotAvailable` | `kube_daemonset_status_number_available` | DaemonSet | 15m | warning | `(max by (namespace, daemonset) (kube_daemonset_status_desired_number_scheduled - kube_daemonset_status_number_available)) > 0` | kubernetes-mixin KubeDaemonSetRolloutStuck (availability part) |
| `ZLKubeReplicationControllerMismatch` | `kube_replicationcontroller_status_available_replicas` | ReplicationController | 15m | info | `(max by (namespace, replicationcontroller) (kube_replicationcontroller_spec_replicas - kube_replicationcontroller_status_available_replicas)) > 0` | operational |
| `ZLKubeJobFailed` | `kube_job_failed` | Job | 15m | warning | `(max by (namespace, job_name) (kube_job_failed{condition="true"})) == 1` | kubernetes-mixin KubeJobFailed |
| `ZLKubeJobNotCompleted` | `kube_job_status_active` | Job | — | warning | `(time() - max by (namespace, job_name) (kube_job_status_start_time and on (namespace, job_name) kube_job_status_active > 0)) > 43200` | kubernetes-mixin KubeJobNotCompleted (kubeJobTimeoutDuration 12h) |
| `ZLKubeCronJobFailing` | `kube_cronjob_status_last_successful_time` | CronJob | 1h | warning | `(max by (namespace, cronjob) (kube_cronjob_status_last_schedule_time - kube_cronjob_status_last_successful_time)) > 3600` | operational (tune to the schedule period) |
| `ZLKubeHpaMaxedOut` | `kube_horizontalpodautoscaler_status_current_replicas` | HorizontalPodAutoscaler | 15m | warning | `(max by (namespace, horizontalpodautoscaler) ((kube_horizontalpodautoscaler_status_current_replicas == kube_horizontalpodautoscaler_spec_max_replicas) and on (namespace, horizontalpodautoscaler) (kube_horizontalpodautoscaler_spec_max_replicas != kube_horizontalpodautoscaler_spec_min_replicas))) > 0` | kubernetes-mixin KubeHpaMaxedOut |
| `ZLKubeHpaReplicasMismatch` | `kube_horizontalpodautoscaler_status_current_replicas` | HorizontalPodAutoscaler | 15m | warning | `(max by (namespace, horizontalpodautoscaler) (abs(kube_horizontalpodautoscaler_status_desired_replicas - kube_horizontalpodautoscaler_status_current_replicas))) > 0` | kubernetes-mixin KubeHpaReplicasMismatch (simplified) |
| `ZLKubePdbNotEnoughHealthyPods` | `kube_poddisruptionbudget_status_current_healthy` | PodDisruptionBudget | 15m | warning | `(max by (namespace, poddisruptionbudget) ((kube_poddisruptionbudget_status_desired_healthy - kube_poddisruptionbudget_status_current_healthy) and on (namespace, poddisruptionbudget) kube_poddisruptionbudget_status_expected_pods > 0)) > 0` | kubernetes-mixin KubePdbNotEnoughHealthyPods |
| `ZLKubePersistentVolumeClaimNotBound` | `kube_persistentvolumeclaim_status_phase` | PersistentVolumeClaim | 15m | warning | `(max by (namespace, persistentvolumeclaim) (kube_persistentvolumeclaim_status_phase{phase=~"Pending\|Lost"})) == 1` | operational |
| `ZLKubePersistentVolumeErrors` | `kube_persistentvolume_status_phase` | PersistentVolume | 5m | critical | `(max by (persistentvolume) (kube_persistentvolume_status_phase{phase=~"Failed\|Pending"})) == 1` | kubernetes-mixin KubePersistentVolumeErrors |
| `ZLKubeVolumeNotAttached` | `kube_volumeattachment_status_attached` | VolumeAttachment | 15m | warning | `(min by (volumeattachment) (kube_volumeattachment_status_attached)) == 0` | operational |
| `ZLKubeEndpointsNotReady` | `kube_endpointslice_endpoints` | Service | 10m | warning | `(count by (namespace, endpointslice) (kube_endpointslice_endpoints{ready="false"}) / count by (namespace, endpointslice) (kube_endpointslice_endpoints)) > 0.5` | operational (fires only when at least one endpoint is not ready) |
| `ZLKubeLeaseNotRenewed` | `kube_lease_renew_time` | Lease | 2m | warning | `(time() - max by (namespace, lease) (kube_lease_renew_time)) > 60` | operational (node heartbeat: kube-node-lease) |
| `ZLKubeCSRDeniedOrFailed` | `kube_certificatesigningrequest_condition` | CertificateSigningRequest | 5m | info | `(max by (certificatesigningrequest, signer_name) (kube_certificatesigningrequest_condition{condition=~"denied\|failed"})) == 1` | operational |

#### kubelet

| Alert | Root (`zoomlens_root`) | Entity | `for` | Severity | Fires when | Origin |
|---|---|---|---|---|---|---|
| `ZLKubeletPlegDurationHigh` | `kubelet_pleg_relist_duration_seconds` | Node | 5m | warning | `(histogram_quantile(0.99, sum by (node, le) (rate(kubelet_pleg_relist_duration_seconds_bucket[5m])))) >= 10` | kubernetes-mixin KubeletPlegDurationHigh |
| `ZLKubeletRuntimeErrors` | `kubelet_runtime_operations_errors_total` | Node | 10m | warning | `(sum by (node) (rate(kubelet_runtime_operations_errors_total[5m])) / sum by (node) (rate(kubelet_runtime_operations_total[5m]))) > 0.05` | RED errors |
| `ZLKubeletPodStartUpLatencyHigh` | `kubelet_pod_start_sli_duration_seconds` | Node | 15m | warning | `(histogram_quantile(0.99, sum by (node, le) (rate(kubelet_pod_start_sli_duration_seconds_bucket[5m])))) > 60` | kubernetes-mixin KubeletPodStartUpLatencyHigh (mixin uses pod_worker_duration) |
| `ZLKubeNodeEviction` | `kubelet_evictions` | Node | — | info | `(sum by (node, eviction_signal) (increase(kubelet_evictions[15m]))) > 0` | kubernetes-mixin KubeNodeEviction |
| `ZLKubeletTooManyPods` | `kubelet_running_pods` | Node | 15m | info | `(max by (node) (kubelet_running_pods) / on (node) max by (node) (kube_node_status_capacity{resource="pods"})) > 0.95` | kubernetes-mixin KubeletTooManyPods |
| `ZLKubeletClientCertificateExpiration` | `kubelet_certificate_manager_client_ttl_seconds` | Node | — | warning | `(min by (node) (kubelet_certificate_manager_client_ttl_seconds)) < 604800` | kubernetes-mixin KubeletClientCertificateExpiration |
| `ZLKubeletNodeMemoryNearCapacity` | `node_memory_working_set_bytes` | Node | 10m | warning | `(max by (node) (node_memory_working_set_bytes) / on (node) max by (node) (kube_node_status_capacity{resource="memory"})) > 0.9` | eviction early-warning |
| `ZLKubeletStorageOperationErrors` | `storage_operation_duration_seconds` | Node | 10m | warning | `(sum by (node, operation_name, volume_plugin) (rate(storage_operation_duration_seconds_count{status!="success"}[5m]))) > 0` | RED errors |
| `ZLKubeletClientErrors` | `rest_client_requests_total` | Node | 15m | warning | `(sum by (node) (rate(rest_client_requests_total{code=~"5.."}[5m])) / sum by (node) (rate(rest_client_requests_total[5m]))) > 0.01` | kubernetes-mixin KubeClientErrors |
| `ZLPodProbeFailures` | `prober_probe_total` | Pod | 5m | warning | `(sum by (node, namespace, pod, container, probe_type) (rate(prober_probe_total{result="failed"}[5m]))) > 0` | RED errors |
| `ZLKubePersistentVolumeFillingUp` | `kubelet_volume_stats_available_bytes` | PersistentVolumeClaim | 1m | critical | `(min by (namespace, persistentvolumeclaim) (kubelet_volume_stats_available_bytes / kubelet_volume_stats_capacity_bytes)) < 0.03` | kubernetes-mixin KubePersistentVolumeFillingUp (critical) |
| `ZLKubePersistentVolumeInodesFillingUp` | `kubelet_volume_stats_inodes_free` | PersistentVolumeClaim | 1m | critical | `(min by (namespace, persistentvolumeclaim) (kubelet_volume_stats_inodes_free / kubelet_volume_stats_inodes)) < 0.03` | kubernetes-mixin KubePersistentVolumeInodesFillingUp (critical) |

#### node-exporter

| Alert | Root (`zoomlens_root`) | Entity | `for` | Severity | Fires when | Origin |
|---|---|---|---|---|---|---|
| `ZLNodeCPUHighUsage` | `node_cpu_seconds_total` | Node | 15m | warning | `(1 - avg by (node) (rate(node_cpu_seconds_total{mode=~"idle\|iowait"}[5m]))) > 0.9` | node-mixin NodeCPUHighUsage |
| `ZLNodeSystemSaturation` | `node_load1` | Node | 15m | warning | `(max by (node) (node_load1) / count by (node) (node_cpu_seconds_total{mode="idle"})) > 2` | node-mixin NodeSystemSaturation |
| `ZLNodeMemoryHighUtilization` | `node_memory_MemAvailable_bytes` | Node | 15m | warning | `(1 - (max by (node) (node_memory_MemAvailable_bytes) / max by (node) (node_memory_MemTotal_bytes))) > 0.9` | node-mixin NodeMemoryHighUtilization |
| `ZLNodeDiskBusy` | `node_disk_io_time_seconds_total` | Node | 10m | warning | `(max by (node, device) (rate(node_disk_io_time_seconds_total[5m]))) > 0.9` | USE utilisation (node-mixin uses io_time_weighted > 10 for 30m) |
| `ZLNodeFilesystemAlmostOutOfSpace` | `node_filesystem_avail_bytes` | Node | 30m | warning | `(min by (node, device, mountpoint) (node_filesystem_avail_bytes{fstype!=""} / node_filesystem_size_bytes{fstype!=""} and node_filesystem_readonly{fstype!=""} == 0)) < 0.05` | node-mixin NodeFilesystemAlmostOutOfSpace |
| `ZLNodeFilesystemAlmostOutOfFiles` | `node_filesystem_files_free` | Node | 1h | warning | `(min by (node, device, mountpoint) (node_filesystem_files_free{fstype!=""} / node_filesystem_files{fstype!=""})) < 0.05` | node-mixin NodeFilesystemAlmostOutOfFiles |
| `ZLNodeNetworkSaturated` | `node_network_receive_bytes_total` | Node | 15m | warning | `(max by (node, device) (rate(node_network_receive_bytes_total{device!~"lo\|veth.*\|cali.*\|cilium.*\|lxc.*\|flannel.*\|cni.*\|docker.*\|br-.*\|vxlan.*\|tunl.*\|genev.*"}[5m]) / (node_network_speed_bytes > 0))) > 0.9` | USE utilisation |
| `ZLNodeNetworkReceiveErrs` | `node_network_receive_errs_total` | Node | 10m | warning | `(max by (node, device) (rate(node_network_receive_errs_total[5m]) / rate(node_network_receive_packets_total[5m]))) > 0.01` | node-mixin NodeNetworkReceiveErrs (mixin for: 1h) |
| `ZLNodeHighNumberConntrackEntriesUsed` | `node_nf_conntrack_entries` | Node | 5m | warning | `(max by (node) (node_nf_conntrack_entries / node_nf_conntrack_entries_limit)) > 0.75` | node-mixin NodeHighNumberConntrackEntriesUsed |
| `ZLNodeFileDescriptorLimit` | `node_filefd_allocated` | Node | 15m | warning | `(max by (node) (node_filefd_allocated / node_filefd_maximum)) > 0.7` | node-mixin NodeFileDescriptorLimit |
| `ZLNodeClockSkewDetected` | `node_timex_offset_seconds` | Node | 10m | warning | `(max by (node) (abs(node_timex_offset_seconds))) > 0.05` | node-mixin NodeClockSkewDetected |
| `ZLNodeClockNotSynchronising` | `node_timex_offset_seconds` | Node | 10m | warning | `(min by (node) (min_over_time(node_timex_sync_status[5m]))) == 0` | node-mixin NodeClockNotSynchronising |
| `ZLNodeExporterCollectorFailed` | `node_scrape_collector_success` | Node | 15m | info | `(min by (node, collector) (node_scrape_collector_success)) == 0` | exporter self-health |
| `ZLNodeSystemdServiceFailed` | `node_systemd_unit_state` | Node | 5m | warning | `(max by (node, name) (node_systemd_unit_state{state="failed"})) == 1` | node-mixin NodeSystemdServiceFailed |
