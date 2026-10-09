# 4. Relations and the entity graph

A single metric tree only knows about **one source and one entity**. Real incidents cross those
boundaries:
- a throttled container is caused by a busy **node**;
- slow API calls are caused by a slow **etcd disk**;
- a pod stuck in `ContainerCreating` is caused by the **kubelet's** image pulls.

Two resources connect the trees:

- **`MetricRelations`** links *metrics* across trees: siblings, explains, eBPF.
- **`EntityGraph`** links *entities*: which pods, nodes, volumes and components depend on which.

---

## 4.1 Sibling groups (17)

A sibling group is a set of **roots in different packs that report the same condition**. When any
member fires, the operator also collects the **depth-1 children of every other member**.

Why this matters: the same underlying problem often shows up in several sources, and each source
has a different piece of the explanation. Take a memory-starved node:
- node-exporter explains *which kind* of memory is short;
- the kubelet explains whether **evictions** are about to start;
- hostmetrics can switch its `paging` scraper on.

| Group | Domain | Members (all roots) |
|---|---|---|
| `node-cpu` | cpu | `node-exporter` · Node · `node_cpu_seconds_total`<br>`hostmetrics` · Node · `cpu` |
| `node-memory` | memory | `node-exporter` · Node · `node_memory_MemAvailable_bytes`<br>`hostmetrics` · Node · `memory`<br>`kubelet` · Node · `node_memory_working_set_bytes`<br>`kubelet` · Node · `kubelet_evictions` |
| `node-disk` | storage | `node-exporter` · Node · `node_disk_io_time_seconds_total`<br>`hostmetrics` · Node · `disk`<br>`kubelet` · Node · `storage_operation_duration_seconds` |
| `node-filesystem` | storage | `node-exporter` · Node · `node_filesystem_avail_bytes`<br>`kubelet` · Node · `kubelet_evictions` |
| `node-network-throughput` | network | `node-exporter` · Node · `node_network_receive_bytes_total`<br>`hostmetrics` · Node · `network` |
| `node-network-errors` | network | `node-exporter` · Node · `node_network_receive_errs_total`<br>`cilium` · Node · `cilium_drop_count_total`<br>`calico` · Node · `felix_int_dataplane_failures` |
| `node-service-routing` | network | `node-exporter` · Node · `node_nf_conntrack_entries`<br>`kube-proxy` · Node · `kubeproxy_sync_proxy_rules_duration_seconds` |
| `node-health` | lifecycle | `kube-state-metrics` · Node · `kube_node_status_condition`<br>`kubelet` · Node · `kubelet_pleg_relist_duration_seconds`<br>`kubelet` · Node · `kubelet_runtime_operations_errors_total`<br>`kube-state-metrics` · Lease · `kube_lease_renew_time` |
| `node-unreachable` | lifecycle | `kube-state-metrics` · Node · `kube_node_spec_taint`<br>`kube-state-metrics` · Lease · `kube_lease_renew_time` |
| `container-runtime` | lifecycle | `kubelet` · Node · `kubelet_runtime_operations_errors_total`<br>`containerd` · Node · `containerd_cri_sandbox_create_network_seconds` |
| `node-pod-capacity` | scheduling | `kubelet` · Node · `kubelet_running_pods`<br>`kube-state-metrics` · Cluster · `kube_node_status_allocatable` |
| `container-memory` | memory | `cadvisor` · Container · `container_memory_working_set_bytes`<br>`kube-state-metrics` · Pod · `kube_pod_container_status_restarts_total` |
| `container-oom` | memory | `cadvisor` · Container · `container_oom_events_total`<br>`kube-state-metrics` · Pod · `kube_pod_container_status_restarts_total` |
| `pod-readiness` | lifecycle | `kube-state-metrics` · Pod · `kube_pod_status_ready`<br>`kubelet` · Pod · `prober_probe_total` |
| `pod-waiting` | lifecycle | `kube-state-metrics` · Pod · `kube_pod_container_status_waiting_reason`<br>`kubelet` · Pod · `prober_probe_total` |
| `pvc-space` | storage | `kubelet` · PersistentVolumeClaim · `kubelet_volume_stats_available_bytes`<br>`kube-state-metrics` · PersistentVolumeClaim · `kube_persistentvolumeclaim_status_phase` |
| `pvc-inodes` | storage | `kubelet` · PersistentVolumeClaim · `kubelet_volume_stats_inodes_free`<br>`kube-state-metrics` · PersistentVolumeClaim · `kube_persistentvolumeclaim_status_phase` |

Notes:
- `kubelet_evictions` appears in two groups: `node-memory` and `node-filesystem`. Evictions happen
  on memory pressure **and** on disk pressure.
- `prober_probe_total` also appears in two groups (`pod-readiness`, `pod-waiting`), and
  `kube_persistentvolumeclaim_status_phase` in two (`pvc-space`, `pvc-inodes`).
- Groups may contain two roots of the same pack (for example the two kubelet roots in
  `node-memory`). That is allowed as long as the group spans at least two packs.

---

## 4.2 Explains links (30)

An explains link says: **when this root fires, these specific child metrics of another entity are
the usual explanation.** Each target has a depth. It is collected once the ladder reaches that
depth.

Why not just collect everything about the related entity? Because it is expensive and noisy. A
node has 75 depth-1 children. When a *container* is throttled, only two or three of them matter:
CPU pressure, scheduler wait and thermal throttling.

| When this root fires | …also collect (depth) |
|---|---|
| `cadvisor` · Container · `container_cpu_usage_seconds_total` | d1 `node-exporter` · Node · `node_pressure_cpu_waiting_seconds_total`<br>d2 `node-exporter` · Node · `node_schedstat_waiting_seconds_total`<br>d3 `node-exporter` · Node · `node_cpu_core_throttles_total` |
| `cadvisor` · Container · `container_cpu_cfs_throttled_periods_total` | d1 `kube-state-metrics` · Cluster · `kube_pod_container_resource_limits`<br>d2 `node-exporter` · Node · `node_pressure_cpu_waiting_seconds_total` |
| `cadvisor` · Container · `container_memory_working_set_bytes` | d1 `node-exporter` · Node · `node_pressure_memory_waiting_seconds_total`<br>d2 `node-exporter` · Node · `node_vmstat_pgmajfault`<br>d1 `kube-state-metrics` · Cluster · `kube_pod_container_resource_limits` |
| `cadvisor` · Container · `container_oom_events_total` | d1 `node-exporter` · Node · `node_vmstat_oom_kill`<br>d1 `kube-state-metrics` · Pod · `kube_pod_container_status_last_terminated_reason` |
| `cadvisor` · Container · `container_fs_writes_bytes_total` | d1 `node-exporter` · Node · `node_pressure_io_waiting_seconds_total`<br>d1 `node-exporter` · Node · `node_disk_write_time_seconds_total`<br>d2 `node-exporter` · Node · `node_disk_io_time_weighted_seconds_total` |
| `cadvisor` · Pod · `container_network_receive_packets_dropped_total` | d1 `node-exporter` · Node · `node_network_receive_drop_total`<br>d2 `node-exporter` · Node · `node_softnet_dropped_total`<br>d2 `node-exporter` · Node · `node_nf_conntrack_stat_drop` |
| `kubelet` · Pod · `prober_probe_total` | d1 `cadvisor` · Container · `container_cpu_cfs_throttled_seconds_total`<br>d1 `cadvisor` · Container · `container_pressure_cpu_waiting_seconds_total`<br>d1 `kube-state-metrics` · Pod · `kube_pod_container_status_ready` |
| `kube-state-metrics` · Pod · `kube_pod_container_status_restarts_total` | d1 `cadvisor` · Container · `container_memory_events_max_total`<br>d2 `kubelet` · Node · `kubelet_started_containers_errors_total` |
| `kube-state-metrics` · Pod · `kube_pod_container_status_waiting_reason` | d1 `kubelet` · Node · `kubelet_image_pull_duration_seconds`<br>d1 `kubelet` · Node · `kubelet_run_podsandbox_errors_total`<br>d2 `containerd` · Node · `containerd_cri_sandboxed_image_pulls_total` |
| `kube-state-metrics` · Pod · `kube_pod_status_phase` | d1 `kube-scheduler` · Scheduler · `scheduler_unschedulable_pods`<br>d1 `kubelet` · Node · `kubelet_admission_rejections_total` |
| `kube-state-metrics` · Deployment · `kube_deployment_status_replicas_available` | d1 `kube-state-metrics` · Pod · `kube_pod_status_reason`<br>d1 `kube-state-metrics` · ReplicaSet · `kube_replicaset_status_replicas` |
| `kube-state-metrics` · StatefulSet · `kube_statefulset_status_replicas_ready` | d1 `kube-state-metrics` · Pod · `kube_pod_status_reason`<br>d1 `kube-state-metrics` · PersistentVolumeClaim · `kube_persistentvolumeclaim_info` |
| `kube-state-metrics` · DaemonSet · `kube_daemonset_status_number_available` | d1 `kube-state-metrics` · Pod · `kube_pod_status_reason`<br>d2 `kube-state-metrics` · Node · `kube_node_info` |
| `kube-state-metrics` · HorizontalPodAutoscaler · `kube_horizontalpodautoscaler_status_current_replicas` | d1 `kube-controller-manager` · ControllerManager · `horizontal_pod_autoscaler_controller_metric_computation_total` |
| `kube-state-metrics` · Job · `kube_job_failed` | d1 `kube-state-metrics` · Pod · `kube_pod_container_status_last_terminated_reason`<br>d2 `kube-controller-manager` · ControllerManager · `job_controller_pod_failures_handled_by_failure_policy_total` |
| `kubelet` · PersistentVolumeClaim · `kubelet_volume_stats_available_bytes` | d1 `kube-state-metrics` · PersistentVolumeClaim · `kube_persistentvolumeclaim_resource_requests_storage_bytes`<br>d2 `node-exporter` · Node · `node_disk_write_time_seconds_total` |
| `kube-state-metrics` · PersistentVolumeClaim · `kube_persistentvolumeclaim_status_phase` | d1 `kube-controller-manager` · ControllerManager · `pv_collector_unbound_pvc_count`<br>d2 `csi` · CSIDriver · `csi_plugin_operations_seconds` |
| `kube-state-metrics` · VolumeAttachment · `kube_volumeattachment_status_attached` | d1 `kube-controller-manager` · ControllerManager · `attachdetach_controller_total_volumes`<br>d1 `kubelet` · Node · `csi_operations_seconds` |
| `kube-state-metrics` · Node · `kube_node_status_condition` | d1 `node-exporter` · Node · `node_pressure_memory_waiting_seconds_total`<br>d1 `node-exporter` · Node · `node_pressure_io_waiting_seconds_total`<br>d1 `kube-controller-manager` · ControllerManager · `node_collector_zone_health` |
| `kube-apiserver` · APIServer · `etcd_request_duration_seconds` | d1 `etcd` · Etcd · `etcd_server_proposals_pending`<br>d2 `etcd` · Etcd · `etcd_network_peer_sent_failures_total`<br>d2 `etcd` · Etcd · `grpc_server_handling_seconds` |
| `kube-apiserver` · APIServer · `apiserver_request_sli_duration_seconds` | d2 `etcd` · Etcd · `etcd_server_slow_apply_total` |
| `kube-apiserver` · APIServer · `apiserver_admission_webhook_admission_duration_seconds` | d1 `kube-state-metrics` · Service · `kube_endpointslice_info` |
| `kube-apiserver` · APIServer · `apiserver_flowcontrol_rejected_requests_total` | d2 `kube-scheduler` · Scheduler · `rest_client_request_retries_total`<br>d2 `kube-controller-manager` · ControllerManager · `rest_client_request_retries_total` |
| `etcd` · Etcd · `etcd_disk_wal_fsync_duration_seconds` | d1 `node-exporter` · Node · `node_disk_write_time_seconds_total`<br>d1 `node-exporter` · Node · `node_pressure_io_waiting_seconds_total`<br>d2 `node-exporter` · Node · `node_disk_io_time_weighted_seconds_total` |
| `etcd` · Etcd · `etcd_disk_backend_commit_duration_seconds` | d1 `node-exporter` · Node · `node_disk_write_time_seconds_total` |
| `etcd` · Etcd · `etcd_network_peer_round_trip_time_seconds` | d1 `node-exporter` · Node · `node_netstat_Tcp_RetransSegs` |
| `kube-scheduler` · Scheduler · `scheduler_pending_pods` | d1 `kube-state-metrics` · Pod · `kube_pod_status_unschedulable`<br>d2 `kube-state-metrics` · Node · `kube_node_info` |
| `kube-proxy` · Node · `kubeproxy_network_programming_duration_seconds` | d1 `kube-controller-manager` · ControllerManager · `endpoint_slice_controller_syncs` |
| `coredns` · CoreDNS · `coredns_dns_request_duration_seconds` | d1 `node-exporter` · Node · `node_netstat_Udp_RcvbufErrors`<br>d1 `node-exporter` · Node · `node_nf_conntrack_stat_drop`<br>d1 `cadvisor` · Container · `container_cpu_cfs_throttled_seconds_total` |
| `coredns` · CoreDNS · `coredns_forward_healthcheck_broken_total` | d1 `node-exporter` · Node · `node_netstat_Udp_InErrors` |

How to read the table by theme:

| Theme | Links |
|---|---|
| **Container → node** (noisy neighbour, resource contention) | cAdvisor CPU, throttling, memory, OOM, disk writes and network drops → node PSI, scheduler wait, page faults, OOM kills, disk write time, softnet and conntrack drops |
| **Container → its limits** | throttling and memory → `kube_pod_container_resource_limits` (KSM) |
| **Pod lifecycle → kubelet / runtime / scheduler** | restarts, waiting, phase → kubelet start errors, image pulls, sandbox errors, admission rejections, scheduler unschedulable pods, containerd image pulls |
| **Workload → pods** | Deployment, StatefulSet, DaemonSet → `kube_pod_status_reason`, replica sets, PVCs, node info |
| **Storage chain** | PVC space and phase, volume attachment → PVC requests, node disk write time, PV controller, CSI operations |
| **Control plane** | API storage latency → etcd pending proposals, peer failures, gRPC latency. etcd fsync/commit/RTT → node disk and TCP retransmits. APF rejections → client retries in scheduler and controller-manager |
| **DNS** | CoreDNS latency and forward health → node UDP buffer errors, conntrack drops, CoreDNS CPU throttling |

---

## 4.3 eBPF links (22)

An eBPF link names the kernel programs to start **on the affected node** for a root, from depth 2
or 3. Programs are started only if the ladder level has `enableEBPF: true` (L2 and L3 in the
policy).

| Root | eBPF programs (depth) |
|---|---|
| `node-exporter` · Node · `node_cpu_seconds_total` | `softirq-latency` (d2), `timers` (d3), `llcstat` (d3) |
| `node-exporter` · Node · `node_load1` | `cgroup` (d2), `syscalls` (d3) |
| `node-exporter` · Node · `node_memory_MemAvailable_bytes` | `oomkill` (d2), `cachestat` (d2), `shrinklat` (d3) |
| `node-exporter` · Node · `node_disk_io_time_seconds_total` | `biolatency` (d2), `ext4dist` (d3), `xfsdist` (d3) |
| `node-exporter` · Node · `node_network_receive_errs_total` | `kfree_skb` (d2), `tcp-retransmit` (d2), `udp-drops` (d2), `softirq-latency-net-rx` (d3), `inet-frags` (d3) |
| `node-exporter` · Node · `node_network_receive_bytes_total` | `tcp-window-clamps` (d2), `softirq-latency-net-rx` (d3) |
| `node-exporter` · Node · `node_nf_conntrack_entries` | `kfree_skb` (d2) |
| `cadvisor` · Container · `container_cpu_usage_seconds_total` | `cgroup` (d2) |
| `cadvisor` · Container · `container_cpu_cfs_throttled_periods_total` | `cfs-throttling` (d2) |
| `cadvisor` · Container · `container_memory_working_set_bytes` | `cachestat` (d2), `shrinklat` (d3) |
| `cadvisor` · Container · `container_oom_events_total` | `oomkill` (d2) |
| `cadvisor` · Container · `container_fs_writes_bytes_total` | `biolatency` (d2), `ext4dist` (d3), `xfsdist` (d3) |
| `cadvisor` · Pod · `container_network_receive_packets_dropped_total` | `kfree_skb` (d2), `udp-drops` (d2), `tcp-retransmit` (d3) |
| `kubelet` · PersistentVolumeClaim · `kubelet_volume_stats_available_bytes` | `biolatency` (d3) |
| `kubelet` · Pod · `prober_probe_total` | `tcp-syn-backlog` (d3), `accept-latency` (d3) |
| `etcd` · Etcd · `etcd_disk_wal_fsync_duration_seconds` | `biolatency` (d2), `ext4dist` (d3), `xfsdist` (d3) |
| `etcd` · Etcd · `etcd_disk_backend_commit_duration_seconds` | `biolatency` (d2) |
| `etcd` · Etcd · `etcd_network_peer_round_trip_time_seconds` | `tcp-retransmit` (d2) |
| `kube-apiserver` · APIServer · `apiserver_request_sli_duration_seconds` | `tcp-syn-backlog` (d3), `accept-latency` (d3) |
| `coredns` · CoreDNS · `coredns_dns_request_duration_seconds` | `udp-drops` (d2), `softirq-latency-net-rx` (d3) |
| `coredns` · CoreDNS · `coredns_dns_responses_total` | `udp-drops` (d2) |
| `kube-proxy` · Node · `kubeproxy_sync_proxy_rules_duration_seconds` | `kfree_skb` (d3) |

The pattern is the same resource, seen from inside the kernel:

| Resource | Depth 2 | Depth 3 |
|---|---|---|
| Disk | `biolatency` (block I/O latency histogram per device and operation) | `ext4dist`, `xfsdist` (filesystem latency) |
| Memory | `oomkill`, `cachestat` (page cache) | `shrinklat` (reclaim latency) |
| CPU | `softirq-latency`, `cgroup` (migrations), `cfs-throttling` | `timers`, `llcstat` (cache misses), `syscalls` |
| Network | `kfree_skb` (drops by reason), `tcp-retransmit`, `udp-drops`, `tcp-window-clamps` | `softirq-latency-net-rx`, `inet-frags`, `tcp-syn-backlog`, `accept-latency` |

When several incidents are open on one node, their programs are **merged** into one ebpf_exporter
pod on that node.

---

## 4.4 Entity classes (26)

| Entity class | Operator kind | Join labels | Note |
|---|---|---|---|
| `Node` | (same) | `node` | Per-node scrapes add node=<K8S_NODE_NAME> in relabel_configs; KSM/Node already has 'node'. KSM/Lease joins with lease == node in namespace kube-node-lease. |
| `Pod` | (same) | `namespace`, `pod` | KSM/Pod has no node label: node comes from kube_pod_info{node} or the informer cache. |
| `Container` | Pod | `namespace`, `pod`, `container` | cAdvisor: filter container!=""; KSM container series carry 'container'. |
| `PersistentVolumeClaim` | (same) | `namespace`, `persistentvolumeclaim` |  |
| `PersistentVolume` | (same) | `persistentvolume` |  |
| `Cluster` | (same) | — |  |
| `Namespace` | (same) | `namespace` |  |
| `Deployment` | (same) | `namespace`, `deployment` |  |
| `ReplicaSet` | (same) | `namespace`, `replicaset` |  |
| `StatefulSet` | (same) | `namespace`, `statefulset` |  |
| `DaemonSet` | (same) | `namespace`, `daemonset` |  |
| `ReplicationController` | (same) | `namespace`, `replicationcontroller` |  |
| `Job` | (same) | `namespace`, `job_name` |  |
| `CronJob` | (same) | `namespace`, `cronjob` |  |
| `HorizontalPodAutoscaler` | (same) | `namespace`, `horizontalpodautoscaler` |  |
| `PodDisruptionBudget` | (same) | `namespace`, `poddisruptionbudget` |  |
| `VolumeAttachment` | (same) | `volumeattachment` |  |
| `Service` | (same) | `namespace`, `endpointslice` | slice -> service via kube_endpointslice_labels{label_kubernetes_io_service_name}. |
| `Lease` | (same) | `namespace`, `lease` |  |
| `CertificateSigningRequest` | (same) | `certificatesigningrequest` |  |
| `APIServer` | (same) | `node` |  |
| `Etcd` | EtcdMember | `node` |  |
| `Scheduler` | (same) | `node` |  |
| `ControllerManager` | (same) | `node` |  |
| `CoreDNS` | Pod | `namespace`, `pod` |  |
| `CSIDriver` | (same) | `driver_name` |  |

`operatorKind` matters because the operator acts on two kinds today, `Pod` and `Node`:
- a `Container` alert is handled as an alert on its **Pod**;
- a `CoreDNS` alert as an alert on the CoreDNS **Pod**;
- `Etcd` alerts carry `zoomlens_entity: EtcdMember` and are skipped for now.

---

## 4.5 Entity edges (38)

| From | Relation | To | Discovery | Defined by | Domains | Both ways |
|---|---|---|---|---|---|---|
| Container | `part-of` | Pod | informer | Pod.spec.containers | cpu, memory, storage, network, dns, lifecycle, control-plane, scheduling | yes |
| Pod | `runs-on` | Node | informer | Pod.spec.nodeName | cpu, memory, storage, network, lifecycle | yes |
| Pod | `co-located-with` | Pod | rule | same Pod.spec.nodeName (2 hops via NODE) | cpu, memory, storage, network | yes |
| Pod | `mounts` | PersistentVolumeClaim | informer | Pod.spec.volumes[].persistentVolumeClaim | storage, lifecycle | yes |
| PersistentVolumeClaim | `bound-to` | PersistentVolume | informer | PersistentVolumeClaim.spec.volumeName | storage | yes |
| PersistentVolume | `attached-via` | VolumeAttachment | informer | VolumeAttachment.spec.source.persistentVolumeName | storage | yes |
| VolumeAttachment | `attached-to` | Node | informer | VolumeAttachment.spec.nodeName | storage | yes |
| PersistentVolume | `provisioned-by` | CSIDriver | informer | PersistentVolume.spec.csi.driver | storage | yes |
| Pod | `owned-by` | ReplicaSet | informer | Pod.metadata.ownerReferences | lifecycle | yes |
| ReplicaSet | `owned-by` | Deployment | informer | ReplicaSet.metadata.ownerReferences | lifecycle | yes |
| Pod | `owned-by` | StatefulSet | informer | Pod.metadata.ownerReferences | lifecycle, storage | yes |
| Pod | `owned-by` | DaemonSet | informer | Pod.metadata.ownerReferences | lifecycle | yes |
| Pod | `owned-by` | ReplicationController | informer | Pod.metadata.ownerReferences | lifecycle | yes |
| Pod | `owned-by` | Job | informer | Pod.metadata.ownerReferences | lifecycle | yes |
| Job | `owned-by` | CronJob | informer | Job.metadata.ownerReferences | lifecycle | yes |
| HorizontalPodAutoscaler | `scales` | Deployment | informer | HorizontalPodAutoscaler.spec.scaleTargetRef | cpu, memory, lifecycle, scheduling | yes |
| HorizontalPodAutoscaler | `scales` | StatefulSet | informer | HorizontalPodAutoscaler.spec.scaleTargetRef | cpu, memory, lifecycle, scheduling | yes |
| PodDisruptionBudget | `protects` | Pod | informer | PodDisruptionBudget.spec.selector | lifecycle | yes |
| Service | `selects` | Pod | informer | EndpointSlice.endpoints[].targetRef | network, lifecycle | yes |
| Pod | `depends-on` | Service | annotation | zoomlens.dev/depends-on: <service>[,<service>] | network, lifecycle | no |
| Pod | `resolves-via` | CoreDNS | rule | every Pod with dnsPolicy ClusterFirst / ClusterFirstWithHostNet | dns, network | no |
| CoreDNS | `runs-on` | Node | informer | Pod.spec.nodeName of k8s-app=kube-dns pods | dns, network, cpu, memory | yes |
| Node | `heartbeats-via` | Lease | informer | Lease kube-node-lease/<node> | lifecycle | yes |
| Namespace | `contains` | Pod | informer | Pod.metadata.namespace | scheduling, lifecycle | no |
| Cluster | `contains` | Node | rule | all Nodes | scheduling | no |
| APIServer | `stores-in` | Etcd | rule | --etcd-servers | control-plane | yes |
| APIServer | `runs-on` | Node | informer | static Pod kube-apiserver-<node> | control-plane, cpu, memory, network | yes |
| Etcd | `runs-on` | Node | informer | static Pod etcd-<node> | control-plane, storage, network | yes |
| Scheduler | `runs-on` | Node | informer | static Pod kube-scheduler-<node> | control-plane, cpu | yes |
| ControllerManager | `runs-on` | Node | informer | static Pod kube-controller-manager-<node> | control-plane, cpu | yes |
| Scheduler | `calls` | APIServer | rule | rest_client host | control-plane | no |
| ControllerManager | `calls` | APIServer | rule | rest_client host | control-plane | no |
| Node | `calls` | APIServer | rule | kubelet / kube-proxy rest_client host | control-plane, lifecycle | no |
| CoreDNS | `calls` | APIServer | rule | kubernetes plugin watch | dns, control-plane | no |
| APIServer | `calls-webhook` | Service | metrics | kube_{validating,mutating}webhookconfiguration_webhook_clientconfig_service | control-plane | no |
| APIServer | `aggregates` | Service | informer | APIService.spec.service | control-plane | no |
| Scheduler | `places` | Pod | rule | pending pods (scheduler_pending_pods) | scheduling | no |
| CertificateSigningRequest | `issued-for` | Node | rule | kubelet client/serving CSR (spec.username system:node:<node>) | lifecycle | yes |

### How gating works, step by step

The firing root has a domain `D`. From the alerting entity, the walk follows only edges whose
`domains` contain `D`, up to `hops` edges (1 at L1 and L2, 2 at L3).

**Example 1: CPU on a pod** (`container_cpu_usage_seconds_total`, domain `cpu`)

```
hop 1:  Pod ─runs-on─▶ Node                 (runs-on allows cpu)        ✔
        Pod ─mounts─▶ PersistentVolumeClaim (mounts allows storage, lifecycle) ✘
        Pod ─resolves-via─▶ CoreDNS         (dns, network)              ✘
        Pod ─owned-by─▶ ReplicaSet          (lifecycle)                 ✘
hop 2:  Node ─runs-on (reverse)─▶ Pods on that node                      ✔  ← co-located pods
```

**Example 2: disk on a node** (`node_disk_io_time_seconds_total`, domain `storage`)

```
hop 1:  Node ─runs-on (reverse)─▶ Pods on the node          (runs-on allows storage)      ✔
        Node ─attached-to (reverse)─▶ VolumeAttachment      (storage)                     ✔ design
hop 2:  VolumeAttachment ─attached-via (reverse)─▶ PersistentVolume (storage)             ✔ design
        Pod ─mounts─▶ PersistentVolumeClaim                 (storage)                     ✔ design
```

**Example 3: API latency** (`etcd_request_duration_seconds`, domain `control-plane`)

```
hop 1:  APIServer ─stores-in─▶ Etcd          (control-plane)  ✔
        APIServer ─runs-on─▶ Node            (control-plane)  ✔
        APIServer ─calls-webhook─▶ Service   (control-plane)  ✔   ← slow admission webhooks
hop 2:  Etcd ─runs-on─▶ Node                 (control-plane)  ✔   ← the etcd node's disk
```

**Example 4: a pod stuck waiting** (`kube_pod_container_status_waiting_reason`, domain
`lifecycle`)

```
hop 1:  Pod ─runs-on─▶ Node (lifecycle ✔) · Pod ─owned-by─▶ ReplicaSet (lifecycle ✔)
        Pod ─mounts─▶ PersistentVolumeClaim (lifecycle ✔) · PodDisruptionBudget, Service (lifecycle ✔)
hop 2:  ReplicaSet ─owned-by─▶ Deployment ✔ · Node ─heartbeats-via─▶ Lease ✔
        PersistentVolumeClaim ─bound-to─▶ PersistentVolume ✘ (storage only)
```

### What the operator implements today

The full graph is the **design**. The operator's current implementation (`crd.Graph` in
`internal/knowledge/crd/store.go`) uses the graph as a **gate on the Pod ↔ Node walk only**:

- It checks the path `Pod → Node → Pod` (from a Pod) or `Node → Pod` (from a Node) against the
  edges, for the root's domain. Because of operator kinds, the `Container` and `CoreDNS` edges
  count as Pod edges.
- It allows as many of the requested hops as the edges permit. The actual walk (which node, which
  pods) is done by the inner Pod/Node graph with Kubernetes API calls.
- Other classes (PVC, PV, Service, CoreDNS as a separate target, control-plane components) are
  **not traversed yet**.
- A root that no pack declares (an application root without a pack) is not gated.

Examples 1 and 2 above work today; examples 3 and 4 are design only. Chapter 6 has the details.
