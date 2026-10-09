# Kubernetes core component metrics catalogue (for metric-tree design)

**Source version:** Kubernetes Metrics Reference **v1.37** (page auto-generated 2026-08-28), from
`kubernetes/website` `main` branch, `content/en/docs/reference/instrumentation/metrics.md`
(rendered at https://kubernetes.io/docs/reference/instrumentation/metrics/). Rename/removal
analysis compares against the same file on the `release-1.32` and `release-1.34` branches.
Go/process runtime metric names come from `prometheus/client_golang` (`process_collector.go`,
`go_collector.go`) because the Kubernetes reference does not list them.

## Conventions

- Names are the exact exposition names from the reference. For histograms the base name is given;
  Prometheus exposes `<name>_bucket`, `<name>_sum`, `<name>_count`.
- **R** in front of a name = my suggested *root* (always-collected) candidate; everything else is a
  candidate *child* metric (collect during incidents). This is a design suggestion, not upstream data.
- Type "custom collector" = the reference's "Custom" type (hand-written collectors on kubelet
  `/metrics/resource`; exposed as gauges/counters, e.g. `*_seconds_total` is a counter).
- Stability: STABLE (API contract, no label changes), BETA, ALPHA (may change or disappear in any release).
- "also: X" = the reference lists the same metric for another component too.
- Within each component, tables are sorted STABLE, BETA, then ALPHA, then by name.
- Scope is kube-apiserver, kube-scheduler, kube-controller-manager, kubelet (`/metrics`,
  `/metrics/resource`, `/metrics/probes`, no cAdvisor), kube-proxy, and the shared families.
  Metrics listed only for cloud-controller-manager are left out.
- A few info/config/internal-bookkeeping metrics are left out of the tables. Their names are listed
  under each table.

## Endpoints

- All components: `/metrics` (and `/metrics/slis`, which serves `kubernetes_healthcheck` and
  `kubernetes_healthchecks_total`).
- kubelet: `/metrics` (kubelet itself), `/metrics/resource` (the STABLE resource metrics
  metrics-server uses), `/metrics/probes` (`prober_probe_total`, `prober_probe_duration_seconds`),
  and `/metrics/cadvisor` (not covered here). PSI (pressure stall) metrics are served on
  `/metrics/cadvisor` only (system-metrics page), so they are not listed here.
- Shared families: the reference lists every component for these. What actually shows up depends on
  what the binary uses. For example, `leader_election_*` is only meaningful on components that run
  leader election (scheduler and controller-manager), and `workqueue_*` only appears for queues that
  exist in that binary.

## Deprecated / renamed metrics (flag these in dashboards and alerts)

### Marked deprecated in the v1.37 reference (still emitted)

| Old metric | Deprecated since | Replacement / note |
|---|---|---|
| `apiserver_storage_objects` (STABLE, gauge) | 1.34 | `apiserver_resource_objects` (help text says "consider using apiserver_resource_objects instead"; labels are now `group, resource` instead of `resource`) |
| `scrape_error` (kubelet /metrics/resource) | 1.29 | `resource_scrape_error` (STABLE), same help text |
| `volume_operation_total_errors` (KCM) | 1.36 | `volume_operation_errors_total` (same labels `operation_name, plugin_name`) |
| `etcd_bookmark_counts` (apiserver, gauge) | 1.36 | `etcd_bookmark_total` |
| `apiserver_cache_list_total`, `apiserver_cache_list_fetched_objects_total`, `apiserver_cache_list_returned_objects_total` | 1.37 | No replacement named in the reference. `apiserver_storage_list_*` now has a `storage` label, so it is the likely successor (my inference) |
| `scheduler_preemption_goroutines_duration_seconds` | 1.37 | No replacement named. Related metrics still present: `scheduler_preemption_goroutines_execution_total`, `scheduler_preemption_execution_duration_seconds` |

### Present in v1.32/v1.34 references, absent in v1.37 (removed; successors present in v1.37)

| Removed metric | Deprecated in | Successor in v1.37 |
|---|---|---|
| `scheduler_pod_scheduling_duration_seconds` (was STABLE) | 1.29 | `scheduler_pod_scheduling_sli_duration_seconds` (BETA) |
| `apiserver_request_slo_duration_seconds` | 1.27 | `apiserver_request_sli_duration_seconds` |
| `apiserver_storage_db_total_size_in_bytes` | 1.28 | `apiserver_storage_size_bytes` (STABLE) |
| `apiserver_flowcontrol_request_concurrency_limit` | 1.30 | `apiserver_flowcontrol_nominal_limit_seats` |
| `apiserver_flowcontrol_request_concurrency_in_use` | 1.31 | `apiserver_flowcontrol_current_executing_seats` |
| `apiserver_encryption_config_controller_automatic_reload_success_total` / `_failures_total` | 1.30 | `apiserver_encryption_config_controller_automatic_reloads_total` (+ `_last_timestamp_seconds`) |
| `kube_apiserver_pod_logs_pods_logs_backend_tls_failure_total`, `kube_apiserver_pod_logs_pods_logs_insecure_backend_total` | 1.27 | `kube_apiserver_pod_logs_backend_tls_failure_total`, `kube_apiserver_pod_logs_insecure_backend_total` |
| `kubelet_credential_provider_plugin_errors` | - | `kubelet_credential_provider_plugin_errors_total` |
| `scheduler_scheduler_cache_size` | - | `scheduler_cache_size` |
| `resourceclaim_controller_*` (`resource_claims`, `allocated_resource_claims`, `create_attempts_total`, `create_failures_total`, `creates_total`) | - | `dynamic_resource_allocation_resource_claims`, `dynamic_resource_allocation_resourceclaim_creates_total` |
| `etcd_version_info` | - | none (`version_info` is a different, component compatibility-version gauge) |
| `kubelet_volume_stats_health_status_abnormal` | - | none listed. It is missing from the v1.37 reference; check on a live kubelet before you rely on it |

Successors were matched by name and help text similarity between versions, except where the
reference's own help text names the replacement (`apiserver_storage_objects`). Treat the matches as
high-confidence mappings, not upstream-declared ones.

## Go runtime and process metrics (all components)

These come from the standard Prometheus client_golang collectors that every component registers.
They are **not** in the Kubernetes metrics reference, and they have no Kubernetes stability level.
Names were checked against the client_golang source.

| Metric | Type | Meaning |
|---|---|---|
| **R** `process_cpu_seconds_total` | counter | Total user+system CPU seconds of the process |
| **R** `process_resident_memory_bytes` | gauge | Resident memory size |
| `process_virtual_memory_bytes` / `process_virtual_memory_max_bytes` | gauge | Virtual memory size / limit |
| **R** `process_open_fds` / `process_max_fds` | gauge | Open file descriptors / limit (FD saturation) |
| `process_start_time_seconds` | gauge | Process start time. Detects restarts |
| `process_network_receive_bytes_total` / `process_network_transmit_bytes_total` | counter | Process network I/O (newer client_golang, Linux) |
| **R** `go_goroutines` | gauge | Number of goroutines (leak / saturation signal) |
| `go_threads` | gauge | OS threads created |
| `go_gc_duration_seconds` | summary | GC pause durations |
| `go_memstats_heap_alloc_bytes`, `go_memstats_heap_inuse_bytes`, `go_memstats_heap_idle_bytes`, `go_memstats_heap_sys_bytes`, `go_memstats_heap_objects`, `go_memstats_next_gc_bytes`, `go_memstats_alloc_bytes`, `go_memstats_alloc_bytes_total`, `go_memstats_sys_bytes`, `go_memstats_last_gc_time_seconds`, `go_memstats_mallocs_total`, `go_memstats_frees_total`, `go_memstats_stack_inuse_bytes` | gauge/counter | Go heap/memory statistics (child metrics for memory incidents) |
| `go_info` | gauge | Go version info |

## kube-apiserver

| Metric | Type | Stability | Key labels | Meaning |
|---|---|---|---|---|
| `apiserver_admission_controller_admission_duration_seconds` | histogram | STABLE | name, operation, rejected, type | Admission controller latency histogram in seconds, identified by name and broken out for each operation and API resource and type (validate or admit). |
| `apiserver_admission_step_admission_duration_seconds` | histogram | STABLE | operation, rejected, type | Admission sub-step latency histogram in seconds, broken out for each operation and API resource and step type (validate or admit). |
| **R** `apiserver_admission_webhook_admission_duration_seconds` | histogram | STABLE | name, operation, rejected, type | Admission webhook latency histogram in seconds, identified by name and broken out for each operation and API resource and type (validate or admit). |
| **R** `apiserver_current_inflight_requests` | gauge | STABLE | request_kind | Maximal number of currently used inflight request limit of this apiserver per request kind in last second. |
| **R** `apiserver_longrunning_requests` | gauge | STABLE | component, group, resource, scope, subresource, verb, version | Gauge of all active long-running apiserver requests broken out by verb, group, version, resource, scope and component. |
| **R** `apiserver_request_duration_seconds` | histogram | STABLE | component, dry_run, group, resource, scope, subresource, verb, version | Response latency distribution in seconds for each verb, dry run value, group, version, resource, subresource, scope and component. |
| **R** `apiserver_request_total` | counter | STABLE | code, component, dry_run, group, resource, scope, subresource, verb, version | Counter of apiserver requests broken out for each verb, dry run value, group, version, resource, scope, component, and HTTP response code. |
| `apiserver_requested_deprecated_apis` | gauge | STABLE | group, removed_release, resource, subresource, version | Gauge of deprecated APIs that have been requested, broken out by API group, version, resource, subresource, and removed_release. |
| `apiserver_response_sizes` | histogram | STABLE | component, group, resource, scope, subresource, verb, version | Response size distribution in bytes for each group, version, verb, resource, subresource, scope and component. |
| `apiserver_storage_objects` | gauge | STABLE | resource | [DEPRECATED, consider using apiserver_resource_objects instead] Number of stored objects at the time of last check split by kind. **DEPRECATED since 1.34.0** |
| **R** `apiserver_storage_size_bytes` | custom collector (gauge/counter) | STABLE | storage_cluster_id | Size of the storage database file physically allocated in bytes. |
| `apiserver_authentication_config_controller_automatic_reload_last_timestamp_seconds` | gauge | BETA | apiserver_id_hash, status | Timestamp of the last automatic reload of authentication configuration split by status and apiserver identity. |
| `apiserver_authentication_config_controller_automatic_reloads_total` | counter | BETA | apiserver_id_hash, status | Total number of automatic reloads of authentication configuration split by status and apiserver identity. |
| `apiserver_authorization_config_controller_automatic_reload_last_timestamp_seconds` | gauge | BETA | apiserver_id_hash, status | Timestamp of the last automatic reload of authorization configuration split by status and apiserver identity. |
| `apiserver_authorization_config_controller_automatic_reloads_total` | counter | BETA | apiserver_id_hash, status | Total number of automatic reloads of authorization configuration split by status and apiserver identity. |
| `apiserver_cel_compilation_duration_seconds` | histogram | BETA | - | CEL compilation time in seconds. |
| `apiserver_cel_evaluation_duration_seconds` | histogram | BETA | - | CEL evaluation time in seconds. |
| `apiserver_flowcontrol_current_executing_requests` | gauge | BETA | flow_schema, priority_level | Number of requests in initial (for a WATCH) or any (for a non-WATCH) execution stage in the API Priority and Fairness subsystem |
| `apiserver_flowcontrol_current_executing_seats` | gauge | BETA | flow_schema, priority_level | Concurrency (number of seats) occupied by the currently executing (initial stage for a WATCH, any stage otherwise) requests in the API Priority and Fairness subsystem |
| **R** `apiserver_flowcontrol_current_inqueue_requests` | gauge | BETA | flow_schema, priority_level | Number of requests currently pending in queues of the API Priority and Fairness subsystem |
| `apiserver_flowcontrol_dispatched_requests_total` | counter | BETA | flow_schema, priority_level | Number of requests executed by API Priority and Fairness subsystem |
| `apiserver_flowcontrol_nominal_limit_seats` | gauge | BETA | priority_level | Nominal number of execution seats configured for each priority level |
| **R** `apiserver_flowcontrol_rejected_requests_total` | counter | BETA | flow_schema, priority_level, reason | Number of requests rejected by API Priority and Fairness subsystem |
| **R** `apiserver_flowcontrol_request_wait_duration_seconds` | histogram | BETA | execute, flow_schema, priority_level | Length of time a request spent waiting in its queue |
| `apiserver_storage_events_received_total` | counter | BETA | group, resource | Number of etcd events received split by kind. |
| `apiserver_validating_admission_policy_check_duration_seconds` | histogram | BETA | enforcement_action, error_type, policy, policy_binding | Validation admission latency for individual validation expressions in seconds, labeled by policy and further including binding and enforcement action taken. |
| `apiserver_validating_admission_policy_check_total` | counter | BETA | enforcement_action, error_type, policy, policy_binding | Validation admission policy check total, labeled by policy and further identified by binding and enforcement action taken. |
| `apiserver_validation_declarative_validation_mismatch_total` | counter | BETA | - | Number of times declarative validation results differed from handwritten validation results for core types. |
| `apiserver_validation_declarative_validation_panic_total` | counter | BETA | - | Number of times declarative validation has panicked during validation. |
| `apiserver_watch_events_sizes` | histogram | BETA | group, resource, version | Watch event size distribution in bytes |
| `apiserver_watch_events_total` | counter | BETA | group, resource, version | Number of events sent in watch clients |
| `apiserver_watch_list_duration_seconds` | histogram | BETA | group, resource, scope, version | Response latency distribution in seconds for watch list requests broken by group, version, resource and scope. |
| `apiserver_webhooks_x509_insecure_sha1_total` | counter | BETA | - | Counts the number of requests to servers with insecure SHA1 signatures in their serving certificate OR the number of connection failures due to the insecure SHA1 signa... |
| `apiserver_webhooks_x509_missing_san_total` | counter | BETA | - | Counts the number of requests to servers missing SAN extension in their serving certificate OR the number of connection failures due to the lack of x509 certificate SA... |
| `serviceaccount_legacy_tokens_total` | counter | BETA | - | Cumulative legacy service account tokens used |
| `serviceaccount_stale_tokens_total` | counter | BETA | - | Cumulative stale projected service account tokens used |
| `serviceaccount_valid_tokens_total` | counter | BETA | - | Cumulative valid projected service account tokens used |
| `aggregator_discovery_aggregation_count_total` | counter | ALPHA | - | Counter of number of times discovery was aggregated |
| `aggregator_discovery_nopeer_requests_total` | counter | ALPHA | - | Counter of number of times no-peer (non peer-aggregated) discovery was requested |
| `aggregator_discovery_peer_aggregated_cache_hits_total` | counter | ALPHA | - | Counter of number of times discovery was served from peer-aggregated cache |
| `aggregator_discovery_peer_aggregated_cache_misses_total` | counter | ALPHA | - | Counter of number of times discovery was aggregated across all API servers |
| **R** `aggregator_unavailable_apiservice` | custom collector (gauge/counter) | ALPHA | name | Gauge of APIServices which are marked as unavailable broken down by APIService name. |
| `aggregator_unavailable_apiservice_total` | counter | ALPHA | name, reason | Counter of APIServices which are marked as unavailable broken down by APIService name and reason. |
| `apiextensions_apiserver_validation_ratcheting_seconds` | histogram | ALPHA | - | Time for comparison of old to new for the purposes of CRDValidationRatcheting during an UPDATE in seconds. |
| `apiserver_admission_match_condition_evaluation_errors_total` | counter | ALPHA | kind, name, operation, type | Admission match condition evaluation errors count, identified by name of resource containing the match condition and broken out for each kind containing matchCondition... |
| `apiserver_admission_match_condition_evaluation_seconds` | histogram | ALPHA | kind, name, operation, type | Admission match condition evaluation time in seconds, identified by name and broken out for each kind containing matchConditions (webhook or policy), operation and typ... |
| `apiserver_admission_match_condition_exclusions_total` | counter | ALPHA | kind, name, operation, type | Admission match condition evaluation exclusions count, identified by name of resource containing the match condition and broken out for each kind containing matchCondi... |
| `apiserver_admission_step_admission_duration_seconds_summary` | summary | ALPHA | operation, rejected, type | Admission sub-step latency summary in seconds, broken out for each operation and API resource and step type (validate or admit). |
| `apiserver_admission_webhook_fail_open_count` | counter | ALPHA | name, type | Admission webhook fail open count, identified by name and broken out for each admission type (validating or admit). |
| **R** `apiserver_admission_webhook_rejection_count` | counter | ALPHA | error_type, name, operation, rejection_code, type | Admission webhook rejection count, identified by name and broken out for each admission type (validating or admit) and operation. |
| `apiserver_admission_webhook_request_total` | counter | ALPHA | code, name, operation, rejected, type | Admission webhook request total, identified by name and broken out for each admission type (validating or admit) and operation. |
| `apiserver_audit_error_total` | counter | ALPHA | plugin | Counter of audit events that failed to be audited properly. |
| `apiserver_audit_event_total` | counter | ALPHA | - | Counter of audit events generated and sent to the audit backend. |
| `apiserver_audit_level_total` | counter | ALPHA | level | Counter of policy levels for audit events (1 per request). |
| `apiserver_audit_requests_rejected_total` | counter | ALPHA | - | Counter of apiserver requests rejected due to an error in audit logging backend. |
| `apiserver_authentication_jwt_authenticator_jwks_fetch_last_timestamp_seconds` | gauge | ALPHA | apiserver_id_hash, jwt_issuer_hash, result | Timestamp of the last successful or failed JWKS fetch split by result, api server identity and jwt issuer for the JWT authenticator. |
| `apiserver_authentication_jwt_authenticator_latency_seconds` | histogram | ALPHA | jwt_issuer_hash, result | Latency of jwt authentication operations in seconds. |
| `apiserver_authorization_decisions_total` | counter | ALPHA | decision, name, type | Total number of terminal decisions made by an authorizer split by authorizer type, name, and decision. |
| `apiserver_authorization_match_condition_evaluation_errors_total` | counter | ALPHA | name, type | Total number of errors when an authorization webhook encounters a match condition error split by authorizer type and name. |
| `apiserver_authorization_match_condition_evaluation_seconds` | histogram | ALPHA | name, type | Authorization match condition evaluation time in seconds, split by authorizer type and name. |
| `apiserver_authorization_match_condition_exclusions_total` | counter | ALPHA | name, type | Total number of exclusions when an authorization webhook is skipped because match conditions exclude it. |
| `apiserver_authorization_webhook_duration_seconds` | histogram | ALPHA | name, result | Request latency in seconds. |
| `apiserver_authorization_webhook_evaluations_fail_open_total` | counter | ALPHA | name, result | NoOpinion results due to webhook timeout or error. |
| `apiserver_authorization_webhook_evaluations_total` | counter | ALPHA | name, result | Round-trips to authorization webhooks. |
| `apiserver_cache_list_fetched_objects_total` | counter | ALPHA | group, index, resource | Number of objects read from watch cache in the course of serving a LIST request **DEPRECATED since 1.37.0** |
| `apiserver_cache_list_returned_objects_total` | counter | ALPHA | group, resource | Number of objects returned for a LIST request from watch cache **DEPRECATED since 1.37.0** |
| `apiserver_cache_list_total` | counter | ALPHA | group, index, resource | Number of LIST requests served from watch cache **DEPRECATED since 1.37.0** |
| `apiserver_certificates_registry_csr_honored_duration_total` | counter | ALPHA | signerName | Total number of issued CSRs with a requested duration that was honored, sliced by signer (only kubernetes.io signer names are specifically identified) |
| `apiserver_certificates_registry_csr_requested_duration_total` | counter | ALPHA | signerName | Total number of issued CSRs with a requested duration, sliced by signer (only kubernetes.io signer names are specifically identified) |
| **R** `apiserver_client_certificate_expiration_seconds` | histogram | ALPHA | - | Distribution of the remaining lifetime on the certificate used to authenticate a request. |
| `apiserver_clusterip_repair_ip_errors_total` | counter | ALPHA | type | Number of errors detected on clusterips by the repair loop broken down by type of error: leak, repair, full, outOfRange, duplicate, unknown, invalid |
| `apiserver_clusterip_repair_reconcile_errors_total` | counter | ALPHA | - | Number of reconciliation failures on the clusterip repair reconcile loop |
| `apiserver_conversion_webhook_duration_seconds` | histogram | ALPHA | failure_type, result | Conversion webhook request latency |
| `apiserver_conversion_webhook_request_total` | counter | ALPHA | failure_type, result | Counter for conversion webhook requests with success/failure and failure error type |
| `apiserver_crd_conversion_webhook_duration_seconds` | histogram | ALPHA | crd_name, from_version, succeeded, to_version | CRD webhook conversion duration in seconds |
| `apiserver_current_inqueue_requests` | gauge | ALPHA | request_kind | Maximal number of queued requests in this apiserver per request kind in last second. |
| `apiserver_delegated_authn_request_duration_seconds` | histogram | ALPHA | code | Request latency in seconds. |
| `apiserver_delegated_authn_request_total` | counter | ALPHA | code | Number of HTTP requests partitioned by status code. |
| `apiserver_delegated_authz_request_duration_seconds` | histogram | ALPHA | code | Request latency in seconds. |
| `apiserver_delegated_authz_request_total` | counter | ALPHA | code | Number of HTTP requests partitioned by status code. |
| `apiserver_egress_dialer_dial_duration_seconds` | histogram | ALPHA | protocol, transport | Dial latency histogram in seconds, labeled by the protocol (http-connect or grpc), transport (tcp or uds) |
| `apiserver_egress_dialer_dial_failure_count` | counter | ALPHA | protocol, stage, transport | Dial failure count, labeled by the protocol (http-connect or grpc), transport (tcp or uds), and stage (connect or proxy). |
| `apiserver_egress_dialer_dial_start_total` | counter | ALPHA | protocol, transport | Dial starts, labeled by the protocol (http-connect or grpc) and transport (tcp or uds). |
| `apiserver_encryption_config_controller_automatic_reload_last_timestamp_seconds` | gauge | ALPHA | apiserver_id_hash, status | Timestamp of the last successful or failed automatic reload of encryption configuration split by apiserver identity. |
| `apiserver_encryption_config_controller_automatic_reloads_total` | counter | ALPHA | apiserver_id_hash, status | Total number of reload successes and failures of encryption configuration split by apiserver identity. |
| `apiserver_envelope_encryption_dek_cache_fill_percent` | gauge | ALPHA | - | Percent of the cache slots currently occupied by cached DEKs. |
| `apiserver_envelope_encryption_dek_cache_inter_arrival_time_seconds` | histogram | ALPHA | transformation_type | Time (in seconds) of inter arrival of transformation requests. |
| `apiserver_envelope_encryption_dek_source_cache_size` | gauge | ALPHA | provider_name | Number of records in data encryption key (DEK) source cache. |
| `apiserver_envelope_encryption_invalid_key_id_from_status_total` | counter | ALPHA | error, provider_name | Number of times an invalid keyID is returned by the Status RPC call split by error. |
| `apiserver_envelope_encryption_key_id_hash_last_timestamp_seconds` | gauge | ALPHA | apiserver_id_hash, key_id_hash, provider_name, transformation_type | The last time in seconds when a keyID was used. |
| `apiserver_envelope_encryption_key_id_hash_status_last_timestamp_seconds` | gauge | ALPHA | apiserver_id_hash, key_id_hash, provider_name | The last time in seconds when a keyID was returned by the Status RPC call. |
| `apiserver_envelope_encryption_key_id_hash_total` | counter | ALPHA | apiserver_id_hash, key_id_hash, provider_name, transformation_type | Number of times a keyID is used split by transformation type, provider, and apiserver identity. |
| `apiserver_envelope_encryption_kms_operations_latency_seconds` | histogram | ALPHA | grpc_status_code, method_name, provider_name | KMS operation duration with gRPC error code status total. |
| `apiserver_externaljwt_fetch_keys_data_timestamp` | gauge | ALPHA | - | Unix Timestamp in seconds of the last successful FetchKeys data_timestamp value returned by the external signer |
| `apiserver_externaljwt_fetch_keys_request_total` | counter | ALPHA | code | Total attempts at syncing supported JWKs |
| `apiserver_externaljwt_fetch_keys_success_timestamp` | gauge | ALPHA | - | Unix Timestamp in seconds of the last successful FetchKeys request |
| `apiserver_externaljwt_request_duration_seconds` | histogram | ALPHA | code, method | Request duration and time for calls to external-jwt-signer |
| `apiserver_externaljwt_sign_request_total` | counter | ALPHA | code | Total attempts at signing JWT |
| `apiserver_flowcontrol_current_inqueue_seats` | gauge | ALPHA | flow_schema, priority_level | Number of seats currently pending in queues of the API Priority and Fairness subsystem |
| `apiserver_flowcontrol_current_limit_seats` | gauge | ALPHA | priority_level | current derived number of execution seats available to each priority level |
| `apiserver_flowcontrol_demand_seats` | histogram (timing-ratio) | ALPHA | priority_level | Observations, at the end of every nanosecond, of (the number of seats each priority level could use) / (nominal number of seats for that level) |
| `apiserver_flowcontrol_demand_seats_average` | gauge | ALPHA | priority_level | Time-weighted average, over last adjustment period, of demand_seats |
| `apiserver_flowcontrol_demand_seats_high_watermark` | gauge | ALPHA | priority_level | High watermark, over last adjustment period, of demand_seats |
| `apiserver_flowcontrol_demand_seats_smoothed` | gauge | ALPHA | priority_level | Smoothed seat demands |
| `apiserver_flowcontrol_demand_seats_stdev` | gauge | ALPHA | priority_level | Time-weighted standard deviation, over last adjustment period, of demand_seats |
| `apiserver_flowcontrol_lower_limit_seats` | gauge | ALPHA | priority_level | Configured lower bound on number of execution seats available to each priority level |
| `apiserver_flowcontrol_priority_level_request_utilization` | histogram (timing-ratio) | ALPHA | phase, priority_level | Observations, at the end of every nanosecond, of number of requests (as a fraction of the relevant limit) waiting or in any stage of execution (but only initial stage ... |
| `apiserver_flowcontrol_priority_level_seat_utilization` | histogram (timing-ratio) | ALPHA | priority_level, phase:executing | Observations, at the end of every nanosecond, of utilization of seats for any stage of execution (but only initial stage for WATCHes) |
| `apiserver_flowcontrol_read_vs_write_current_requests` | histogram (timing-ratio) | ALPHA | phase, request_kind | Observations, at the end of every nanosecond, of the number of requests (as a fraction of the relevant limit) waiting or in regular stage of execution |
| `apiserver_flowcontrol_request_dispatch_no_accommodation_total` | counter | ALPHA | flow_schema, priority_level | Number of times a dispatch attempt resulted in a non accommodation due to lack of available seats |
| `apiserver_flowcontrol_request_execution_seconds` | histogram | ALPHA | flow_schema, priority_level, type | Duration of initial stage (for a WATCH) or any (for a non-WATCH) stage of request execution in the API Priority and Fairness subsystem |
| `apiserver_flowcontrol_request_queue_length_after_enqueue` | histogram | ALPHA | flow_schema, priority_level | Length of queue in the API Priority and Fairness subsystem, as seen by each request after it is enqueued |
| `apiserver_flowcontrol_target_seats` | gauge | ALPHA | priority_level | Seat allocation targets |
| `apiserver_flowcontrol_upper_limit_seats` | gauge | ALPHA | priority_level | Configured upper bound on number of execution seats available to each priority level |
| `apiserver_flowcontrol_work_estimated_seats` | histogram | ALPHA | flow_schema, priority_level | Number of estimated seats (maximum of initial and final seats) associated with requests in API Priority and Fairness |
| `apiserver_impersonation_attempts_duration_seconds` | histogram | ALPHA | decision, mode | Latency of impersonation attempts in seconds split by mode and decision. |
| `apiserver_impersonation_attempts_total` | counter | ALPHA | decision, mode | Total number of impersonation attempts split by mode and decision. |
| `apiserver_impersonation_authorization_attempts_duration_seconds` | histogram | ALPHA | decision, mode | Latency of authorization checks made by the impersonation handler in seconds split by mode and decision. |
| `apiserver_impersonation_authorization_attempts_total` | counter | ALPHA | decision, mode | Total number of authorization checks made by the impersonation handler split by mode and decision. |
| `apiserver_init_events_total` | counter | ALPHA | group, resource | Counter of init events processed in watch cache broken by resource type. |
| `apiserver_kube_aggregator_x509_insecure_sha1_total` | counter | ALPHA | - | Counts the number of requests to servers with insecure SHA1 signatures in their serving certificate OR the number of connection failures due to the insecure SHA1 signa... |
| `apiserver_kube_aggregator_x509_missing_san_total` | counter | ALPHA | - | Counts the number of requests to servers missing SAN extension in their serving certificate OR the number of connection failures due to the lack of x509 certificate SA... |
| `apiserver_manifest_admission_config_controller_automatic_reload_last_timestamp_seconds` | gauge | ALPHA | apiserver_id_hash, plugin, status | Timestamp of the last automatic reload of admission manifest configuration split by status, plugin, and apiserver identity. |
| `apiserver_manifest_admission_config_controller_automatic_reloads_total` | counter | ALPHA | apiserver_id_hash, plugin, status | Total number of automatic reloads of admission manifest configuration split by status, plugin, and apiserver identity. |
| `apiserver_mutating_admission_policy_check_duration_seconds` | histogram | ALPHA | error_type, policy, policy_binding | Mutation admission latency for individual mutation expressions in seconds, labeled by policy and binding. |
| `apiserver_mutating_admission_policy_check_total` | counter | ALPHA | error_type, policy, policy_binding | Mutation admission policy check total, labeled by policy and further identified by binding. |
| `apiserver_nodeport_repair_port_errors_total` | counter | ALPHA | type | Number of errors detected on ports by the repair loop broken down by type of error: leak, repair, full, outOfRange, duplicate, unknown |
| `apiserver_nodeport_repair_reconcile_errors_total` | counter | ALPHA | - | Number of reconciliation failures on the nodeport repair reconcile loop |
| `apiserver_peer_discovery_sync_errors_total` | counter | ALPHA | type | Total number of errors encountered while syncing discovery information from a peer kube-apiserver |
| `apiserver_peer_proxy_errors_total` | counter | ALPHA | group, resource, type, version | Total number of errors encountered while proxying requests to a peer kube apiserver |
| `apiserver_request_aborts_total` | counter | ALPHA | group, resource, scope, subresource, verb, version | Number of requests which apiserver aborted possibly due to a timeout, for each group, version, verb, resource, subresource and scope |
| `apiserver_request_body_size_bytes` | histogram | ALPHA | group, resource, verb | Apiserver request body size in bytes broken out by resource and verb. |
| `apiserver_request_filter_duration_seconds` | histogram | ALPHA | filter | Request filter latency distribution in seconds, for each filter type |
| `apiserver_request_post_timeout_total` | counter | ALPHA | source, status | Tracks the activity of the request handlers after the associated requests have been timed out by the apiserver |
| **R** `apiserver_request_sli_duration_seconds` | histogram | ALPHA | component, group, resource, scope, subresource, verb, version | Response latency distribution (not counting webhook duration and priority & fairness queue wait times) in seconds for each verb, group, version, resource, subresource,... |
| **R** `apiserver_request_terminations_total` | counter | ALPHA | code, component, group, resource, scope, subresource, verb, version | Number of requests which apiserver terminated in self-defense. |
| `apiserver_rerouted_request_total` | counter | ALPHA | code, group, resource, version | Total number of requests that were proxied to a peer kube-apiserver because the local apiserver was not capable of serving it, broken down by 'group', 'version', and '... |
| **R** `apiserver_resource_objects` | gauge | ALPHA | group, resource | Number of stored objects at the time of last check split by kind. |
| `apiserver_resource_size_estimate_bytes` | gauge | ALPHA | group, resource | Estimated size of stored objects in database. |
| `apiserver_selfrequest_total` | counter | ALPHA | group, resource, subresource, verb | Counter of apiserver self-requests broken out for each verb, API resource and subresource. |
| `apiserver_storage_consistency_checks_total` | counter | ALPHA | group, resource, status | Counter for status of consistency checks between etcd and watch cache |
| `apiserver_storage_data_key_generation_duration_seconds` | histogram | ALPHA | - | Latencies in seconds of data encryption key(DEK) generation operations. |
| `apiserver_storage_data_key_generation_failures_total` | counter | ALPHA | - | Total number of failed data encryption key(DEK) generation operations. |
| `apiserver_storage_decode_errors_total` | counter | ALPHA | group, resource | Number of stored object decode errors split by object type |
| `apiserver_storage_envelope_transformation_cache_misses_total` | counter | ALPHA | - | Total number of cache misses while accessing key decryption key(KEK). |
| `apiserver_storage_list_duration_seconds` | histogram | ALPHA | group, resource, streamed | Latency of the storage layer GetList call in seconds, including object decode, split by whether etcd RangeStream was used. |
| `apiserver_storage_list_evaluated_objects_total` | counter | ALPHA | group, resource, storage | Number of objects tested in the course of serving a LIST request from storage |
| `apiserver_storage_list_fetched_objects_total` | counter | ALPHA | group, index, resource, storage | Number of objects read from storage in the course of serving a LIST request |
| `apiserver_storage_list_returned_objects_total` | counter | ALPHA | group, resource, storage | Number of objects returned for a LIST request from storage |
| `apiserver_storage_list_total` | counter | ALPHA | group, index, resource, storage | Number of LIST requests served from storage |
| `apiserver_storage_transformation_duration_seconds` | histogram | ALPHA | transformation_type, transformer_prefix | Latencies in seconds of value transformation operations. |
| `apiserver_storage_transformation_operations_total` | counter | ALPHA | resource, status, transformation_type, transformer_prefix | Total number of transformations. |
| `apiserver_stream_translator_requests_total` | counter | ALPHA | code | Total number of requests that were handled by the StreamTranslatorProxy, which processes streaming RemoteCommand/V5 |
| `apiserver_stream_tunnel_requests_total` | counter | ALPHA | code | Total number of requests that were handled by the StreamTunnelProxy, which processes streaming PortForward/V2 |
| `apiserver_terminated_watchers_total` | counter | ALPHA | group, resource | Counter of watchers closed due to unresponsiveness broken by resource type. |
| **R** `apiserver_tls_handshake_errors_total` | counter | ALPHA | - | Number of requests dropped with 'TLS handshake error from' error |
| `apiserver_validation_declarative_validation_panics_total` | counter | ALPHA | validation_identifier | Number of panics in declarative validation, broken down by validation identifier. |
| `apiserver_validation_declarative_validation_parity_discrepancies_total` | counter | ALPHA | validation_identifier | Number of discrepancies between declarative and handwritten validation, broken down by validation identifier. |
| `apiserver_watch_cache_consistent_read_total` | counter | ALPHA | fallback, group, resource, success | Counter for consistent reads from cache. |
| `apiserver_watch_cache_events_dispatched_total` | counter | ALPHA | group, resource | Counter of events dispatched in watch cache broken by resource type. |
| `apiserver_watch_cache_events_received_total` | counter | ALPHA | group, resource | Counter of events received in watch cache broken by resource type. |
| `apiserver_watch_cache_initialization_duration_seconds` | histogram | ALPHA | group, resource | Histogram of watch cache initialization duration in seconds, broken by resource type. |
| `apiserver_watch_cache_initialization_errors_total` | counter | ALPHA | group, resource | Counter of watch cache initialization errors broken by resource type. |
| `apiserver_watch_cache_initializations_total` | counter | ALPHA | group, resource | Counter of watch cache initializations broken by resource type. |
| `apiserver_watch_cache_read_wait_seconds` | histogram | ALPHA | group, resource | Histogram of time spent waiting for a watch cache to become fresh. |
| `apiserver_watch_cache_resource_version` | gauge | ALPHA | group, resource | Current resource version of watch cache broken by resource type. |
| `apiserver_watch_events_dispatch_duration_seconds` | histogram | ALPHA | group, resource, stage | Histogram of watch event dispatch latency broken by resource type and pipeline stage. |
| `apiserver_watch_filtered_events_total` | counter | ALPHA | group, resource | Counter of events filtered out by shard selector during watch dispatch, broken by resource type. |
| `apiserver_watch_shards_total` | gauge | ALPHA | group, resource | Number of active sharded watch connections broken by resource type. |
| `apiserver_websocket_streaming_requests_total` | counter | ALPHA | proxy_type, subresource | Total number of WebSocket streaming requests (exec/attach/portforward) routed by the API server, labeled by subresource and proxy_type. |
| `authenticated_user_requests` | counter | ALPHA | username | Counter of authenticated requests broken out by username. |
| `authentication_attempts` | counter | ALPHA | result | Counter of authenticated attempts. |
| `authentication_duration_seconds` | histogram | ALPHA | result | Authentication duration in seconds broken out by result. |
| `authentication_token_cache_active_fetch_count` | gauge | ALPHA | status |  |
| `authentication_token_cache_fetch_total` | counter | ALPHA | status |  |
| `authentication_token_cache_request_duration_seconds` | histogram | ALPHA | status |  |
| `authentication_token_cache_request_total` | counter | ALPHA | status |  |
| `authorization_attempts_total` | counter | ALPHA | result | Counter of authorization attempts broken down by result. |
| `authorization_duration_seconds` | histogram | ALPHA | result | Authorization duration in seconds broken out by result. |
| `etcd_bookmark_counts` | gauge | ALPHA | group, resource | Number of etcd bookmarks (progress notify events) split by kind. **DEPRECATED since 1.36.0** |
| `etcd_bookmark_total` | counter | ALPHA | group, resource | Number of etcd bookmarks (progress notify events) split by kind. |
| `etcd_lease_object_counts` | histogram | ALPHA | - | Number of objects attached to a single etcd lease. |
| **R** `etcd_request_duration_seconds` | histogram | ALPHA | group, operation, resource | Etcd request latency in seconds for each operation and object type. |
| **R** `etcd_request_errors_total` | counter | ALPHA | group, operation, resource | Etcd failed request counts for each operation and object type. |
| `etcd_requests_total` | counter | ALPHA | group, operation, resource | Etcd request counts for each operation and object type. |
| `field_validation_request_duration_seconds` | histogram | ALPHA | field_validation | Response latency distribution in seconds for each field validation value |
| `kube_apiserver_clusterip_allocator_allocated_ips` | gauge | ALPHA | cidr | Gauge measuring the number of allocated IPs for Services |
| `kube_apiserver_clusterip_allocator_allocation_duration_seconds` | histogram | ALPHA | cidr | Duration in seconds to allocate a Cluster IP by ServiceCIDR |
| `kube_apiserver_clusterip_allocator_allocation_errors_total` | counter | ALPHA | cidr, scope | Number of errors trying to allocate Cluster IPs |
| `kube_apiserver_clusterip_allocator_allocation_total` | counter | ALPHA | cidr, scope | Number of Cluster IPs allocations |
| `kube_apiserver_clusterip_allocator_available_ips` | gauge | ALPHA | cidr | Gauge measuring the number of available IPs for Services |
| `kube_apiserver_nodeport_allocator_allocated_ports` | gauge | ALPHA | - | Gauge measuring the number of allocated NodePorts for Services |
| `kube_apiserver_nodeport_allocator_allocation_errors_total` | counter | ALPHA | scope | Number of errors trying to allocate NodePort |
| `kube_apiserver_nodeport_allocator_allocation_total` | counter | ALPHA | scope | Number of NodePort allocations |
| `kube_apiserver_nodeport_allocator_available_ports` | gauge | ALPHA | - | Gauge measuring the number of available NodePorts for Services |
| `kube_apiserver_pod_logs_backend_tls_failure_total` | counter | ALPHA | - | Total number of requests for pods/logs that failed due to kubelet server TLS verification |
| `kube_apiserver_pod_logs_insecure_backend_total` | counter | ALPHA | usage | Total number of requests for pods/logs sliced by usage type: enforce_tls, skip_tls_allowed, skip_tls_denied |
| `node_authorizer_graph_actions_duration_seconds` | histogram | ALPHA | operation | Histogram of duration of graph actions in node authorizer. |
| `pod_security_errors_total` | counter | ALPHA | fatal, request_operation, resource, subresource | Number of errors preventing normal evaluation. |
| `pod_security_evaluations_total` | counter | ALPHA | decision, mode, policy_level, policy_version, request_operation, resource, subresource | Number of policy evaluations that occurred, not counting ignored or exempt requests. |
| `pod_security_exemptions_total` | counter | ALPHA | request_operation, resource, subresource | Number of exempt requests, not counting ignored or out of scope requests. |
| `serviceaccount_invalid_legacy_auto_token_uses_total` | counter | ALPHA | - | Cumulative invalid auto-generated legacy tokens used |
| `serviceaccount_legacy_auto_token_uses_total` | counter | ALPHA | - | Cumulative auto-generated legacy tokens used |
| `serviceaccount_legacy_manual_token_uses_total` | counter | ALPHA | - | Cumulative manually created legacy tokens used |
| `watch_cache_capacity` | gauge | ALPHA | group, resource | Total capacity of watch cache broken by resource type. |
| `watch_cache_capacity_decrease_total` | counter | ALPHA | group, resource | Total number of watch cache capacity decrease events broken by resource type. |
| `watch_cache_capacity_increase_total` | counter | ALPHA | group, resource | Total number of watch cache capacity increase events broken by resource type. |

Omitted as low operational value (info/config/internal bookkeeping): `aggregator_openapi_v2_regeneration_count`, `aggregator_openapi_v2_regeneration_duration`, `apiextensions_openapi_v2_regeneration_count`, `apiextensions_openapi_v3_regeneration_count`, `apiserver_authentication_config_controller_last_config_info`, `apiserver_authentication_jwt_authenticator_jwks_fetch_last_key_set_info`, `apiserver_authorization_config_controller_last_config_info`, `apiserver_encryption_config_controller_last_config_info`, `apiserver_flowcontrol_current_r`, `apiserver_flowcontrol_dispatch_r`, `apiserver_flowcontrol_epoch_advance_total`, `apiserver_flowcontrol_latest_s`, `apiserver_flowcontrol_next_discounted_s_bounds`, `apiserver_flowcontrol_next_s_bounds`, `apiserver_flowcontrol_seat_fair_frac`, `apiserver_flowcontrol_watch_count_samples`, `apiserver_manifest_admission_config_controller_last_config_info`, `apiserver_request_timestamp_comparison_time`

## kube-scheduler

| Metric | Type | Stability | Key labels | Meaning |
|---|---|---|---|---|
| `kube_pod_resource_limit` | custom collector (gauge/counter) | STABLE | namespace, pod, node, scheduler, priority, resource, unit | Resources limit for workloads on the cluster, broken down by pod. |
| `kube_pod_resource_request` | custom collector (gauge/counter) | STABLE | namespace, pod, node, scheduler, priority, resource, unit | Resources requested by workloads on the cluster, broken down by pod. |
| `scheduler_framework_extension_point_duration_seconds` | histogram | STABLE | extension_point, profile, status | Latency for running all plugins of a specific extension point. |
| **R** `scheduler_pending_pods` | gauge | STABLE | queue | Number of pending pods, by the queue type. |
| `scheduler_pod_scheduling_attempts` | histogram | STABLE | - | Number of attempts to successfully schedule a pod. |
| **R** `scheduler_preemption_attempts_total` | counter | STABLE | - | Total preemption attempts in the cluster till now |
| `scheduler_preemption_victims` | histogram | STABLE | - | Number of selected preemption victims for preemption initiated by a single pod |
| `scheduler_queue_incoming_pods_total` | counter | STABLE | event, queue | Number of pods added to scheduling queues by event and queue type. |
| **R** `scheduler_schedule_attempts_total` | counter | STABLE | profile, result | Number of attempts to schedule pods, by the result. |
| **R** `scheduler_scheduling_attempt_duration_seconds` | histogram | STABLE | profile, result | Scheduling attempt latency in seconds (scheduling algorithm + binding) |
| `scheduler_goroutines` | gauge | BETA | operation | Number of running goroutines split by the work they do such as binding. |
| `scheduler_permit_wait_duration_seconds` | histogram | BETA | result | Duration of waiting on permit. |
| `scheduler_plugin_evaluation_total` | counter | BETA | extension_point, plugin, profile | Number of attempts to schedule pods by each plugin and the extension point (available only in PreFilter, Filter, PreScore, and Score). |
| `scheduler_plugin_execution_duration_seconds` | histogram | BETA | extension_point, plugin, status | Duration for running a plugin at a specific extension point. |
| **R** `scheduler_pod_scheduling_sli_duration_seconds` | histogram | BETA | attempts | E2e latency for a pod being scheduled, from the time the pod enters the scheduling queue and might involve multiple scheduling attempts. |
| `scheduler_scheduling_algorithm_duration_seconds` | histogram | BETA | - | Scheduling algorithm latency in seconds |
| **R** `scheduler_unschedulable_pods` | gauge | BETA | plugin, profile | The number of unschedulable pods broken down by plugin name. |
| `dynamic_resource_allocation_resourceclaim_creates_total` | counter | ALPHA | admin_access, owner_api_group, owner_api_kind, status | Number of ResourceClaims creation requests, categorized by creation status, admin access, and owning API group and kind (either Pod or scheduling.k8s.io/PodGroup) also: kube-controller-manager |
| `scheduler_async_api_call_execution_duration_seconds` | histogram | ALPHA | call_type, result | Duration in seconds for executing API call in the async dispatcher. |
| `scheduler_async_api_call_execution_total` | counter | ALPHA | call_type, result | Total number of API calls executed by the async dispatcher. |
| `scheduler_batch_attempts_total` | counter | ALPHA | profile, result | Counts of results when we attempt to use batching. |
| `scheduler_batch_cache_flushed_total` | counter | ALPHA | profile, reason | Counts of cache flushes by reason. |
| `scheduler_batch_rescore_attempts_total` | counter | ALPHA | profile | Counts of rescore attempts during opportunistic batching. |
| `scheduler_batch_rescore_duration_seconds` | histogram | ALPHA | profile | Latency for rescoring a node during opportunistic batching. |
| `scheduler_cache_size` | gauge | ALPHA | type | Number of nodes, pods, and assumed (bound) pods in the scheduler cache. |
| `scheduler_dra_bindingconditions_allocations_total` | counter | ALPHA | driver, profile, status | Number of allocations using devices with BindingConditions, counted per driver per scheduling attempt |
| `scheduler_dra_bindingconditions_wait_duration_seconds` | histogram | ALPHA | driver, profile, status | Time in seconds spent waiting for BindingConditions to be satisfied during PreBind. |
| `scheduler_event_handling_duration_seconds` | histogram | ALPHA | event | Event handling latency in seconds. |
| `scheduler_generated_placements_total` | counter | ALPHA | profile | Number of candidate placements generated when scheduling pod groups. |
| `scheduler_get_node_hint_duration_seconds` | histogram | ALPHA | hinted, profile | Latency for getting a node hint. |
| `scheduler_inflight_events` | gauge | ALPHA | event | Number of events currently tracked in the scheduling queue. |
| `scheduler_pending_async_api_calls` | gauge | ALPHA | call_type | Number of API calls currently pending in the async queue. |
| `scheduler_placement_evaluation_duration_seconds` | histogram | ALPHA | profile, result | Latency in seconds of evaluating a single candidate placement when scheduling pod groups, by result. |
| `scheduler_placement_evaluations_total` | counter | ALPHA | profile, result | Number of candidate placements evaluated when scheduling pod groups, by result. |
| `scheduler_pod_scheduled_after_flush_total` | counter | ALPHA | - | Number of pods that were successfully scheduled after being flushed from unschedulableEntities due to timeout. |
| `scheduler_podgroup_schedule_attempts_total` | counter | ALPHA | profile, result | Number of attempts to schedule pod group, by the result. |
| `scheduler_podgroup_scheduling_algorithm_duration_seconds` | histogram | ALPHA | - | Pod group scheduling algorithm latency in seconds |
| `scheduler_podgroup_scheduling_attempt_duration_seconds` | histogram | ALPHA | profile, result | Pod group scheduling attempt latency in seconds |
| `scheduler_pre_queueing_hint_evaluations_total` | counter | ALPHA | plugin, result | Number of PreQueueingHint evaluations, labeled by plugin and result (all_pods or narrowed). |
| `scheduler_preemption_evaluation_duration_seconds` | histogram | ALPHA | preemptor, result | Duration in seconds for identifying the target preemption victims. |
| `scheduler_preemption_execution_duration_seconds` | histogram | ALPHA | preemptor, result | Duration in seconds for preempting the target preemption victims. |
| `scheduler_preemption_goroutines_duration_seconds` | histogram | ALPHA | result | Duration in seconds for running goroutines for the preemption. **DEPRECATED since 1.37.0** |
| `scheduler_preemption_goroutines_execution_total` | counter | ALPHA | result | Number of preemption goroutines executed. |
| `scheduler_preemption_pdb_violations_total` | counter | ALPHA | preemptor | Total number of pod disruption budget violations caused by preemption. |
| `scheduler_preemption_workload_disruptions` | histogram | ALPHA | preemptor | Number of workload preemption units being preempted. |
| `scheduler_queue_incoming_entities_total` | counter | ALPHA | event, queue, type | Number of scheduling entities added to scheduling queues by event, queue type, and entity type. |
| `scheduler_queued_entities` | gauge | ALPHA | queue, type | Number of queued scheduling entities ('pod' or 'podgroup'; 'pod' stands for individual pods that are not members of any podgroup) by the queue type. |
| `scheduler_queueing_hint_execution_duration_seconds` | histogram | ALPHA | event, hint, plugin | Duration for running a queueing hint function of a plugin. |
| `scheduler_store_schedule_results_duration_seconds` | histogram | ALPHA | profile | Latency for storing scheduling results. |
| `scheduler_volume_binder_cache_requests_total` | counter | ALPHA | operation | Total number for request volume binding cache |
| `scheduler_volume_scheduling_stage_error_total` | counter | ALPHA | operation | Volume scheduling stage error count |
| `scheduler_workload_preemption_attempts_total` | counter | ALPHA | result | Total preemption attempts initiated by workload (including pod groups) in the cluster till now. |
| `scheduler_workload_preemption_victims` | histogram | ALPHA | - | Number of pod preemption victims caused by workload preemption. |

## kube-controller-manager

| Metric | Type | Stability | Key labels | Meaning |
|---|---|---|---|---|
| `cronjob_controller_job_creation_skew_duration_seconds` | histogram | STABLE | - | Time between when a cronjob is scheduled to be run, and when the corresponding job is created |
| `job_controller_job_pods_finished_total` | counter | STABLE | completion_mode, result | The number of finished Pods that are fully tracked |
| `job_controller_job_sync_duration_seconds` | histogram | STABLE | action, completion_mode, result | The time it took to sync a job |
| `job_controller_job_syncs_total` | counter | STABLE | action, completion_mode, result | The number of job syncs |
| **R** `job_controller_jobs_finished_total` | counter | STABLE | completion_mode, reason, result | The number of finished jobs |
| **R** `node_collector_evictions_total` | counter | STABLE | zone | Number of Node evictions that happened since current instance of NodeController started. |
| `endpoint_slice_controller_desired_endpoint_slices` | gauge | BETA | - | Number of EndpointSlices that would exist with perfect endpoint allocation |
| `endpoint_slice_controller_endpoints_added_per_sync` | histogram | BETA | - | Number of endpoints added on each Service sync |
| `endpoint_slice_controller_endpoints_desired` | gauge | BETA | - | Number of endpoints desired |
| `endpoint_slice_controller_endpoints_removed_per_sync` | histogram | BETA | - | Number of endpoints removed on each Service sync |
| `endpoint_slice_controller_num_endpoint_slices` | gauge | BETA | - | Number of EndpointSlices |
| `endpoint_slice_controller_services_count_by_traffic_distribution` | gauge | BETA | traffic_distribution | Number of Services using some specific trafficDistribution |
| `horizontal_pod_autoscaler_controller_metric_computation_duration_seconds` | histogram | BETA | action, error, metric_type | The time(seconds) that the HPA controller takes to calculate one metric. |
| `horizontal_pod_autoscaler_controller_metric_computation_total` | counter | BETA | action, error, metric_type | Number of metric computations. |
| `horizontal_pod_autoscaler_controller_reconciliation_duration_seconds` | histogram | BETA | action, error | The time(seconds) that the HPA controller takes to reconcile once. |
| `horizontal_pod_autoscaler_controller_reconciliations_total` | counter | BETA | action, error | Number of reconciliations of HPA controller. |
| `job_controller_pod_failures_handled_by_failure_policy_total` | counter | BETA | action | The number of failed Pods handled by failure policy with, respect to the failure policy action applied based on the matched, rule. |
| `job_controller_terminated_pods_tracking_finalizer_total` | counter | BETA | event | The number of terminated pods (phase=Failed/Succeeded), that have the finalizer batch.kubernetes.io/job-tracking, The event label can be "add" or "delete". |
| `attach_detach_controller_attachdetach_controller_forced_detaches` | counter | ALPHA | reason | Number of times the A/D Controller performed a forced detach |
| `attachdetach_controller_total_volumes` | custom collector (gauge/counter) | ALPHA | plugin_name, state | Number of volumes in A/D Controller |
| `clustertrustbundle_publisher_sync_duration_seconds` | histogram | ALPHA | code | The time it took to sync a cluster trust bundle. |
| `clustertrustbundle_publisher_sync_total` | counter | ALPHA | code | Number of syncs that occurred in cluster trust bundle publisher. |
| `daemonset_controller_stale_sync_skips_total` | counter | ALPHA | group, resource | Total number of DaemonSet syncs skipped due to a stale watch cache. |
| `device_taint_eviction_controller_pod_deletion_duration_seconds` | histogram | ALPHA | - | Latency, in seconds, between the time when a device taint effect has been activated and a Pod's deletion via DeviceTaintEvictionController. |
| `device_taint_eviction_controller_pod_deletions_total` | counter | ALPHA | - | Total number of Pods deleted by DeviceTaintEvictionController since its start. |
| `dynamic_resource_allocation_resource_claims` | custom collector (gauge/counter) | ALPHA | allocated, admin_access, source | Number of ResourceClaims, categorized by allocation status, admin access, and source. |
| `dynamic_resource_allocation_resourceclaim_creates_total` | counter | ALPHA | admin_access, owner_api_group, owner_api_kind, status | Number of ResourceClaims creation requests, categorized by creation status, admin access, and owning API group and kind (either Pod or scheduling.k8s.io/PodGroup) also: kube-scheduler |
| `endpoint_slice_controller_changes` | counter | ALPHA | operation | Number of EndpointSlice changes |
| `endpoint_slice_controller_endpointslices_changed_per_sync` | histogram | ALPHA | topology, traffic_distribution | Number of EndpointSlices changed on each Service sync |
| `endpoint_slice_controller_syncs` | counter | ALPHA | result | Number of EndpointSlice syncs |
| `endpoint_slice_mirroring_controller_addresses_skipped_per_sync` | histogram | ALPHA | - | Number of addresses skipped on each Endpoints sync due to being invalid or exceeding MaxEndpointsPerSubset |
| `endpoint_slice_mirroring_controller_changes` | counter | ALPHA | operation | Number of EndpointSlice changes |
| `endpoint_slice_mirroring_controller_desired_endpoint_slices` | gauge | ALPHA | - | Number of EndpointSlices that would exist with perfect endpoint allocation |
| `endpoint_slice_mirroring_controller_endpoints_added_per_sync` | histogram | ALPHA | - | Number of endpoints added on each Endpoints sync |
| `endpoint_slice_mirroring_controller_endpoints_desired` | gauge | ALPHA | - | Number of endpoints desired |
| `endpoint_slice_mirroring_controller_endpoints_removed_per_sync` | histogram | ALPHA | - | Number of endpoints removed on each Endpoints sync |
| `endpoint_slice_mirroring_controller_endpoints_sync_duration` | histogram | ALPHA | - | Duration of syncEndpoints() in seconds |
| `endpoint_slice_mirroring_controller_endpoints_updated_per_sync` | histogram | ALPHA | - | Number of endpoints updated on each Endpoints sync |
| `endpoint_slice_mirroring_controller_num_endpoint_slices` | gauge | ALPHA | - | Number of EndpointSlices |
| `ephemeral_volume_controller_create_failures_total` | counter | ALPHA | - | Number of PersistentVolumeClaim creation requests |
| `ephemeral_volume_controller_create_total` | counter | ALPHA | - | Number of PersistentVolumeClaim creation requests |
| `garbagecollector_controller_resources_sync_error_total` | counter | ALPHA | - | Number of garbage collector resources sync errors |
| `horizontal_pod_autoscaler_controller_desired_replicas` | gauge | ALPHA | hpa_name, namespace | Current desired replica count for HPA objects. |
| `horizontal_pod_autoscaler_controller_num_horizontal_pod_autoscalers` | gauge | ALPHA | - | Current number of controlled HPA objects. |
| `horizontal_pod_autoscaler_controller_stale_sync_skips_total` | counter | ALPHA | group, resource | Total number of HPA syncs skipped due to a stale watch cache. |
| `job_controller_job_finished_indexes_total` | counter | ALPHA | backoffLimit, status | The number of finished indexes. |
| `job_controller_job_pods_creation_total` | counter | ALPHA | reason, status | The number of Pods created by the Job controller labelled with a reason for the Pod creation., This metric also distinguishes between Pods created using different PodR... |
| `job_controller_jobs_by_external_controller_total` | counter | ALPHA | controller_name | The number of Jobs managed by an external controller |
| `job_controller_stale_sync_skips_total` | counter | ALPHA | group, resource | Total number of Job syncs skipped due to a stale watch cache. |
| **R** `node_collector_unhealthy_nodes_in_zone` | gauge | ALPHA | zone | Gauge measuring number of not Ready Nodes per zones. |
| `node_collector_update_all_nodes_health_duration_seconds` | histogram | ALPHA | - | Duration in seconds for NodeController to update the health of all nodes. |
| `node_collector_update_node_health_duration_seconds` | histogram | ALPHA | - | Duration in seconds for NodeController to update the health of a single node. |
| **R** `node_collector_zone_health` | gauge | ALPHA | zone | Gauge measuring percentage of healthy nodes per zone. |
| `node_collector_zone_size` | gauge | ALPHA | zone | Gauge measuring number of registered Nodes per zones. |
| `node_ipam_controller_cidrset_allocation_tries_per_request` | histogram | ALPHA | clusterCIDR | Number of endpoints added on each Service sync |
| `node_ipam_controller_cidrset_cidrs_allocations_total` | counter | ALPHA | clusterCIDR | Counter measuring total number of CIDR allocations. |
| `node_ipam_controller_cidrset_cidrs_releases_total` | counter | ALPHA | clusterCIDR | Counter measuring total number of CIDR releases. |
| `node_ipam_controller_cidrset_usage_cidrs` | gauge | ALPHA | clusterCIDR | Gauge measuring percentage of allocated CIDRs. |
| `node_ipam_controller_cirdset_max_cidrs` | gauge | ALPHA | clusterCIDR | Maximum number of CIDRs that can be allocated. |
| `pod_gc_collector_force_delete_pod_errors_total` | counter | ALPHA | namespace, reason | Number of errors encountered when forcefully deleting the pods since the Pod GC Controller started. |
| `pod_gc_collector_force_delete_pods_total` | counter | ALPHA | namespace, reason | Number of pods that are being forcefully deleted since the Pod GC Controller started. |
| `pv_collector_bound_pv_count` | custom collector (gauge/counter) | ALPHA | storage_class | Gauge measuring number of persistent volume currently bound |
| `pv_collector_bound_pvc_count` | custom collector (gauge/counter) | ALPHA | namespace, storage_class, volume_attributes_class | Gauge measuring number of persistent volume claim currently bound |
| `pv_collector_total_pv_count` | custom collector (gauge/counter) | ALPHA | plugin_name, volume_mode | Gauge measuring total number of persistent volumes |
| `pv_collector_unbound_pv_count` | custom collector (gauge/counter) | ALPHA | storage_class | Gauge measuring number of persistent volume currently unbound |
| `pv_collector_unbound_pvc_count` | custom collector (gauge/counter) | ALPHA | namespace, storage_class, volume_attributes_class | Gauge measuring number of persistent volume claim currently unbound |
| `replicaset_controller_sorting_deletion_age_ratio` | histogram | ALPHA | - | The ratio of chosen deleted pod's ages to the current youngest pod's age (at the time). |
| `replicaset_controller_stale_sync_skips_total` | counter | ALPHA | group, resource | Total number of ReplicaSet syncs skipped due to a stale watch cache. |
| `resourcepoolstatusrequest_controller_request_processing_duration_seconds` | histogram | ALPHA | driver_name | Time taken to process a ResourcePoolStatusRequest |
| `resourcepoolstatusrequest_controller_request_processing_errors_total` | counter | ALPHA | driver_name | Total number of errors encountered while processing ResourcePoolStatusRequests |
| `resourcepoolstatusrequest_controller_requests_processed_total` | counter | ALPHA | driver_name | Total number of ResourcePoolStatusRequests processed |
| `retroactive_storageclass_errors_total` | counter | ALPHA | - | Total number of failed retroactive StorageClass assignments to persistent volume claim |
| `retroactive_storageclass_total` | counter | ALPHA | - | Total number of retroactive StorageClass assignments to persistent volume claim |
| `root_ca_cert_publisher_sync_duration_seconds` | histogram | ALPHA | code | Number of namespace syncs happened in root ca cert publisher. |
| `root_ca_cert_publisher_sync_total` | counter | ALPHA | code | Number of namespace syncs happened in root ca cert publisher. |
| `selinux_warning_controller_selinux_volume_conflict` | custom collector (gauge/counter) | ALPHA | property, pod1_namespace, pod1_name, pod1_value, pod2_namespace, pod2_name, pod2_value | Conflict between two Pods using the same volume |
| `statefulset_controller_stale_sync_skips_total` | counter | ALPHA | group, resource | Total number of StatefulSet syncs skipped due to a stale watch cache. |
| `statefulset_controller_statefulset_max_unavailable` | gauge | ALPHA | pod_management_policy, statefulset_name, statefulset_namespace | Maximum number of unavailable pods allowed during StatefulSet rolling updates |
| `statefulset_controller_statefulset_unavailable_replicas` | gauge | ALPHA | pod_management_policy, statefulset_name, statefulset_namespace | Current number of unavailable pods in StatefulSet |
| `storage_count_attachable_volumes_in_use` | custom collector (gauge/counter) | ALPHA | node, volume_plugin | Measure number of volumes in use |
| `taint_eviction_controller_pod_deletion_duration_seconds` | histogram | ALPHA | - | Latency, in seconds, between the time when a taint effect has been activated for the Pod and its deletion via TaintEvictionController. |
| `taint_eviction_controller_pod_deletions_total` | counter | ALPHA | - | Total number of Pods deleted by TaintEvictionController since its start. |
| `ttl_after_finished_controller_job_deletion_duration_seconds` | histogram | ALPHA | - | The time it took to delete the job since it became eligible for deletion |
| **R** `volume_operation_errors_total` | counter | ALPHA | operation_name, plugin_name | Total volume operation errors |
| `volume_operation_total_errors` | counter | ALPHA | operation_name, plugin_name | Total volume operation errors **DEPRECATED since 1.36.0** |

## kubelet

| Metric | Type | Stability | Key labels | Endpoint | Meaning |
|---|---|---|---|---|---|
| `container_cpu_usage_seconds_total` | custom collector (gauge/counter) | STABLE | container, pod, namespace | /metrics/resource | Cumulative cpu time consumed by the container in core-seconds |
| `container_memory_working_set_bytes` | custom collector (gauge/counter) | STABLE | container, pod, namespace | /metrics/resource | Current working set of the container in bytes |
| `container_start_time_seconds` | custom collector (gauge/counter) | STABLE | container, pod, namespace | /metrics/resource | Start time of the container since unix epoch in seconds |
| **R** `node_cpu_usage_seconds_total` | custom collector (gauge/counter) | STABLE | - | /metrics/resource | Cumulative cpu time consumed by the node in core-seconds |
| **R** `node_memory_working_set_bytes` | custom collector (gauge/counter) | STABLE | - | /metrics/resource | Current working set of the node in bytes |
| `pod_cpu_usage_seconds_total` | custom collector (gauge/counter) | STABLE | pod, namespace | /metrics/resource | Cumulative cpu time consumed by the pod in core-seconds |
| `pod_memory_working_set_bytes` | custom collector (gauge/counter) | STABLE | pod, namespace | /metrics/resource | Current working set of the pod in bytes |
| `resource_scrape_error` | custom collector (gauge/counter) | STABLE | - | /metrics/resource | 1 if there was an error while getting container metrics, 0 otherwise |
| `kubelet_image_volume_mounted_errors_total` | counter | BETA | - | /metrics | Number of failed image volume mounts. |
| `kubelet_image_volume_mounted_succeed_total` | counter | BETA | - | /metrics | Number of successful image volume mounts. |
| `kubelet_image_volume_requested_total` | counter | BETA | - | /metrics | Number of requested image volumes. |
| **R** `prober_probe_total` | counter | BETA | container, namespace, pod, pod_uid, probe_type, result | /metrics/probes | Cumulative number of a liveness, readiness or startup probe for a container by result. |
| **R** `storage_operation_duration_seconds` | histogram | BETA | migrated, operation_name, status, volume_plugin | /metrics | Storage operation duration |
| `volume_operation_total_seconds` | histogram | BETA | operation_name, plugin_name | /metrics | Storage operation end to end duration in seconds |
| `container_swap_limit_bytes` | custom collector (gauge/counter) | ALPHA | container, pod, namespace | /metrics/resource | Current amount of the container swap limit in bytes. |
| `container_swap_usage_bytes` | custom collector (gauge/counter) | ALPHA | container, pod, namespace | /metrics/resource | Current amount of the container swap usage in bytes. |
| `csi_node_storage_health_status` | gauge | ALPHA | driver_name, reason, status | /metrics | CSI node storage backend health conditions currently reported (1 = present). |
| `csi_operations_seconds` | histogram | ALPHA | driver_name, grpc_status_code, method_name, migrated | /metrics | Container Storage Interface operation duration with gRPC error code status total |
| `dra_grpc_operations_duration_seconds` | histogram | ALPHA | driver_name, grpc_status_code, method_name | /metrics | Duration in seconds of the DRA gRPC operations |
| `dra_node_prepare_skips_total` | counter | ALPHA | driver_name | /metrics | Cumulative number of times NodePrepareResources gRPC calls were skipped due to SkipNodeOperations. |
| `dra_node_unprepare_skips_total` | counter | ALPHA | driver_name | /metrics | Cumulative number of times NodeUnprepareResources gRPC calls were skipped due to SkipNodeOperations. |
| `dra_operations_duration_seconds` | histogram | ALPHA | is_error, operation_name | /metrics | Latency histogram in seconds for the duration of handling all ResourceClaims referenced by a pod when the pod starts or stops. |
| `dra_resource_claims_in_use` | custom collector (gauge/counter) | ALPHA | driver_name | /metrics | The number of ResourceClaims that are currently in use on the node, by driver name (driver_name label value) and across all drivers (special value <any> for driver_name). |
| `force_cleaned_failed_volume_operation_errors_total` | counter | ALPHA | - | /metrics | The number of volumes that failed force cleanup after their reconstruction failed during kubelet startup. |
| `force_cleaned_failed_volume_operations_total` | counter | ALPHA | - | /metrics | The number of volumes that were force cleaned after their reconstruction failed during kubelet startup. |
| `kubelet_active_pods` | gauge | ALPHA | static | /metrics | The number of pods the kubelet considers active and which are being considered when admitting new pods. |
| **R** `kubelet_admission_rejections_total` | counter | ALPHA | reason | /metrics | Cumulative number pod admission rejections by the Kubelet. |
| `kubelet_certificate_manager_client_expiration_renew_errors` | counter | ALPHA | - | /metrics | Counter of certificate renewal errors. |
| **R** `kubelet_certificate_manager_client_ttl_seconds` | gauge | ALPHA | - | /metrics | Gauge of the TTL (time-to-live) of the Kubelet's client certificate. |
| `kubelet_certificate_manager_server_rotation_seconds` | histogram | ALPHA | - | /metrics | Histogram of the number of seconds the previous certificate lived before being rotated. |
| `kubelet_certificate_manager_server_ttl_seconds` | gauge | ALPHA | - | /metrics | Gauge of the shortest TTL (time-to-live) of the Kubelet's serving certificate. |
| `kubelet_cgroup_manager_duration_seconds` | histogram | ALPHA | operation_type | /metrics | Duration in seconds for cgroup manager operations. |
| `kubelet_container_aligned_compute_resources_count` | counter | ALPHA | boundary, scope | /metrics | Cumulative number of aligned compute resources allocated to containers by alignment type. |
| `kubelet_container_aligned_compute_resources_failure_count` | counter | ALPHA | boundary, scope | /metrics | Cumulative number of failures to allocate aligned compute resources to containers by alignment type. |
| `kubelet_container_log_filesystem_used_bytes` | custom collector (gauge/counter) | ALPHA | uid, namespace, pod, container | /metrics | Bytes used by the container's logs on the filesystem. |
| `kubelet_container_requested_resizes_total` | counter | ALPHA | operation, requirement, resource | /metrics | Number of requested resizes, counted at the container level. |
| `kubelet_containers_per_pod_count` | histogram | ALPHA | - | /metrics | The number of containers per pod. |
| `kubelet_cpu_manager_allocation_per_numa` | gauge | ALPHA | numa_node | /metrics | Number of CPUs allocated per NUMA node |
| `kubelet_cpu_manager_exclusive_cpu_allocation_count` | gauge | ALPHA | - | /metrics | The total number of CPUs exclusively allocated to containers running on this node |
| `kubelet_cpu_manager_pinning_errors_total` | counter | ALPHA | - | /metrics | The number of cpu core allocations which required pinning failed. |
| `kubelet_cpu_manager_pinning_requests_total` | counter | ALPHA | - | /metrics | The number of cpu core allocations which required pinning. |
| `kubelet_cpu_manager_shared_pool_size_millicores` | gauge | ALPHA | - | /metrics | The size of the shared CPU pool for non-guaranteed QoS pods, in millicores. |
| `kubelet_credential_provider_plugin_duration` | histogram | ALPHA | plugin_name | /metrics | Duration of execution in seconds for credential provider plugin |
| `kubelet_credential_provider_plugin_errors_total` | counter | ALPHA | plugin_name | /metrics | Number of errors from credential provider plugin |
| `kubelet_desired_pods` | gauge | ALPHA | static | /metrics | The number of pods the kubelet is being instructed to run. |
| `kubelet_device_plugin_alloc_duration_seconds` | histogram | ALPHA | resource_name | /metrics | Duration in seconds to serve a device plugin Allocation request. |
| `kubelet_device_plugin_registration_total` | counter | ALPHA | resource_name | /metrics | Cumulative number of device plugin registrations. |
| `kubelet_evented_pleg_connection_error_count` | counter | ALPHA | - | /metrics | The number of errors encountered during the establishment of streaming connection with the CRI runtime. |
| `kubelet_evented_pleg_connection_latency_seconds` | histogram | ALPHA | - | /metrics | The latency of streaming connection with the CRI runtime, measured in seconds. |
| `kubelet_evented_pleg_connection_success_count` | counter | ALPHA | - | /metrics | The number of times a streaming client was obtained to receive CRI Events. |
| `kubelet_eviction_stats_age_seconds` | histogram | ALPHA | eviction_signal | /metrics | Time between when stats are collected, and when pod is evicted based on those stats by eviction signal |
| **R** `kubelet_evictions` | counter | ALPHA | eviction_signal | /metrics | Cumulative number of pod evictions by eviction signal |
| `kubelet_graceful_shutdown_end_time_seconds` | gauge | ALPHA | - | /metrics | Last graceful shutdown end time since unix epoch in seconds |
| `kubelet_graceful_shutdown_start_time_seconds` | gauge | ALPHA | - | /metrics | Last graceful shutdown start time since unix epoch in seconds |
| `kubelet_http_inflight_requests` | gauge | ALPHA | long_running, method, path, server_type | /metrics | Number of the inflight http requests |
| `kubelet_http_requests_duration_seconds` | histogram | ALPHA | long_running, method, path, server_type | /metrics | Duration in seconds to serve http requests |
| `kubelet_http_requests_total` | counter | ALPHA | long_running, method, path, server_type | /metrics | Number of the http requests received since the server started |
| `kubelet_image_garbage_collected_total` | counter | ALPHA | reason | /metrics | Total number of images garbage collected by the kubelet, whether through disk usage or image age. |
| `kubelet_image_manager_ensure_image_requests_total` | counter | ALPHA | present_locally, pull_policy, pull_required | /metrics | Number of ensure-image requests processed by the kubelet. |
| `kubelet_image_pull_duration_seconds` | histogram | ALPHA | image_size_in_bytes | /metrics | Duration in seconds to pull an image. |
| `kubelet_imagemanager_image_mustpull_checks_total` | counter | ALPHA | result | /metrics | Counter for how many times kubelet checked whether credentials need to be re-verified to access an image |
| `kubelet_imagemanager_inmemory_pulledrecords_usage_percent` | gauge | ALPHA | - | /metrics | The ImagePulledRecords in-memory cache usage in percent. |
| `kubelet_imagemanager_inmemory_pullintents_usage_percent` | gauge | ALPHA | - | /metrics | The ImagePullIntents in-memory cache usage in percent. |
| `kubelet_imagemanager_ondisk_pulledrecords` | gauge | ALPHA | - | /metrics | Number of ImagePulledRecords stored on disk. |
| `kubelet_imagemanager_ondisk_pullintents` | gauge | ALPHA | - | /metrics | Number of ImagePullIntents stored on disk. |
| `kubelet_lifecycle_handler_http_fallbacks_total` | counter | ALPHA | - | /metrics | The number of times lifecycle handlers successfully fell back to http from https. |
| `kubelet_managed_ephemeral_containers` | gauge | ALPHA | - | /metrics | Current number of ephemeral containers in pods managed by this kubelet. |
| `kubelet_memory_manager_pinning_errors_total` | counter | ALPHA | - | /metrics | The number of memory pages allocations which required pinning that failed. |
| `kubelet_memory_manager_pinning_requests_total` | counter | ALPHA | - | /metrics | The number of memory pages allocations which required pinning. |
| `kubelet_memory_qos_node_memory_low_bytes` | gauge | ALPHA | - | /metrics | Total cgroup v2 memory.low in bytes for Burstable pods. |
| `kubelet_memory_qos_node_memory_min_bytes` | gauge | ALPHA | - | /metrics | Total cgroup v2 memory.min in bytes for Guaranteed pods. |
| `kubelet_mirror_pods` | gauge | ALPHA | - | /metrics | The number of mirror pods the kubelet will try to create (one per admitted static pod) |
| `kubelet_node_startup_duration_seconds` | gauge | ALPHA | - | /metrics | Duration in seconds of node startup in total. |
| `kubelet_node_startup_post_registration_duration_seconds` | gauge | ALPHA | - | /metrics | Duration in seconds of node startup after registration. |
| `kubelet_node_startup_pre_kubelet_duration_seconds` | gauge | ALPHA | - | /metrics | Duration in seconds of node startup before kubelet starts. |
| `kubelet_node_startup_pre_registration_duration_seconds` | gauge | ALPHA | - | /metrics | Duration in seconds of node startup before registration. |
| `kubelet_node_startup_registration_duration_seconds` | gauge | ALPHA | - | /metrics | Duration in seconds of node startup during registration. |
| `kubelet_orphan_pod_cleaned_volumes` | gauge | ALPHA | - | /metrics | The total number of orphaned Pods whose volumes were cleaned in the last periodic sweep. |
| `kubelet_orphan_pod_cleaned_volumes_errors` | gauge | ALPHA | - | /metrics | The number of orphaned Pods whose volumes failed to be cleaned in the last periodic sweep. |
| `kubelet_orphaned_runtime_pods_total` | counter | ALPHA | - | /metrics | Number of pods that have been detected in the container runtime without being already known to the pod worker. |
| `kubelet_pleg_discard_events` | counter | ALPHA | - | /metrics | The number of discard events in PLEG. |
| **R** `kubelet_pleg_last_seen_seconds` | gauge | ALPHA | - | /metrics | Timestamp in seconds when PLEG was last seen active. |
| `kubelet_pleg_pod_relist_duration_seconds` | histogram | ALPHA | - | /metrics | Duration in seconds for relisting a single pod in PLEG. |
| **R** `kubelet_pleg_relist_duration_seconds` | histogram | ALPHA | - | /metrics | Duration in seconds for relisting pods in PLEG. |
| `kubelet_pleg_relist_interval_seconds` | histogram | ALPHA | - | /metrics | Interval in seconds between relisting in PLEG. |
| `kubelet_pod_deferred_accepted_resizes_total` | counter | ALPHA | retry_trigger | /metrics | Cumulative number of resizes that were accepted after being deferred. |
| `kubelet_pod_deferred_resize_duration_seconds` | histogram | ALPHA | priority_bucket, resolution | /metrics | Duration in seconds that a pod resize remains deferred before completion. |
| `kubelet_pod_in_progress_resizes` | gauge | ALPHA | - | /metrics | Number of in-progress resizes for pods. |
| `kubelet_pod_infeasible_resizes_total` | counter | ALPHA | reason_detail | /metrics | Number of infeasible resizes for pods. |
| `kubelet_pod_level_resources_admission_total` | counter | ALPHA | config_mode, qos_class | /metrics | Total number of pods admitted during Kubelet admission, categorized by resource configuration mode (pod_and_container_level, pod_level, container_level, or empty for u... |
| `kubelet_pod_pending_resizes` | gauge | ALPHA | priority_bucket, reason | /metrics | Number of pending resizes for pods. |
| `kubelet_pod_requests_get_total` | counter | ALPHA | server_api_version, status_code | /metrics | Number of requests to the PodsAPI Get endpoint. |
| `kubelet_pod_requests_list_total` | counter | ALPHA | server_api_version, status_code | /metrics | Number of requests to the PodsAPI List endpoint. |
| `kubelet_pod_requests_total` | counter | ALPHA | server_api_version, status_code | /metrics | Cumulative number of requests to the PodsAPI endpoint. |
| `kubelet_pod_requests_watch_total` | counter | ALPHA | server_api_version, status_code | /metrics | Number of requests to the PodsAPI Watch endpoint. |
| `kubelet_pod_resize_duration_milliseconds` | histogram | ALPHA | success | /metrics | Duration in milliseconds to actuate a pod resize |
| `kubelet_pod_resources_endpoint_errors_get` | counter | ALPHA | server_api_version | /metrics | Number of requests to the PodResource Get endpoint which returned error. |
| `kubelet_pod_resources_endpoint_errors_get_allocatable` | counter | ALPHA | server_api_version | /metrics | Number of requests to the PodResource GetAllocatableResources endpoint which returned error. |
| `kubelet_pod_resources_endpoint_errors_list` | counter | ALPHA | server_api_version | /metrics | Number of requests to the PodResource List endpoint which returned error. |
| `kubelet_pod_resources_endpoint_requests_get` | counter | ALPHA | server_api_version | /metrics | Number of requests to the PodResource Get endpoint. |
| `kubelet_pod_resources_endpoint_requests_get_allocatable` | counter | ALPHA | server_api_version | /metrics | Number of requests to the PodResource GetAllocatableResources endpoint. |
| `kubelet_pod_resources_endpoint_requests_list` | counter | ALPHA | server_api_version | /metrics | Number of requests to the PodResource List endpoint. |
| `kubelet_pod_resources_endpoint_requests_total` | counter | ALPHA | server_api_version | /metrics | Cumulative number of requests to the PodResource endpoint. |
| `kubelet_pod_start_duration_seconds` | histogram | ALPHA | - | /metrics | Duration in seconds from kubelet seeing a pod for the first time to the pod starting to run |
| **R** `kubelet_pod_start_sli_duration_seconds` | histogram | ALPHA | - | /metrics | Duration in seconds to start a pod, excluding time to pull images and run init containers, measured from pod creation timestamp to when all its containers are reported... |
| `kubelet_pod_start_total_duration_seconds` | histogram | ALPHA | - | /metrics | Duration in seconds to start a pod since creation, including time to pull images and run init containers, measured from pod creation timestamp to when all its containe... |
| `kubelet_pod_status_sync_duration_seconds` | histogram | ALPHA | - | /metrics | Duration in seconds to sync a pod status update. |
| `kubelet_pod_watch_events_dropped_total` | counter | ALPHA | - | /metrics | Cumulative number of pod watch events dropped. |
| `kubelet_pod_worker_duration_seconds` | histogram | ALPHA | operation_type | /metrics | Duration in seconds to sync a single pod. |
| `kubelet_pod_worker_start_duration_seconds` | histogram | ALPHA | - | /metrics | Duration in seconds from kubelet seeing a pod to starting a worker. |
| `kubelet_podcertificate_states` | custom collector (gauge/counter) | ALPHA | signer_name, state | /metrics | Gauge vector reporting the number of pod certificate projected volume sources, faceted by signer_name and state. |
| `kubelet_preemptions` | counter | ALPHA | preemption_signal | /metrics | Cumulative number of pod preemptions by preemption resource |
| `kubelet_resource_manager_allocation_errors_total` | counter | ALPHA | resource_name, source | /metrics | Number of errors encountered during exclusive resource allocation. |
| `kubelet_resource_manager_allocations_total` | counter | ALPHA | resource_name, source | /metrics | Number of exclusive resource allocations performed by a resource manager. |
| `kubelet_resource_manager_container_assignments_total` | counter | ALPHA | assignment_type, resource_name | /metrics | Number of containers with a specific type of resource assignment. |
| `kubelet_restarted_pods_total` | counter | ALPHA | static | /metrics | Number of pods that have been restarted because they were deleted and recreated with the same UID while the kubelet was watching them (common for static pods, extremel... |
| `kubelet_run_podsandbox_duration_seconds` | histogram | ALPHA | runtime_handler | /metrics | Duration in seconds of the run_podsandbox operations. |
| **R** `kubelet_run_podsandbox_errors_total` | counter | ALPHA | runtime_handler | /metrics | Cumulative number of the run_podsandbox operation errors by RuntimeClass.Handler. |
| **R** `kubelet_running_containers` | gauge | ALPHA | container_state | /metrics | Number of containers currently running |
| **R** `kubelet_running_pods` | gauge | ALPHA | - | /metrics | Number of pods that have a running pod sandbox |
| **R** `kubelet_runtime_operations_duration_seconds` | histogram | ALPHA | operation_type | /metrics | Duration in seconds of runtime operations. |
| **R** `kubelet_runtime_operations_errors_total` | counter | ALPHA | operation_type | /metrics | Cumulative number of runtime operation errors by operation type. |
| `kubelet_runtime_operations_total` | counter | ALPHA | operation_type | /metrics | Cumulative number of runtime operations by operation type. |
| `kubelet_server_expiration_renew_errors` | counter | ALPHA | - | /metrics | Counter of certificate renewal errors. |
| `kubelet_sleep_action_terminated_early_total` | counter | ALPHA | - | /metrics | The number of times lifecycle sleep handler got terminated before it finishes |
| **R** `kubelet_started_containers_errors_total` | counter | ALPHA | code, container_type | /metrics | Cumulative number of errors when starting containers |
| `kubelet_started_containers_total` | counter | ALPHA | container_type | /metrics | Cumulative number of containers started |
| `kubelet_started_host_process_containers_errors_total` | counter | ALPHA | code, container_type | /metrics | Cumulative number of errors when starting hostprocess containers. |
| `kubelet_started_host_process_containers_total` | counter | ALPHA | container_type | /metrics | Cumulative number of hostprocess containers started. |
| **R** `kubelet_started_pods_errors_total` | counter | ALPHA | - | /metrics | Cumulative number of errors when starting pods |
| `kubelet_started_pods_total` | counter | ALPHA | - | /metrics | Cumulative number of pods started |
| `kubelet_started_user_namespaced_pods_errors_total` | counter | ALPHA | - | /metrics | Cumulative number of errors when starting pods with user namespaces. |
| `kubelet_started_user_namespaced_pods_total` | counter | ALPHA | - | /metrics | Cumulative number of pods with user namespaces started. |
| `kubelet_terminated_containers_total` | counter | ALPHA | container_type, exit_code, reason | /metrics | Cumulative number of container terminations. |
| `kubelet_topology_manager_admission_duration_ms` | histogram | ALPHA | - | /metrics | Duration in milliseconds to serve a pod admission request. |
| `kubelet_topology_manager_admission_errors_total` | counter | ALPHA | - | /metrics | The number of admission request failures where resources could not be aligned. |
| `kubelet_topology_manager_admission_requests_total` | counter | ALPHA | - | /metrics | The number of admission requests where resources have to be aligned. |
| `kubelet_volume_metric_collection_duration_seconds` | histogram | ALPHA | metric_source | /metrics | Duration in seconds to calculate volume stats |
| **R** `kubelet_volume_stats_available_bytes` | custom collector (gauge/counter) | ALPHA | namespace, persistentvolumeclaim | /metrics | Number of available bytes in the volume |
| **R** `kubelet_volume_stats_capacity_bytes` | custom collector (gauge/counter) | ALPHA | namespace, persistentvolumeclaim | /metrics | Capacity in bytes of the volume |
| `kubelet_volume_stats_inodes` | custom collector (gauge/counter) | ALPHA | namespace, persistentvolumeclaim | /metrics | Maximum number of inodes in the volume |
| **R** `kubelet_volume_stats_inodes_free` | custom collector (gauge/counter) | ALPHA | namespace, persistentvolumeclaim | /metrics | Number of free inodes in the volume |
| `kubelet_volume_stats_inodes_used` | custom collector (gauge/counter) | ALPHA | namespace, persistentvolumeclaim | /metrics | Number of used inodes in the volume |
| `kubelet_volume_stats_used_bytes` | custom collector (gauge/counter) | ALPHA | namespace, persistentvolumeclaim | /metrics | Number of used bytes in the volume |
| `kubelet_websocket_streaming_requests_total` | counter | ALPHA | subresource | /metrics | Total number of WebSocket streaming requests (exec/attach/portforward) received by the kubelet. |
| `kubelet_working_pods` | gauge | ALPHA | config, lifecycle, static | /metrics | Number of pods the kubelet is actually running, broken down by lifecycle phase, whether the pod is desired, orphaned, or runtime only (also orphaned), and whether the ... |
| `node_swap_usage_bytes` | custom collector (gauge/counter) | ALPHA | - | /metrics/resource | Current swap usage of the node in bytes. |
| `plugin_manager_total_plugins` | custom collector (gauge/counter) | ALPHA | socket_path, state | /metrics | Number of plugins in Plugin Manager |
| `pod_swap_usage_bytes` | custom collector (gauge/counter) | ALPHA | pod, namespace | /metrics/resource | Current amount of the pod swap usage in bytes. |
| `prober_probe_duration_seconds` | histogram | ALPHA | container, namespace, pod, probe_type | /metrics/probes | Duration in seconds for a probe response. |
| `reconstruct_volume_operations_errors_total` | counter | ALPHA | - | /metrics | The number of volumes that failed reconstruction from the operating system during kubelet startup. |
| `reconstruct_volume_operations_total` | counter | ALPHA | - | /metrics | The number of volumes that were attempted to be reconstructed from the operating system during kubelet startup. |
| `scrape_error` | custom collector (gauge/counter) | ALPHA | - | /metrics/resource | 1 if there was an error while getting container metrics, 0 otherwise **DEPRECATED since 1.29.0** |
| `volume_manager_selinux_container_errors_total` | gauge | ALPHA | access_mode | /metrics | Number of errors when kubelet cannot compute SELinux context for a container. |
| `volume_manager_selinux_container_warnings_total` | gauge | ALPHA | access_mode | /metrics | Number of errors when kubelet cannot compute SELinux context for a container that are ignored. |
| `volume_manager_selinux_pod_context_mismatch_errors_total` | gauge | ALPHA | access_mode | /metrics | Number of errors when a Pod defines different SELinux contexts for its containers that use the same volume. |
| `volume_manager_selinux_pod_context_mismatch_warnings_total` | gauge | ALPHA | access_mode | /metrics | Number of errors when a Pod defines different SELinux contexts for its containers that use the same volume. |
| `volume_manager_selinux_volume_context_mismatch_errors_total` | gauge | ALPHA | access_mode, volume_plugin | /metrics | Number of errors when a Pod uses a volume that is already mounted with a different SELinux context than the Pod needs. |
| `volume_manager_selinux_volume_context_mismatch_warnings_total` | gauge | ALPHA | access_mode, volume_plugin | /metrics | Number of errors when a Pod uses a volume that is already mounted with a different SELinux context than the Pod needs. |
| `volume_manager_selinux_volumes_admitted_total` | gauge | ALPHA | access_mode, volume_plugin | /metrics | Number of volumes whose SELinux context was fine and will be mounted with mount -o context option. |
| `volume_manager_total_volumes` | custom collector (gauge/counter) | ALPHA | plugin_name, state | /metrics | Number of volumes in Volume Manager |

Omitted as low operational value (info/config/internal bookkeeping): `kubelet_cgroup_version`, `kubelet_credential_provider_config_info`, `kubelet_cri_losing_support`, `kubelet_metrics_provider`, `kubelet_node_name`

## kube-proxy

| Metric | Type | Stability | Key labels | Meaning |
|---|---|---|---|---|
| `kubeproxy_conntrack_reconciler_deleted_entries_total` | counter | ALPHA | ip_family | Cumulative conntrack flows deleted by conntrack reconciler |
| `kubeproxy_conntrack_reconciler_sync_duration_seconds` | histogram | ALPHA | ip_family | ReconcileConntrackFlowsLatency latency in seconds |
| `kubeproxy_iptables_ct_state_invalid_dropped_packets_total` | custom collector (gauge/counter) | ALPHA | - | packets dropped by iptables to work around conntrack problems |
| `kubeproxy_iptables_localhost_nodeports_accepted_packets_total` | custom collector (gauge/counter) | ALPHA | - | Number of packets accepted on nodeports of loopback interface |
| **R** `kubeproxy_network_programming_duration_seconds` | histogram | ALPHA | ip_family | In Cluster Network Programming Latency in seconds |
| `kubeproxy_nftables_localhost_nodeport_listener_creation_failures_total` | counter | ALPHA | ip_family | Number of localhost NodePort userspace proxy listener creation failures. |
| `kubeproxy_nftables_localhost_nodeport_listeners` | gauge | ALPHA | ip_family | Number of active localhost NodePort userspace proxy listeners. |
| `kubeproxy_nftables_localhost_nodeport_rejected_packets_total` | custom collector (gauge/counter) | ALPHA | ip_family, protocol | Number of packets rejected on localhost NodePorts. |
| **R** `kubeproxy_proxy_healthz_total` | counter | ALPHA | code | Cumulative proxy healthz HTTP status |
| `kubeproxy_proxy_livez_total` | counter | ALPHA | code | Cumulative proxy livez HTTP status |
| `kubeproxy_sync_full_proxy_rules_duration_seconds` | histogram | ALPHA | ip_family | SyncProxyRules latency in seconds for full resyncs |
| `kubeproxy_sync_partial_proxy_rules_duration_seconds` | histogram | ALPHA | ip_family | SyncProxyRules latency in seconds for partial resyncs |
| **R** `kubeproxy_sync_proxy_rules_duration_seconds` | histogram | ALPHA | ip_family | SyncProxyRules latency in seconds |
| **R** `kubeproxy_sync_proxy_rules_endpoint_changes_pending` | gauge | ALPHA | - | Pending proxy rules Endpoint changes |
| `kubeproxy_sync_proxy_rules_endpoint_changes_total` | counter | ALPHA | - | Cumulative proxy rules Endpoint changes |
| `kubeproxy_sync_proxy_rules_iptables_last` | gauge | ALPHA | ip_family, table | Number of iptables rules written by kube-proxy in last sync |
| `kubeproxy_sync_proxy_rules_iptables_partial_restore_failures_total` | counter | ALPHA | ip_family | Cumulative proxy iptables partial restore failures |
| **R** `kubeproxy_sync_proxy_rules_iptables_restore_failures_total` | counter | ALPHA | ip_family | Cumulative proxy iptables restore failures |
| `kubeproxy_sync_proxy_rules_iptables_total` | gauge | ALPHA | ip_family, table | Total number of iptables rules owned by kube-proxy |
| **R** `kubeproxy_sync_proxy_rules_last_queued_timestamp_seconds` | gauge | ALPHA | ip_family | The last time a sync of proxy rules was queued |
| **R** `kubeproxy_sync_proxy_rules_last_timestamp_seconds` | gauge | ALPHA | ip_family | The last time proxy rules were successfully synced |
| `kubeproxy_sync_proxy_rules_nftables_cleanup_failures_total` | counter | ALPHA | ip_family | Cumulative proxy nftables cleanup failures |
| **R** `kubeproxy_sync_proxy_rules_nftables_sync_failures_total` | counter | ALPHA | ip_family | Cumulative proxy nftables sync failures |
| `kubeproxy_sync_proxy_rules_no_local_endpoints_total` | gauge | ALPHA | ip_family, traffic_policy | Number of services with a Local traffic policy and no endpoints |
| **R** `kubeproxy_sync_proxy_rules_service_changes_pending` | gauge | ALPHA | - | Pending proxy rules Service changes |
| `kubeproxy_sync_proxy_rules_service_changes_total` | counter | ALPHA | - | Cumulative proxy rules Service changes |
| `kubeproxy_sync_proxy_rules_winkernel_lb_create_failures_total` | counter | ALPHA | error, ip_family, lb_type | Cumulative proxy winkernel lb create failures |
| `kubeproxy_sync_proxy_rules_winkernel_lb_delete_failures_total` | counter | ALPHA | error, ip_family, lb_type | Cumulative proxy winkernel lb delete failures |
| `kubeproxy_sync_proxy_rules_winkernel_lb_update_failures_total` | counter | ALPHA | error, ip_family, lb_type | Cumulative proxy winkernel lb update failures |

## Shared families (client-go rest_client, workqueue, informer, leader election, healthchecks, component-base)

| Metric | Type | Stability | Key labels | Meaning |
|---|---|---|---|---|
| **R** `kubernetes_healthcheck` | gauge | STABLE | name, type | This metric records the result of a single healthcheck. |
| `kubernetes_healthchecks_total` | counter | STABLE | name, status, type | This metric records the results of all healthcheck. |
| `running_managed_controllers` | gauge | BETA | manager, name | Indicates where instances of a controller are currently running |
| **R** `workqueue_adds_total` | counter | BETA | name | Total number of adds handled by workqueue |
| **R** `workqueue_depth` | gauge | BETA | name | Current depth of workqueue |
| **R** `workqueue_longest_running_processor_seconds` | gauge | BETA | name | How many seconds has the longest running processor for workqueue been running. |
| **R** `workqueue_queue_duration_seconds` | histogram | BETA | name | How long in seconds an item stays in workqueue before being requested. |
| **R** `workqueue_retries_total` | counter | BETA | name | Total number of retries handled by workqueue |
| **R** `workqueue_unfinished_work_seconds` | gauge | BETA | name | How many seconds of work has done that is in progress and hasn't been observed by work_duration. |
| `workqueue_work_duration_seconds` | histogram | BETA | name | How long in seconds processing an item from workqueue takes. |
| `informer_processing_latency_seconds` | histogram | ALPHA | group, name, resource, version | Time taken to process events after popping from the queue. |
| `informer_queued_items` | gauge | ALPHA | group, name, resource, version | Number of items currently queued in the FIFO. |
| `informer_store_resource_version` | gauge | ALPHA | group, name, resource, version | The 15 least significant digits of the resource version of the store. |
| **R** `leader_election_master_status` | gauge | ALPHA | name | Gauge of if the reporting system is master of the relevant lease, 0 indicates backup, 1 indicates master. |
| **R** `leader_election_slowpath_total` | counter | ALPHA | name | Total number of slow path exercised in renewing leader leases. |
| `rest_client_dns_resolution_duration_seconds` | histogram | ALPHA | host | DNS resolver latency in seconds. |
| `rest_client_exec_plugin_call_total` | counter | ALPHA | call_status, code | Number of calls to an exec plugin, partitioned by the type of event encountered (no_error, plugin_execution_error, plugin_not_found_error, client_internal_error) and a... |
| `rest_client_exec_plugin_certificate_rotation_age` | histogram | ALPHA | - | Histogram of the number of seconds the last auth exec plugin client certificate lived before being rotated. |
| `rest_client_exec_plugin_policy_call_total` | counter | ALPHA | allowed, denied | Number of comparisons of an exec plugin to the plugin policy and allowlist (if any), partitioned by whether or not the policy permits the plugin |
| `rest_client_exec_plugin_ttl_seconds` | gauge | ALPHA | - | Gauge of the shortest TTL (time-to-live) of the client certificate(s) managed by the auth exec plugin. |
| **R** `rest_client_rate_limiter_duration_seconds` | histogram | ALPHA | host, verb | Client side rate limiter latency in seconds. |
| **R** `rest_client_request_duration_seconds` | histogram | ALPHA | host, verb | Request latency in seconds. |
| `rest_client_request_retries_total` | counter | ALPHA | code, host, verb | Number of request retries, partitioned by status code, verb, and host. |
| `rest_client_request_size_bytes` | histogram | ALPHA | host, verb | Request size in bytes. |
| **R** `rest_client_requests_total` | counter | ALPHA | code, host, method | Number of HTTP requests, partitioned by status code, method, and host. |
| `rest_client_response_size_bytes` | histogram | ALPHA | host, verb | Response size in bytes. |
| `rest_client_transport_ca_reload_total` | counter | ALPHA | reason, result | Number of times a CA reload is attempted, partitioned by the result and reason for the reload attempt |

Omitted as low operational value (info/config/internal bookkeeping): `disabled_metrics_total`, `hidden_metrics_total`, `kubernetes_build_info`, `kubernetes_feature_enabled`, `registered_metrics_total`, `rest_client_transport_cache_entries`, `rest_client_transport_cache_gc_calls_total`, `rest_client_transport_cert_rotation_gc_calls_total`, `rest_client_transport_create_calls_total`, `version_info`