# Add-on / infrastructure component Prometheus metric catalogue

Research date: 2026-09-30. Everything below comes from upstream docs or source files fetched on that date. Nothing here is invented. Where an exposition name had to be *derived* from source (for example a namespace plus a suffix added by a library), it is marked **(derived)**.

Legend for the "Tree" hints, which are only suggestions for the metric tree: **R** = root (always collect), **C** = child (collect during incidents / drill-down).

---

## 1. etcd

**Sources**
- https://etcd.io/docs/v3.5/metrics/ and https://etcd.io/docs/v3.6/metrics/ (`etcd-io/website` `content/en/docs/v3.{5,6}/metrics/_index.md`). The v3.5 and v3.6 prose pages are the same apart from one markdown note.
- The "Generated list of metrics" (`etcd-metrics-latest.txt`) is **byte-identical for v3.5 and v3.6**. Its header says `server_version="3.5.0-pre"`, so it is old. Only metrics that had a sample when it was generated appear in it. Label-vector metrics such as `peer_sent_failures_total` are missing.
- Source: `etcd-io/etcd@main` `server/etcdserver/api/rafthttp/metrics.go` and `server/etcdserver/metrics.go`, used for the extras in 1b.
- Mixin: `etcd-io/etcd@main` `contrib/mixin/alerts/alerts.libsonnet`.

**Notes**
- The docs say the `etcd_` namespace is stable ("If there is any change of these metrics, it will be included in release notes") and `etcd_debugging_` is volatile.
- `etcd_server_proposals_committed_total` and `etcd_server_proposals_applied_total` are **gauges** even though their names end in `_total`.
- `grpc_server_handling_seconds` (histogram, which `etcdGRPCRequestsSlow` needs) is only exported with `--metrics=extensive`. The flag help text in `server/embed/config.go` says: "specify 'extensive' to include server side grpc histogram metrics and the full Go runtime metrics set".

### 1a. Documented / generated metrics (etcd_*, grpc_*, os_*; go_/process_ omitted except the FD ones)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `etcd_cluster_version` | gauge | cluster_version | Which version is running. 1 for 'cluster_version' label with current cluster version |
| `etcd_debugging_auth_revision` | gauge | - | The current revision of auth store. |
| `etcd_debugging_disk_backend_commit_rebalance_duration_seconds` | histogram | - | The latency distributions of commit.rebalance called by bboltdb backend. |
| `etcd_debugging_disk_backend_commit_spill_duration_seconds` | histogram | - | The latency distributions of commit.spill called by bboltdb backend. |
| `etcd_debugging_disk_backend_commit_write_duration_seconds` | histogram | - | The latency distributions of commit.write called by bboltdb backend. |
| `etcd_debugging_lease_granted_total` | counter | - | The total number of granted leases. |
| `etcd_debugging_lease_renewed_total` | counter | - | The number of renewed leases seen by the leader. |
| `etcd_debugging_lease_revoked_total` | counter | - | The total number of revoked leases. |
| `etcd_debugging_lease_ttl_total` | histogram | - | Bucketed histogram of lease TTLs. |
| `etcd_debugging_mvcc_compact_revision` | gauge | - | The revision of the last compaction in store. |
| `etcd_debugging_mvcc_current_revision` | gauge | - | The current revision of store. |
| `etcd_debugging_mvcc_db_compaction_keys_total` | counter | - | Total number of db keys compacted. |
| `etcd_debugging_mvcc_db_compaction_last` | gauge | - | The unix time of the last db compaction. Resets to 0 on start. |
| `etcd_debugging_mvcc_db_compaction_pause_duration_milliseconds` | histogram | - | Bucketed histogram of db compaction pause duration. |
| `etcd_debugging_mvcc_db_compaction_total_duration_milliseconds` | histogram | - | Bucketed histogram of db compaction total duration. |
| `etcd_debugging_mvcc_db_total_size_in_bytes` | gauge | - | Total size of the underlying database physically allocated in bytes. |
| `etcd_debugging_mvcc_delete_total` | counter | - | Total number of deletes seen by this member. |
| `etcd_debugging_mvcc_events_total` | counter | - | Total number of events sent by this member. |
| `etcd_debugging_mvcc_index_compaction_pause_duration_milliseconds` | histogram | - | Bucketed histogram of index compaction pause duration. |
| `etcd_debugging_mvcc_keys_total` | gauge | - | Total number of keys. |
| `etcd_debugging_mvcc_pending_events_total` | gauge | - | Total number of pending events to be sent. |
| `etcd_debugging_mvcc_put_total` | counter | - | Total number of puts seen by this member. |
| `etcd_debugging_mvcc_range_total` | counter | - | Total number of ranges seen by this member. |
| `etcd_debugging_mvcc_slow_watcher_total` | gauge | - | Total number of unsynced slow watchers. |
| `etcd_debugging_mvcc_total_put_size_in_bytes` | gauge | - | The total size of put kv pairs seen by this member. |
| `etcd_debugging_mvcc_txn_total` | counter | - | Total number of txns seen by this member. |
| `etcd_debugging_mvcc_watch_stream_total` | gauge | - | Total number of watch streams. |
| `etcd_debugging_mvcc_watcher_total` | gauge | - | Total number of watchers. |
| `etcd_debugging_server_lease_expired_total` | counter | - | The total number of expired leases. |
| `etcd_debugging_snap_save_marshalling_duration_seconds` | histogram | - | The marshalling cost distributions of save called by snapshot. |
| `etcd_debugging_snap_save_total_duration_seconds` | histogram | - | The total latency distributions of save called by snapshot. |
| `etcd_debugging_store_expires_total` | counter | - | Total number of expired keys. |
| `etcd_debugging_store_reads_total` | counter | action | Total number of reads action by (get/getRecursive), local to this member. |
| `etcd_debugging_store_watch_requests_total` | counter | - | Total number of incoming watch requests (new or reestablished). |
| `etcd_debugging_store_watchers` | gauge | - | Count of currently active watchers. |
| `etcd_debugging_store_writes_total` | counter | action | Total number of writes (e.g. set/compareAndDelete) seen by this member. |
| `etcd_disk_backend_commit_duration_seconds` | histogram | - | The latency distributions of commit called by backend. |
| `etcd_disk_backend_defrag_duration_seconds` | histogram | - | The latency distribution of backend defragmentation. |
| `etcd_disk_backend_snapshot_duration_seconds` | histogram | - | The latency distribution of backend snapshots. |
| `etcd_disk_wal_fsync_duration_seconds` | histogram | - | The latency distributions of fsync called by WAL. |
| `etcd_disk_wal_write_bytes_total` | gauge | - | Total number of bytes written in WAL. |
| `etcd_mvcc_db_open_read_transactions` | gauge | - | The number of currently open read transactions |
| `etcd_mvcc_db_total_size_in_bytes` | gauge | - | Total size of the underlying database physically allocated in bytes. |
| `etcd_mvcc_db_total_size_in_use_in_bytes` | gauge | - | Total size of the underlying database logically in use in bytes. |
| `etcd_mvcc_delete_total` | counter | - | Total number of deletes seen by this member. |
| `etcd_mvcc_hash_duration_seconds` | histogram | - | The latency distribution of storage hash operation. |
| `etcd_mvcc_hash_rev_duration_seconds` | histogram | - | The latency distribution of storage hash by revision operation. |
| `etcd_mvcc_put_total` | counter | - | Total number of puts seen by this member. |
| `etcd_mvcc_range_total` | counter | - | Total number of ranges seen by this member. |
| `etcd_mvcc_txn_total` | counter | - | Total number of txns seen by this member. |
| `etcd_network_active_peers` | gauge | Local, Remote | The current number of active peer connections. |
| `etcd_network_client_grpc_received_bytes_total` | counter | - | The total number of bytes received from grpc clients. |
| `etcd_network_client_grpc_sent_bytes_total` | counter | - | The total number of bytes sent to grpc clients. |
| `etcd_network_peer_received_bytes_total` | counter | From | The total number of bytes received from peers. |
| `etcd_network_peer_round_trip_time_seconds` | histogram | To | Round-Trip-Time histogram between peers |
| `etcd_network_peer_sent_bytes_total` | counter | To | The total number of bytes sent to peers. |
| `etcd_server_client_requests_total` | counter | client_api_version, type | The total number of client requests per client version. |
| `etcd_server_go_version` | gauge | server_go_version | Which Go version server is running with. 1 for 'server_go_version' label with current version. |
| `etcd_server_has_leader` | gauge | - | Whether or not a leader exists. 1 is existence, 0 is not. |
| `etcd_server_health_failures` | counter | - | The total number of failed health checks |
| `etcd_server_health_success` | counter | - | The total number of successful health checks |
| `etcd_server_heartbeat_send_failures_total` | counter | - | The total number of leader heartbeat send failures (likely overloaded from slow disk). |
| `etcd_server_id` | gauge | server_id | Server or member ID in hexadecimal format. 1 for 'server_id' label with current ID. |
| `etcd_server_is_leader` | gauge | - | Whether or not this member is a leader. 1 if is, 0 otherwise. |
| `etcd_server_is_learner` | gauge | - | Whether or not this member is a learner. 1 if is, 0 otherwise. |
| `etcd_server_leader_changes_seen_total` | counter | - | The number of leader changes seen. |
| `etcd_server_learner_promote_successes` | counter | - | The total number of successful learner promotions while this member is leader. |
| `etcd_server_proposals_applied_total` | gauge | - | The total number of consensus proposals applied. |
| `etcd_server_proposals_committed_total` | gauge | - | The total number of consensus proposals committed. |
| `etcd_server_proposals_failed_total` | counter | - | The total number of failed proposals seen. |
| `etcd_server_proposals_pending` | gauge | - | The current number of pending proposals to commit. |
| `etcd_server_quota_backend_bytes` | gauge | - | Current backend storage quota size in bytes. |
| `etcd_server_read_indexes_failed_total` | counter | - | The total number of failed read indexes seen. |
| `etcd_server_slow_apply_total` | counter | - | The total number of slow apply requests (likely overloaded from slow disk). |
| `etcd_server_slow_read_indexes_total` | counter | - | The total number of pending read indexes not in sync with leader's or timed out read index requests. |
| `etcd_server_snapshot_apply_in_progress_total` | gauge | - | 1 if the server is applying the incoming snapshot. 0 if none. |
| `etcd_server_version` | gauge | server_version | Which version is running. 1 for 'server_version' label with current version. |
| `etcd_snap_db_fsync_duration_seconds` | histogram | - | The latency distributions of fsyncing .snap.db file |
| `etcd_snap_db_save_total_duration_seconds` | histogram | - | The total latency distributions of v3 snapshot save |
| `etcd_snap_fsync_duration_seconds` | histogram | - | The latency distributions of fsync called by snap. |
| `grpc_server_handled_total` | counter | grpc_code, grpc_method, grpc_service, grpc_type | Total number of RPCs completed on the server, regardless of success or failure. |
| `grpc_server_msg_received_total` | counter | grpc_method, grpc_service, grpc_type | Total number of RPC stream messages received on the server. |
| `grpc_server_msg_sent_total` | counter | grpc_method, grpc_service, grpc_type | Total number of gRPC stream messages sent by the server. |
| `grpc_server_started_total` | counter | grpc_method, grpc_service, grpc_type | Total number of RPCs started on the server. |
| `os_fd_limit` | gauge | - | The file descriptor limit. |
| `os_fd_used` | gauge | - | The number of used file descriptors. |
| `process_open_fds` / `process_max_fds` | gauge | - | Open FDs and the FD limit. The docs warn that when FDs run out, etcd may panic because it cannot create new WAL files |

### 1b. Extra metrics seen only in source (etcd main), not in the generated list

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `etcd_network_peer_sent_failures_total` | counter | To | Send failures to a peer (also in the v3.5 docs table). Used by etcdMembersDown |
| `etcd_network_peer_received_failures_total` | counter | From | Receive failures from a peer (also in the v3.5 docs table) |
| `etcd_network_disconnected_peers_total` | counter | Local, Remote | Disconnected peers |
| `etcd_network_snapshot_send_success`, `etcd_network_snapshot_send_failures`, `etcd_network_snapshot_send_inflights_total` | (types not verified) | To | Raft snapshot sends |
| `etcd_network_snapshot_send_total_duration_seconds` | histogram | To | Snapshot send latency |
| `etcd_network_snapshot_receive_success`, `etcd_network_snapshot_receive_failures`, `etcd_network_snapshot_receive_inflights_total` | (types not verified) | From | Raft snapshot receives |
| `etcd_network_snapshot_receive_total_duration_seconds` | histogram | From | Snapshot receive latency |
| `etcd_server_learner_promote_failures` | counter | Reason | Failed learner promotions |
| `etcd_server_request_duration_seconds` | histogram | type, success | Server request duration (in main; not in the generated list) |
| `etcd_server_feature_enabled` | gauge | name, stage | Feature-gate state (main) |
| `grpc_server_handling_seconds` | histogram | grpc_type, grpc_service, grpc_method | gRPC handling latency. Needs `--metrics=extensive` |

`grpc_server_*` comes from go-grpc-prometheus.

### 1c. etcd-mixin alerts → metrics (etcd main, contrib/mixin)

| Alert | Severity / for | Metric(s) | Condition |
|---|---|---|---|
| etcdMembersDown | warning / 20m | `up`, `etcd_network_peer_sent_failures_total` | a member is down, or peer send-failure rate > 0.01 |
| etcdInsufficientMembers | critical / 3m | `up` | up members < (count+1)/2 |
| etcdNoLeader | critical / 1m | `etcd_server_has_leader` | == 0 |
| etcdHighNumberOfLeaderChanges | warning / 5m | `etcd_server_leader_changes_seen_total` | increase over 15m >= 4 |
| etcdHighNumberOfFailedGRPCRequests | warning / 10m (>1%); critical / 5m (>5%) | `grpc_server_handled_total` | grpc_code in Unknown, FailedPrecondition, ResourceExhausted, Internal, Unavailable, DataLoss, DeadlineExceeded |
| etcdGRPCRequestsSlow | critical / 10m | `grpc_server_handling_seconds_bucket` | p99 unary (grpc_method != Defragment) > 0.15s |
| etcdMemberCommunicationSlow | warning / 10m | `etcd_network_peer_round_trip_time_seconds_bucket` | p99 > 0.15s |
| etcdHighNumberOfFailedProposals | warning / 15m | `etcd_server_proposals_failed_total` | rate[15m] > 5 |
| etcdHighFsyncDurations | warning (>0.5s) and critical (>1s), 10m | `etcd_disk_wal_fsync_duration_seconds_bucket` | p99 |
| etcdHighCommitDurations | warning / 10m | `etcd_disk_backend_commit_duration_seconds_bucket` | p99 > 0.25s |
| etcdDatabaseQuotaLowSpace | critical / 10m | `etcd_mvcc_db_total_size_in_bytes`, `etcd_server_quota_backend_bytes` | > 95% of quota |
| etcdExcessiveDatabaseGrowth | warning / 10m | same two | predict_linear over 4h exceeds the quota |
| etcdDatabaseHighFragmentationRatio | warning / 10m | `etcd_mvcc_db_total_size_in_use_in_bytes`, `etcd_mvcc_db_total_size_in_bytes` | in_use/total < 0.5 and in_use > 100 MiB |

The mixin uses `etcd_mvcc_db_total_size_in_bytes`, not `etcd_debugging_mvcc_db_total_size_in_bytes`. Both exist in the generated list; the debugging one is the legacy name.

**Suggested etcd tree**
- **R**: `up`, `etcd_server_has_leader`, `etcd_server_leader_changes_seen_total`, `etcd_server_proposals_failed_total`, `etcd_server_proposals_pending`, `etcd_disk_wal_fsync_duration_seconds`, `etcd_disk_backend_commit_duration_seconds`, `etcd_network_peer_round_trip_time_seconds`, `etcd_mvcc_db_total_size_in_bytes`, `etcd_mvcc_db_total_size_in_use_in_bytes`, `etcd_server_quota_backend_bytes`, `grpc_server_handled_total`.
- **C**: `etcd_network_peer_sent_failures_total` and `etcd_network_peer_received_failures_total`, `etcd_server_heartbeat_send_failures_total`, `etcd_server_slow_apply_total`, `etcd_server_slow_read_indexes_total`, `etcd_server_read_indexes_failed_total`, `etcd_server_proposals_committed_total` vs `etcd_server_proposals_applied_total`, `etcd_debugging_mvcc_*` (watchers, slow_watcher_total, keys, compaction), `etcd_disk_backend_defrag_duration_seconds`, `etcd_snap_*`, `grpc_server_handling_seconds`, `os_fd_used` and `os_fd_limit`.

---

## 2. CoreDNS

**Sources**
- coredns.io/plugins/{metrics, cache, forward, health, ready, reload, kubernetes, ...}, via the plugin READMEs in `coredns/coredns@master` (CoreVersion 1.14.7 in `coremain/version.go`).
- Metric types were confirmed from the plugin source (`plugin/metrics/vars/vars.go`, `cache/metrics.go`, `forward/metrics.go`, `pkg/proxy/metrics.go`, `health/overloaded.go`, `reload/metrics.go`, `kubernetes/metrics.go`, `kubernetes/object/metrics.go`).
- Default endpoint: `localhost:9153/metrics` (prometheus plugin).
- The **ready** plugin exports no metrics; its README has no Metrics section.

| Metric | Type | Labels | Meaning | Tree |
|---|---|---|---|---|
| `coredns_build_info` | gauge | version, revision, goversion | Build info | |
| `coredns_panics_total` | counter | - | Total panics | R |
| `coredns_dns_requests_total` | counter | server, zone, view, proto, family, type | Query count | R |
| `coredns_dns_request_duration_seconds` | histogram | server, zone, view, type | Time to process each query | R |
| `coredns_dns_request_size_bytes` | histogram | server, zone, view, proto | Request size (before rewrites) | |
| `coredns_dns_do_requests_total` | counter | server, view, zone | Queries with the DO bit set | |
| `coredns_dns_response_size_bytes` | histogram | server, zone, view, proto | Response size | |
| `coredns_dns_responses_total` | counter | server, zone, view, rcode, plugin | Responses by rcode and plugin | R |
| `coredns_dns_https_responses_total` | counter | server, status | DoH responses by HTTP status | |
| `coredns_dns_quic_responses_total` | counter | server, status | DoQ responses by QUIC app code | |
| `coredns_plugin_enabled` | gauge | server, zone, view, name | Whether a plugin is enabled | |
| `coredns_cache_entries` | gauge | server, type, zones, view | Cache elements by cache type | C |
| `coredns_cache_hits_total` | counter | server, type, zones, view | Cache hits | C |
| `coredns_cache_misses_total` | counter | server, zones, view | Cache misses. **Deprecated**: derive from requests minus hits | |
| `coredns_cache_requests_total` | counter | server, zones, view | Cache requests | C |
| `coredns_cache_prefetch_total` | counter | server, zones, view | Prefetches | |
| `coredns_cache_drops_total` | counter | server, zones, view | Responses not cached because the question name did not match | C |
| `coredns_cache_served_stale_total` | counter | server, zones, view | Served from stale entries (a sign that the upstream is failing) | C |
| `coredns_cache_evictions_total` | counter | server, type, zones, view | Cache evictions | C |
| `coredns_forward_healthcheck_broken_total` | counter | - | All upstreams unhealthy; spraying at random | R |
| `coredns_forward_max_concurrent_rejects_total` | counter | - | Queries rejected at max_concurrent | R |
| `coredns_proxy_request_duration_seconds` | histogram | proxy_name="forward", to, rcode | Per-upstream latency. The `_count` series replaces forward requests/responses | C |
| `coredns_proxy_healthcheck_failures_total` | counter | proxy_name, to, rcode | Failed upstream health checks | C |
| `coredns_proxy_conn_cache_hits_total` | counter | proxy_name, to, proto | Upstream connection cache hits | |
| `coredns_proxy_conn_cache_misses_total` | counter | proxy_name, to, proto | Upstream connection cache misses | |
| `coredns_forward_requests_total` | counter | to | **Deprecated** (use `sum(coredns_proxy_request_duration_seconds_count)`) | |
| `coredns_forward_responses_total` | counter | to, rcode | **Deprecated** | |
| `coredns_forward_request_duration_seconds` | histogram | to, rcode | **Deprecated** (use `coredns_proxy_request_duration_seconds`) | |
| `coredns_forward_healthcheck_failures_total` | counter | to, rcode | **Deprecated** (use `coredns_proxy_healthcheck_failures_total`) | |
| `coredns_health_request_duration_seconds` | histogram | - (no server label) | Self health-check duration (once per second). A rise means overload | R |
| `coredns_health_request_failures_total` | counter | - | Self health-check failures | R |
| `coredns_reload_failed_total` | counter | - | Failed Corefile reloads | C |
| `coredns_reload_version_info` | gauge | hash, value | Hash of the loaded config | |
| `coredns_kubernetes_dns_programming_duration_seconds` | histogram | service_kind (cluster_ip, headless_with_selector, headless_without_selector) | DNS programming latency SLI | C |
| `coredns_kubernetes_rest_client_request_duration_seconds` | histogram | verb, host | apiserver request latency from CoreDNS | C |
| `coredns_kubernetes_rest_client_rate_limiter_duration_seconds` | histogram | verb, host | Client-side rate-limiter latency | C |
| `coredns_kubernetes_rest_client_requests_total` | counter | method, code, host | apiserver requests by code | C |

Other plugins (from READMEs; types not verified):
- `coredns_autopath_success_total{server}`
- `coredns_hosts_entries{}` and `coredns_hosts_reload_timestamp_seconds{}`
- `coredns_dnssec_cache_entries{server,type}`, `coredns_dnssec_cache_hits_total{server}`, `coredns_dnssec_cache_misses_total{server}`
- `coredns_grpc_request_duration_seconds{to}`, `coredns_grpc_requests_total{to}`, `coredns_grpc_responses_total{to,rcode}`
- `coredns_acl_blocked_requests_total`, `coredns_acl_filtered_requests_total`, `coredns_acl_allowed_requests_total`, `coredns_acl_dropped_requests_total`
- `coredns_dns64_requests_translated_total`
- `coredns_local_localhost_requests_total`
- `coredns_template_matches_total`, `coredns_template_template_failures_total`, `coredns_template_rr_failures_total`

### 2a. CoreDNS mixin alerts (povilasv/coredns-mixin@master, the de-facto standard mixin)

| Alert | Severity / for | Metric | Condition |
|---|---|---|---|
| CoreDNSDown | critical / 15m | `up` (absent) | target missing |
| CoreDNSLatencyHigh | critical / 10m | `coredns_dns_request_duration_seconds_bucket` | p99 > 4s (`corednsLatencyCriticalSeconds`) |
| CoreDNSErrorsHigh | critical (>3%) and warning (>1%), 10m | `coredns_dns_responses_total` | rcode="SERVFAIL" ratio |
| CoreDNSForwardLatencyHigh | critical / 10m | `coredns_forward_request_duration_seconds_bucket` | p99 > 4s (`corednsForwardLatencyCriticalSeconds`) |
| CoreDNSForwardErrorsHigh | critical (>3%) and warning (>1%), 10m | `coredns_forward_responses_total` | SERVFAIL ratio |
| CoreDNSForwardHealthcheckFailureCount | warning / 10m | `coredns_forward_healthcheck_failures_total` | rate > 0 |
| CoreDNSForwardHealthcheckBrokenCount | warning / 10m | `coredns_forward_healthcheck_broken_total` | rate > 0 |

**Caveat:** the mixin's forward alerts still use the **deprecated** `coredns_forward_*` series. On current CoreDNS the replacements are `coredns_proxy_request_duration_seconds{proxy_name="forward"}` and `coredns_proxy_healthcheck_failures_total`. Rewrite those alerts (or check which series your CoreDNS version exposes) before relying on them.

---

## 3. Cloudflare ebpf_exporter

**Sources**
- `cloudflare/ebpf_exporter@master`: the `examples/` directory (every `.yaml` and the key `.bpf.c` files were fetched) and `README.md`.
- Each exposed name gets the prefix **`ebpf_exporter_`**. The README output confirms this, e.g. `ebpf_exporter_timer_starts_total`, `ebpf_exporter_bio_latency_seconds_bucket`.
- In the YAML, histograms carry an extra `bucket` label. It becomes `le` in the exposition, so it is left out of the label column below.

**Kernel requirements (from the README)**
- "all examples are expected to work on Linux 5.15 ... Thanks to CO-RE, examples are also supposed to work on any modern kernel with BTF enabled."
- Needs `/sys/kernel/btf/vmlinux`, or an external BTF passed with `--btf.path` (btfhub-archive).
- Capabilities: `CAP_BPF` and `CAP_PERFMON`; before Linux 5.8, use `CAP_SYS_ADMIN` instead. `CAP_IPC_LOCK` is needed for `perf_event_array` (used by oomkill). `CAP_SYS_RESOURCE` is needed on kernels before 5.11 (no memcg accounting of BPF memory). `CAP_SYS_ADMIN` is needed for module BTF.
- Examples must be compiled before use (`make -C examples clean build`).

**Per-example notes (from the .bpf.c files)**
- `cachestat.yaml` is for kernel >= 5.16 (it uses `raw_tp/writeback_dirty_folio`). `cachestat-pre-kernel-5.16.yaml` is for <= 5.15 (it uses `fentry/account_page_dirtied`).
- `biolatency` uses `raw_tp/block_rq_insert` and `raw_tp/block_rq_issue`, and handles the tracepoint argument change in v5.11-rc1.
- Attach points:
  - `tcp-retransmit`: `fentry/tcp_retransmit_skb` and `fentry/tcp_send_loss_probe`
  - `cfs-throttling`: `fentry/unthrottle_cfs_rq`
  - `oomkill`: `kprobe/oom_kill_process`
  - `udp-drops`: `tp_btf/udp_fail_queue_rcv_skb`
  - `kfree_skb`: `tp_btf/kfree_skb`
  - `shrinklat`: `kprobe` and `kretprobe` on `shrink_node`
  - `tcp-syn-backlog`: `kprobe/tcp_v4_syn_recv_sock` and `kprobe/tcp_v6_syn_recv_sock`
  - `timers`: `tp_btf/timer_start`
- *Unverified general knowledge, not from the repo:* fentry and tp_btf programs need BTF and BPF trampolines (around 5.5+). kprobes depend on the kernel symbol not being inlined. The `reason` field used by kfree_skb only exists on kernels with skb drop reasons (around 5.17+).

**All example files (43).** 5 are tracing-only (OpenTelemetry spans, no Prometheus metrics): `bio-trace`, `cfs-throttling-trace`, `exec-trace`, `sched-trace`, `sock-trace`. The other 38 are:

accept-latency, biolatency, bpf-jit, cachestat, cachestat-pre-kernel-5.16, cephfs-dist, cfs-throttling, cgroup, cgroup-rstat-flushing, cgroup_id_map, cgroup_id_map_cgrp_storage, ext4dist, icmp-ip, inet-frags, kfree_skb, kstack, llcstat, oomkill, pci, percpu-softirq, raw-tracepoints, regexp, shrinklat, softirq-latency, softirq-latency-net-rx, syscalls, tcp-retransmit, tcp-syn-backlog, tcp-syn-backlog-exp2zero, tcp-window-clamps, timers, udp-drops, unix-accept-latency, unix-socket-backlog, uprobe, usdt, xdp, xfsdist.

There is **no** runqueue / runqlat example. Scheduler coverage is limited to `sched-trace` (tracing only), `cgroup` / `cgroup_id_map*` (sched migrations) and `cfs-throttling`.

| Example | Metric | Type | Labels | Help |
|---|---|---|---|---|
| accept-latency | `ebpf_exporter_accept_latency_seconds` | histogram | port | Latency histogram for how long sockets sit in accept queue |
| biolatency | `ebpf_exporter_bio_latency_seconds` | histogram | device, operation | Block IO latency histogram |
| bpf-jit | `ebpf_exporter_bpf_jit_pages_currently_allocated` | counter | - | Current number of pages allocated for bpf jit |
| cachestat-pre-kernel-5.16 | `ebpf_exporter_page_cache_ops_total` | counter | operation | Page cache operation counters by type |
| cachestat | `ebpf_exporter_page_cache_ops_total` | counter | operation | Page cache operation counters by type |
| cephfs-dist | `ebpf_exporter_cephfs_latency_seconds` | histogram | operation | cephfs IO latency histogram |
| cfs-throttling | `ebpf_exporter_cfs_throttling_seconds` | histogram | cgroup | Latency histogram for CFS throttling |
| cgroup-rstat-flushing | `ebpf_exporter_cgroup_rstat_flush_total` | counter | level | Total number of times cgroup rstat were flushed (recorded per level) |
| cgroup-rstat-flushing | `ebpf_exporter_cgroup_rstat_locked_total` | counter | contended, yield, level | Times rstat lock was obtainted with state for cgroup level, contended and yield |
| cgroup-rstat-flushing | `ebpf_exporter_cgroup_rstat_map_errors_total` | counter | type | Map related errors |
| cgroup-rstat-flushing | `ebpf_exporter_cgroup_rstat_lock_wait_seconds` | histogram | - | Latency histogram for lock contention associated wait time to obtain lock |
| cgroup-rstat-flushing | `ebpf_exporter_cgroup_rstat_lock_hold_seconds` | histogram | - | Latency histogram for lock hold time |
| cgroup-rstat-flushing | `ebpf_exporter_cgroup_rstat_flush_latency_seconds` | histogram | level | Latency histogram for flush time |
| cgroup | `ebpf_exporter_cgroup_sched_migrations_total` | counter | cgroup | Number of sched:sched_migrate_task events per cgroup |
| cgroup_id_map | `ebpf_exporter_cgroup_sched_migrations_total` | counter | cgroup | Number of sched:sched_migrate_task events per cgroup |
| cgroup_id_map | `ebpf_exporter_cgroup_sched_migrations_not_match_total` | counter | cgroup | Number of sched:sched_migrate_task events per cgroup not match cgroup id map |
| cgroup_id_map_cgrp_storage | `ebpf_exporter_cgroup_cgrp_storage_sched_migrations_total` | counter | cgroup | Number of sched:sched_migrate_task events per cgroup |
| cgroup_id_map_cgrp_storage | `ebpf_exporter_cgroup_cgrp_storage_sched_migrations_not_match_total` | counter | cgroup | Number of sched:sched_migrate_task events per cgroup not match cgroup id map |
| ext4dist | `ebpf_exporter_ext4_latency_seconds` | histogram | operation | ext4 IO latency histogram |
| icmp-ip | `ebpf_exporter_icmp4_received_packets_total` | counter | source_addr | Number of icmp packets received over IPv4 |
| icmp-ip | `ebpf_exporter_icmp6_received_packets_total` | counter | source_addr | Number of icmp packets received over IPv6 |
| inet-frags | `ebpf_exporter_inet_frags_total` | counter | interface, ip_version | Netfilter inet fragments per interface and IP version |
| kfree_skb | `ebpf_exporter_kfree_skb_total` | counter | eth_proto, ip_proto, port, reason | Number of calls into skb:kfree_skb with a per-reason breakdown |
| kstack | `ebpf_exporter_mark_page_accessed_total` | counter | kstack | Total number of calls to mark_page_accessed by kstack |
| llcstat | `ebpf_exporter_llc_references_total` | counter | cpu | Last level cache operations by type |
| llcstat | `ebpf_exporter_llc_misses_total` | counter | cpu | Last level cache operations by type |
| oomkill | `ebpf_exporter_oom_kills_total` | counter | cgroup_path | Count global and cgroup level OOMs |
| pci | `ebpf_exporter_pci_user_read_config_ops_total` | counter | pci_vendor, pci_device, pci_class, pci_subclass | The number of operations reading pci configs |
| percpu-softirq | `ebpf_exporter_softirqs_total` | counter | vec, cpu | Total number of softirq calls per cpu |
| raw-tracepoints | `ebpf_exporter_raw_timer_starts_total` | counter | function | Timers fired in the kernel |
| regexp | `ebpf_exporter_mm_shrink_slab_events_total` | counter | comm | Total number of calls into vmscan:mm_shrink_slab_end by comm |
| shrinklat | `ebpf_exporter_shrink_node_latency_seconds` | histogram | - | Latency histogram for shrink_node calls |
| softirq-latency-net-rx | `ebpf_exporter_softirq_wait_seconds` | histogram | - | Latency histogram for how long it takes from NET_RX softirq raise to entry aka wait time |
| softirq-latency-net-rx | `ebpf_exporter_softirq_runtime_seconds` | histogram | - | Latency histogram for how long it takes from NET_RX softirq entry to exit aka runtime |
| softirq-latency-net-rx | `ebpf_exporter_softirq_raised_total` | counter | - | Total number of times NET_RX softirq were raised |
| softirq-latency-net-rx | `ebpf_exporter_softirq_serviced_total` | counter | - | Total number of times NET_RX softirq were serviced |
| softirq-latency | `ebpf_exporter_softirq_entry_latency_seconds` | histogram | kind | Latency histogram for how long it takes from softirq raise to entry |
| softirq-latency | `ebpf_exporter_softirq_service_latency_seconds` | histogram | kind | Latency histogram for how long it takes from softirq entry to exit |
| softirq-latency | `ebpf_exporter_softirq_raised_total` | counter | kind | Total number of times softirq were raised |
| softirq-latency | `ebpf_exporter_softirq_serviced_total` | counter | kind | Total number of times softirq were serviced |
| syscalls | `ebpf_exporter_syscalls_total` | counter | syscall | Total number of executed syscalls by name |
| syscalls | `ebpf_exporter_syscall_errors_total` | counter | errno | Total number of errors returned from syscalls by errno |
| tcp-retransmit | `ebpf_exporter_tcp_retransmit_ipv4_packets_total` | counter | local_ip, peer_ip, main_port, type | Total count of IPv4 TCP retransmissions |
| tcp-retransmit | `ebpf_exporter_tcp_retransmit_ipv6_packets_total` | counter | local_ip, peer_ip, main_port, type | Total count of IPv6 TCP retransmissions |
| tcp-syn-backlog-exp2zero | `ebpf_exporter_tcp_syn_backlog` | histogram | - | TCP SYN backlog size |
| tcp-syn-backlog | `ebpf_exporter_tcp_syn_backlog` | histogram | - | TCP SYN backlog size |
| tcp-window-clamps | `ebpf_exporter_tcp_window_clamps_total` | counter | - | Number of times that TCP window was clamped to a low value |
| timers | `ebpf_exporter_timer_starts_total` | counter | function | Timers fired in the kernel |
| udp-drops | `ebpf_exporter_udp_fail_queue_rcv_skbs_total` | counter | local_port | Total number of UDP SKBs that failed to be delivered due to full receive buffer |
| unix-accept-latency | `ebpf_exporter_unix_accept_latency_seconds` | histogram | addr | UNIX stream socket connect-to-accept latency histogram |
| unix-socket-backlog | `ebpf_exporter_unix_socket_backlog` | histogram | addr | Unix socket backlog size |
| uprobe | `ebpf_exporter_libc_malloc_calls_total` | counter | cgroup | Number of libc malloc calls per cgroup |
| usdt | `ebpf_exporter_python_module_imports_total` | counter | module | Number of python:import__find__load__start USDT calls per module |
| xdp | `ebpf_exporter_xdp_incoming_packets_total` | counter | ip, protocol, dst_port | Incoming packets going through xdp by protocol and port |
| xfsdist | `ebpf_exporter_xfs_latency_seconds` | histogram | operation | xfs IO latency histogram |

**Label values (from the YAML static_maps)**
- `bio_latency_seconds`: `operation` is one of read, write, flush, discard, secure_erase, write_zeroes, drv_in, drv_out. `device` is the major:minor decoded device name.
- `tcp_retransmit_*`: `type` is retransmit or tail_loss_probe. **High cardinality** because of local_ip, peer_ip and main_port.
- `page_cache_ops_total`: `operation` is one of cache_access, cache_writes, page_add_lru, page_mark_dirties.
- `kfree_skb_total`: `reason` is decoded from SKB_DROP_REASON_*.
- `cfs_throttling_seconds.cgroup` and `oom_kills_total.cgroup_path` hold cgroup paths. **High cardinality** on busy nodes.
- `bpf_jit_pages_currently_allocated`: the YAML comment says "Technically this is a gauge, but ebpf_exporter only knows counters".

**Exporter self-metrics (README)**
- `ebpf_exporter_enabled_configs`
- `ebpf_exporter_ebpf_programs`, `ebpf_exporter_ebpf_program_info`, `ebpf_exporter_ebpf_program_attached`
- `ebpf_exporter_ebpf_program_run_count_total`, `ebpf_exporter_ebpf_program_run_time_seconds`
- `ebpf_exporter_decoder_errors_total`

**Suggested tree**
- **R** (cheap, low cardinality once aggregated): `oom_kills_total`, `udp_fail_queue_rcv_skbs_total`, `kfree_skb_total` (sum by reason), `tcp_window_clamps_total`, `cfs_throttling_seconds`, `ebpf_exporter_ebpf_program_attached`.
- **C** (incident only; costly or high cardinality): `bio_latency_seconds`, `tcp_retransmit_ipv4_packets_total` and `tcp_retransmit_ipv6_packets_total`, `page_cache_ops_total`, `shrink_node_latency_seconds`, `tcp_syn_backlog`, `accept_latency_seconds`, the softirq-latency families, `timer_starts_total`, `syscalls_total` and `syscall_errors_total`, `ext4_latency_seconds` and `xfs_latency_seconds`, `cgroup_rstat_*`.

---

## 4. containerd (containerd-specific only; kubelet/cAdvisor runtime metrics are covered elsewhere)

**Sources**
- `containerd/containerd@main` `docs/ops.md`: "containerd also exports its own metrics as well as container level metrics via the Prometheus metrics format under `/v1/metrics`". The endpoint is enabled with `[metrics] address = "127.0.0.1:1234"` (TCP only).
- There is **no documented metric list**. The names below come from source and the `docker/go-metrics` naming rules (`makeName(name, unit)` appends `_seconds` to timers, `_total` to counters, and the unit to gauges). They are all **(derived)**; confirm against a live `/v1/metrics`.

**CRI plugin** (`internal/cri/server/metrics.go`, namespace `containerd`, subsystem `cri`)

| Metric (derived) | Type | Labels | Meaning |
|---|---|---|---|
| `containerd_cri_sandbox_create_network_seconds` | histogram | - | Time to create the pod network (CNI ADD) |
| `containerd_cri_sandbox_delete_network_seconds` | histogram | - | Time to delete the pod network |
| `containerd_cri_sandbox_runtime_create_seconds`, `containerd_cri_sandbox_runtime_stop_seconds`, `containerd_cri_sandbox_remove_seconds` | histogram | runtime | Sandbox lifecycle latency |
| `containerd_cri_container_create_seconds`, `..._container_start_seconds`, `..._container_stop_seconds`, `..._container_remove_seconds`, `..._container_checkpoint_seconds` | histogram | runtime | Container lifecycle latency |
| `containerd_cri_sandbox_list_seconds`, `containerd_cri_container_list_seconds`, `containerd_cri_stream_sandbox_list_seconds`, `containerd_cri_stream_container_list_seconds` | histogram | - | List latency |
| `containerd_cri_container_events_dropped_total` | counter | - | Container events dropped since start |
| `containerd_cri_network_plugin_operations_total` + `_total` suffix | counter | operation_type | CNI operations. The source name already ends in `_total`, so go-metrics probably exposes `..._total_total` |
| `containerd_cri_network_plugin_operations_errors_total` (+ `_total`) | counter | operation_type | CNI operation errors (same caveat) |
| `containerd_cri_network_plugin_operations_duration_seconds` (+ `_seconds`) | histogram | operation_type | CNI operation latency (same caveat) |

**Image pulls** (`internal/cri/server/images/metrics.go`, subsystem `cri_sandboxed`)
- `containerd_cri_sandboxed_image_pulls_total{status}` (counter)
- `containerd_cri_sandboxed_in_progress_image_pulls_total` (gauge)
- `containerd_cri_sandboxed_image_pulling_throughput_mibps` (histogram). The older `containerd_cri_sandboxed_image_pulling_throughput` is deprecated.

**cgroup v2 per-task metrics** (`core/metrics/cgroups/v2/*.go`, namespace `container`, all exported as gauges)
- CPU: `container_cpu_usage_usec_microseconds`, `container_cpu_nr_periods_total`, `container_cpu_nr_throttled_total`, `container_cpu_throttled_usec_microseconds`
- Memory: `container_memory_usage_bytes`, `container_memory_usage_limit_bytes`, `container_memory_oom_total`, plus about 30 more `container_memory_*` fields
- PIDs: `container_pids_limit`, `container_pids_current`
- IO: `container_io_rbytes_bytes`, `container_io_wbytes_bytes`, `container_io_rios_total`, `container_io_wios_total` (labels major, minor)

These cgroup metrics overlap heavily with cAdvisor. Treat them as child metrics at most, or skip them.

**Suggested:** R = `containerd_cri_sandbox_create_network_seconds` and the network_plugin error counter. C = the other lifecycle histograms and image pull counters.

---

## 5. Optional CNI (collect only if that CNI is deployed)

### 5a. Cilium agent (optional)

Source: `cilium/cilium@main` `Documentation/observability/metrics.rst`. Exported under the `cilium_` namespace. Types are not given in the table.

| Metric | Labels | Meaning |
|---|---|---|
| `cilium_drop_count_total` | reason, direction | Dropped packets |
| `cilium_drop_bytes_total` | reason, direction | Dropped bytes |
| `cilium_endpoint_regenerations_total` | reason, outcome, error | Completed endpoint regenerations |
| `cilium_endpoint_regeneration_time_stats_seconds` | scope, status | Endpoint regeneration time |
| `cilium_endpoint_state` | endpoint_state | Endpoints per state |
| `cilium_bpf_map_pressure` | map_name | Required vs configured map size; >= 1.0 means the map is full |
| `cilium_bpf_map_ops_total` | map_name, operation, outcome | eBPF map operations (outcome=fail means errors) |
| `cilium_policy_change_total` | source, operation, outcome | Policy changes by outcome |
| `cilium_policy_implementation_delay` | source | Delay from a policy change to datapath enforcement |
| `cilium_unreachable_nodes`, `cilium_unreachable_health_endpoints` | - | Node and health-endpoint reachability |
| `cilium_controllers_failing` | - | Number of failing controllers |
| `cilium_ipcache_errors_total` | type, error | ipcache errors |
| `cilium_k8s_client_api_calls_total` | host, method, return_code | kube-apiserver calls |

### 5b. Calico Felix (optional)

Source: https://docs.tigera.io/calico/latest/reference/felix/prometheus (Calico 3.32; default port 9091). Types are not stated on the page.

| Metric | Meaning |
|---|---|
| `felix_int_dataplane_failures` | Dataplane updates that failed and will be retried |
| `felix_int_dataplane_apply_time_seconds` | Time to apply a dataplane update |
| `felix_resync_state` | 1 = waiting for datastore, 2 = resync in progress, 3 = in sync |
| `felix_resyncs_started` | Datastore resyncs started |
| `felix_iptables_restore_errors`, `felix_iptables_save_errors` | iptables-restore / iptables-save errors |
| `felix_iptables_lock_retries` | Retries because the iptables lock was held |
| `felix_bpf_dirty_dataplane_endpoints` | BPF endpoints left dirty after a failure (eBPF dataplane) |
| `felix_active_local_endpoints`, `felix_active_local_policies` | Local endpoint and policy counts |
| `felix_calc_graph_update_time_seconds` | Calculation-graph update latency |
| `felix_logs_dropped`, `felix_log_errors` | Logging back-pressure and errors |

---

## 6. CSI

| Metric | Type | Labels | Source | Meaning |
|---|---|---|---|---|
| `csi_sidecar_operations_seconds` | histogram | driver_name, method_name, grpc_status_code (+ optional `migrated` and custom labels) | kubernetes-csi/csi-lib-utils `metrics/metrics.go` (subsystem `csi_sidecar`, name `operations_seconds`; help "Container Storage Interface operation duration with gRPC error code status total") | Latency and gRPC code of CSI calls from sidecars (external-provisioner, attacher, resizer, snapshotter) to the driver |
| `csi_plugin_operations_seconds` | histogram | same | csi-lib-utils (subsystem `csi_plugin`) **(derived)** | Same metric emitted from the driver side, if the driver uses csi-lib-utils |
| `csi_operations_seconds` | histogram, ALPHA | driver_name, grpc_status_code, method_name, migrated | Kubernetes Metrics Reference (kubelet /metrics) | Kubelet-side CSI call duration (NodeStage, NodePublish, ...) by gRPC code |
| `storage_operation_duration_seconds` | histogram, BETA | migrated, operation_name, status, volume_plugin | Kubernetes Metrics Reference (kubelet) | Volume operation duration with status |
| `volume_operation_total_seconds` | histogram, BETA | operation_name, plugin_name | Kubernetes Metrics Reference (kubelet) | End-to-end volume operation duration |

**Suggested**
- **R**: error rate from `csi_sidecar_operations_seconds_count{grpc_status_code!="OK"}` and `csi_operations_seconds_count{grpc_status_code!="OK"}`, plus `storage_operation_duration_seconds` by `status`.
- **C**: the per-method latency buckets.
