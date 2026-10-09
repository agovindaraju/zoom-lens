# Prometheus metric catalogue: node_exporter + cAdvisor (kubelet /metrics/cadvisor)

Researched 2026-09-30 against `master` of:

- prometheus/node_exporter: README.md, `collector/*.go` (each metric name below was read from a `BuildFQName`/`NewDesc` call or a map key in the source), `docs/node-mixin/{alerts,rules,dashboards,lib,config.libsonnet}`
- google/cadvisor: `docs/storage/prometheus.md`, `lib/metrics/prometheus.go`, `lib/metrics/prometheus_machine.go`, `lib/container/factory.go`, `cmd/cadvisor.go`
- kubernetes/kubernetes: `pkg/kubelet/server/server.go` (the metric set the kubelet exposes at /metrics/cadvisor) and `pkg/features/kube_features.go` (KubeletPSI)
- kubernetes-monitoring/kubernetes-mixin: `rules/`, `alerts/`, `dashboards/`

Conventions: `node_` namespace. "Def" = collector is enabled by default (`registerCollector(..., defaultEnabled, ...)`). Types: C = counter, G = gauge, U = untyped (as emitted in source). `rate()` works on U counters too.

---------------------------------------------------------------------------------------------------

## PART A. node_exporter

### A.0 Collector enablement summary (from README and source)

Enabled by default (Linux-relevant): arp, bcache, bonding, btrfs, conntrack, cpu, cpufreq, diskstats, dmi, dmmultipath, edac, entropy, fibrechannel, filefd, filesystem, hwmon, infiniband, ipvs, kernel_hung, loadavg, mdadm, meminfo, netclass, netdev, netstat, nfs, nfsd, nvme, os, powersupplyclass, pressure, rapl, schedstat, selinux, sockstat, softnet, stat, tapestats, textfile, thermal_zone, time, timex, udp_queues, uname, vmstat, watchdog, xfs, zfs.

Disabled by default: buddyinfo, cgroups, cpu_vulnerabilities, drm, drbd, **ethtool**, interrupts, ksmd, lnstat, logind, meminfo_numa, mountstats, network_route, nvmesubsystem, pcidevice, perf, **processes**, qdisc, slabinfo, softirqs, sysctl, swap, **systemd**, tcpstat, wifi, xfrm, zoneinfo.

Deprecated: ntp, runit, supervisord.

Enable with `--collector.<name>`, disable with `--no-collector.<name>`.

Exporter self-metrics (always present, from `collector/collector.go`):

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| node_scrape_collector_duration_seconds | G | collector | How long each collector took. |
| node_scrape_collector_success | G | collector | 1 if the collector succeeded. Use this to detect broken collectors. |

### A.1 CPU: collector `cpu` (Def), `cpufreq` (Def)

| Metric | Type | Labels | Collector / gate | Meaning |
|---|---|---|---|---|
| node_cpu_seconds_total | C | cpu, mode (user,nice,system,idle,iowait,irq,softirq,steal) | cpu, Def | CPU time per mode, from /proc/stat. **Root.** |
| node_cpu_guest_seconds_total | C | cpu, mode (user,nice) | cpu, Def (`--collector.cpu.guest`, default true) | Time spent running guests (VMs). |
| node_cpu_core_throttles_total | C | package, core | cpu, Def | Thermal throttle events per core. |
| node_cpu_package_throttles_total | C | package | cpu, Def | Thermal throttle events per package. |
| node_cpu_isolated | G | cpu | cpu, Def (only when isolated CPUs exist) | 1 for each isolcpus CPU. |
| node_cpu_online | G | cpu | cpu, Def | 1 if the CPU is online. |
| node_cpu_info | G | package, core, cpu, vendor, family, model, model_name, microcode, stepping, cachesize | cpu, needs `--collector.cpu.info` | Constant 1 with CPU info labels. |
| node_cpu_frequency_hertz | G | package, core, cpu | cpu, needs `--collector.cpu.info` (only if cpufreq is off or uses its prefix) | CPU frequency from /proc/cpuinfo. |
| node_cpu_flag_info | G | flag | cpu, needs `--collector.cpu.info.flags-include` | CPU flag present. |
| node_cpu_bug_info | G | bug | cpu, needs `--collector.cpu.info.bugs-include` | CPU bug present. |
| node_cpu_frequency_hertz | G | cpu | cpufreq, Def | Current CPU frequency (cpuinfo_cur_freq). |
| node_cpu_frequency_avg_hertz | G | cpu | cpufreq, Def | Average frequency. |
| node_cpu_frequency_min_hertz / node_cpu_frequency_max_hertz | G | cpu | cpufreq, Def | Hardware min/max frequency. |
| node_cpu_scaling_frequency_hertz | G | cpu | cpufreq, Def | Current scaled frequency. |
| node_cpu_scaling_frequency_min_hertz / node_cpu_scaling_frequency_max_hertz | G | cpu | cpufreq, Def | Scaling min/max. |
| node_cpu_scaling_governor | G | cpu, governor | cpufreq, Def | 1 for the active governor. |

Note: with `--collector.cpufreq.enable-cpufreq-prefix` the cpufreq metrics use the `node_cpufreq_` subsystem instead of `node_cpu_`.

### A.2 Load, scheduler, kernel counters: `loadavg` (Def), `stat` (Def), `schedstat` (Def), `kernel_hung` (Def)

| Metric | Type | Labels | Collector | Meaning |
|---|---|---|---|---|
| node_load1 | G | - | loadavg | 1m load average. **Root** (node-mixin NodeSystemSaturation). |
| node_load5 | G | - | loadavg | 5m load average. |
| node_load15 | G | - | loadavg | 15m load average. |
| node_procs_running | G | - | stat | Processes in runnable state. |
| node_procs_blocked | G | - | stat | Processes blocked waiting for I/O. |
| node_context_switches_total | C | - | stat | Context switches. |
| node_forks_total | C | - | stat | Forks. |
| node_intr_total | C | - | stat | Interrupts serviced. |
| node_boot_time_seconds | G | - | stat | Boot time as a unix timestamp (use it to detect reboots). |
| node_softirqs_total | C | vector | stat | Softirqs per vector. |
| node_schedstat_running_seconds_total | C | cpu | schedstat | Time CPU spent running tasks. |
| node_schedstat_waiting_seconds_total | C | cpu | schedstat | Time tasks spent waiting on the runqueue for this CPU. This is a scheduler latency signal. |
| node_schedstat_timeslices_total | C | cpu | schedstat | Timeslices executed. |
| node_kernel_hung_tasks_total | C | - | kernel_hung | Hung tasks detected (/proc/sys/kernel/hung_task_detect_count). |

### A.3 Pressure stall information (PSI): collector `pressure` (Def; kernel 4.20+ with CONFIG_PSI; IRQ needs 6.1+)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| node_pressure_cpu_waiting_seconds_total | C | - | "some" CPU: time processes waited for CPU. |
| node_pressure_io_waiting_seconds_total | C | - | "some" IO: time processes waited on IO. |
| node_pressure_io_stalled_seconds_total | C | - | "full" IO: time no process could make progress because of IO. |
| node_pressure_memory_waiting_seconds_total | C | - | "some" memory: time processes waited for memory. |
| node_pressure_memory_stalled_seconds_total | C | - | "full" memory: time no process could make progress because of memory. |
| node_pressure_irq_stalled_seconds_total | C | - | "full" IRQ stall time. |

The source emits no "full" CPU metric. PSI metrics are good cheap root candidates for saturation, although the node-mixin does not use them.

### A.4 Memory: `meminfo` (Def), `vmstat` (Def)

meminfo: each /proc/meminfo field becomes `node_memory_<Field>_bytes`. All are G, with no labels. Field keys found in `meminfo_linux.go`:

Active, Active_anon, Active_file, AnonHugePages, AnonPages, Bounce, Buffers, Cached, CmaFree, CmaTotal, CommitLimit, Committed_AS, DirectMap1G, DirectMap2M, DirectMap4k, Dirty, HardwareCorrupted, Hugepagesize, Inactive, Inactive_anon, Inactive_file, KernelStack, Mapped, MemAvailable, MemFree, MemTotal, Mlocked, NFS_Unstable, PageTables, Percpu, SReclaimable, SUnreclaim, Shmem, ShmemHugePages, ShmemPmdMapped, Slab, SwapCached, SwapFree, SwapTotal, Unevictable, VmallocChunk, VmallocTotal, VmallocUsed, Writeback, WritebackTmp, Zswap, Zswapped (each with a `_bytes` suffix)

Also, without a `_bytes` suffix (these are page counts): node_memory_HugePages_Free, node_memory_HugePages_Rsvd, node_memory_HugePages_Surp, node_memory_HugePages_Total.

Key ones:

| Metric | Meaning |
|---|---|
| node_memory_MemTotal_bytes | Total usable RAM. **Root** |
| node_memory_MemAvailable_bytes | Estimated memory available without swapping. **Root** (NodeMemoryHighUtilization) |
| node_memory_MemFree_bytes, node_memory_Buffers_bytes, node_memory_Cached_bytes, node_memory_Slab_bytes | Used by the node-mixin memory rule and dashboards. |
| node_memory_SwapTotal_bytes / node_memory_SwapFree_bytes | Swap capacity and free swap. |
| node_memory_Dirty_bytes / node_memory_Writeback_bytes | Writeback backlog. |
| node_memory_Committed_AS_bytes / node_memory_CommitLimit_bytes | Overcommit. |
| node_memory_HardwareCorrupted_bytes | Memory poisoned by hardware errors. |

vmstat: `node_vmstat_<field>` is U with no labels. The default field filter `--collector.vmstat.fields` is `^(oom_kill|pgpg|pswp|pg.*fault).*`. It matches these /proc/vmstat fields:

| Metric | Meaning |
|---|---|
| node_vmstat_oom_kill | Kernel OOM kills (counter). **Root candidate** |
| node_vmstat_pgmajfault | Major page faults (counter). **Root** (NodeMemoryMajorPagesFaults: rate > 500 for 15m) |
| node_vmstat_pgfault | Minor and major page faults. |
| node_vmstat_pgpgin / node_vmstat_pgpgout | Pages paged in and out. |
| node_vmstat_pswpin / node_vmstat_pswpout | Pages swapped in and out. |

The names are kernel field names. The regex list comes from source, and these specific names appear in the node-mixin (pgmajfault) or follow directly from the regex applied to standard kernel vmstat fields. Other fields need `--collector.vmstat.fields`.

### A.5 Disk I/O: `diskstats` (Def). Labels: `device` (plus extras on info metrics)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| node_disk_reads_completed_total | C | device | Reads completed. |
| node_disk_writes_completed_total | C | device | Writes completed. |
| node_disk_read_bytes_total | C | device | Bytes read. |
| node_disk_written_bytes_total | C | device | Bytes written. |
| node_disk_read_time_seconds_total | C | device | Time spent on reads (use with reads_completed for latency). |
| node_disk_write_time_seconds_total | C | device | Time spent on writes. |
| node_disk_io_time_seconds_total | C | device | Time the device was busy (utilisation). **Root** |
| node_disk_io_time_weighted_seconds_total | C | device | Weighted busy time (queue depth). **Root** (NodeDiskIOSaturation: rate > 10 for 30m) |
| node_disk_io_now | G | device | I/Os in progress. |
| node_disk_reads_merged_total / node_disk_writes_merged_total | C | device | Merged requests. |
| node_disk_discards_completed_total, node_disk_discards_merged_total, node_disk_discarded_sectors_total, node_disk_discard_time_seconds_total | C | device | Discard/TRIM stats. |
| node_disk_flush_requests_total, node_disk_flush_requests_time_seconds_total | C | device | Flush stats. |
| node_disk_info | G | device, major, minor, path, wwn, model, serial, revision, rotational | Device info. |
| node_disk_filesystem_info | G | device, type, usage, uuid, version | udev filesystem info. |
| node_disk_device_mapper_info | G | device, name, uuid, vg_name, lv_name, lv_layer | DM/LVM mapping. |
| node_disk_device_mapper_backing_device_info | G | device, backing_device | DM backing device. |
| node_disk_ata_write_cache, node_disk_ata_write_cache_enabled, node_disk_ata_rotation_rate_rpm | G | device | ATA properties (udev). |

### A.6 Filesystem: `filesystem` (Def). Labels: `device, mountpoint, fstype, device_error`

| Metric | Type | Meaning |
|---|---|---|
| node_filesystem_size_bytes | G | Filesystem size. **Root** |
| node_filesystem_avail_bytes | G | Space available to non-root users. **Root** (NodeFilesystemSpaceFillingUp / AlmostOutOfSpace) |
| node_filesystem_free_bytes | G | Free space, including root-reserved space. |
| node_filesystem_files | G | Total inodes. |
| node_filesystem_files_free | G | Free inodes. **Root** (NodeFilesystemFilesFillingUp / AlmostOutOfFiles) |
| node_filesystem_readonly | G | 1 if mounted read-only (used as a guard in alerts). |
| node_filesystem_device_error | G | 1 if statfs failed for the mount. |
| node_filesystem_purgeable_bytes | G | Purgeable space (macOS). |
| node_filesystem_mount_info | G | Labels are device, major, minor, mountpoint. Mount info. |

Default mount-point and fs-type exclude flags drop /dev, /proc, /sys, and the kubelet/docker paths shown in the README example. Container overlay mounts are therefore mostly excluded.

### A.7 Network

#### netdev (Def). Label `device`. Default Linux path is netlink, with legacy names unless `--collector.netdev.enable-detailed-metrics` is set

All C, `node_network_<key>_total`:

| Metric | Meaning |
|---|---|
| node_network_receive_bytes_total / node_network_transmit_bytes_total | Bytes. **Root** |
| node_network_receive_packets_total / node_network_transmit_packets_total | Packets. |
| node_network_receive_errs_total / node_network_transmit_errs_total | Errors. **Root** (NodeNetworkReceiveErrs / TransmitErrs) |
| node_network_receive_drop_total / node_network_transmit_drop_total | Drops (receive_drop includes rx_missed_errors). **Root candidate** |
| node_network_receive_fifo_total / node_network_transmit_fifo_total | FIFO errors. |
| node_network_receive_frame_total | Frame, length, over, and CRC errors. |
| node_network_receive_multicast_total | Multicast received. |
| node_network_receive_compressed_total / node_network_transmit_compressed_total | Compressed packets. |
| node_network_receive_nohandler_total | Packets dropped because there was no protocol handler. |
| node_network_transmit_colls_total | Collisions. |
| node_network_transmit_carrier_total | Carrier, aborted, heartbeat, and window errors. |
| node_network_address_info (G; device, address, netmask, scope) | Only with `--collector.netdev.address-info`. |

With `--collector.netdev.enable-detailed-metrics` the raw keys are used instead: receive_errors, receive_dropped, receive_missed_errors, receive_crc_errors, receive_length_errors, receive_over_errors, receive_frame_errors, receive_fifo_errors, transmit_errors, transmit_dropped, transmit_aborted_errors, transmit_carrier_errors, transmit_fifo_errors, transmit_heartbeat_errors, transmit_window_errors, multicast, collisions, each with a `_total` suffix.

#### netclass (Def). Label `device` (from /sys/class/net)

| Metric | Type | Meaning |
|---|---|---|
| node_network_up | G | 1 if operstate is "up". |
| node_network_info | G | Labels are device, address, broadcast, duplex, operstate, adminstate, ifalias. |
| node_network_carrier | G | Carrier present. |
| node_network_carrier_changes_total, node_network_carrier_up_changes_total, node_network_carrier_down_changes_total | C | Link flaps. |
| node_network_mtu_bytes, node_network_speed_bytes, node_network_transmit_queue_length | G | Interface properties. |
| node_network_address_assign_type, node_network_device_id, node_network_dormant, node_network_flags, node_network_iface_id, node_network_iface_link, node_network_iface_link_mode, node_network_name_assign_type, node_network_net_dev_group, node_network_protocol_type | G | Other sysfs properties. |

#### netstat (Def). `node_netstat_<Proto>_<Field>`, U, no labels

Default `--collector.netstat.fields` regex applied to the 311 generated descriptors in `netstat_descs_linux.go`. It exports exactly these 42:

- TCP: node_netstat_Tcp_ActiveOpens, node_netstat_Tcp_PassiveOpens, node_netstat_Tcp_CurrEstab (gauge-like), node_netstat_Tcp_InSegs, node_netstat_Tcp_OutSegs, node_netstat_Tcp_RetransSegs, node_netstat_Tcp_InErrs, node_netstat_Tcp_OutRsts
- TcpExt: node_netstat_TcpExt_ListenOverflows, node_netstat_TcpExt_ListenDrops, node_netstat_TcpExt_SyncookiesSent, node_netstat_TcpExt_SyncookiesRecv, node_netstat_TcpExt_SyncookiesFailed, node_netstat_TcpExt_TCPTimeouts, node_netstat_TcpExt_TCPSynRetrans, node_netstat_TcpExt_TCPOFOQueue, node_netstat_TcpExt_TCPRcvQDrop
- IP: node_netstat_Ip_Forwarding, node_netstat_IpExt_InOctets, node_netstat_IpExt_OutOctets, node_netstat_Ip6_InOctets, node_netstat_Ip6_OutOctets
- ICMP: node_netstat_Icmp_InMsgs, node_netstat_Icmp_OutMsgs, node_netstat_Icmp_InErrors, node_netstat_Icmp6_InMsgs, node_netstat_Icmp6_OutMsgs, node_netstat_Icmp6_InErrors
- UDP: node_netstat_Udp_InDatagrams, node_netstat_Udp_OutDatagrams, node_netstat_Udp_NoPorts, node_netstat_Udp_InErrors, node_netstat_Udp_RcvbufErrors, node_netstat_Udp_SndbufErrors, node_netstat_Udp6_InDatagrams, node_netstat_Udp6_OutDatagrams, node_netstat_Udp6_NoPorts, node_netstat_Udp6_InErrors, node_netstat_Udp6_RcvbufErrors, node_netstat_Udp6_SndbufErrors, node_netstat_UdpLite_InErrors, node_netstat_UdpLite6_InErrors

The kubernetes-mixin network dashboards use Tcp_RetransSegs, Tcp_OutSegs, and TcpExt_TCPSynRetrans (retransmit ratio).

#### sockstat (Def). G, no labels

- node_sockstat_sockets_used: IPv4 sockets in use.
- `node_sockstat_<PROTO>_<state>` where the state is one of inuse, orphan, tw, alloc, mem, memory, and mem_bytes (mem in bytes, only where mem exists). The protocols are whatever /proc/net/sockstat{,6} report. Typical names are node_sockstat_TCP_inuse, node_sockstat_TCP_tw, node_sockstat_TCP_orphan, node_sockstat_TCP_alloc, node_sockstat_TCP_mem_bytes, node_sockstat_UDP_inuse, node_sockstat_TCP6_inuse. These are built dynamically, so the exact set depends on the kernel.

#### conntrack (Def). G, no labels

| Metric | Meaning |
|---|---|
| node_nf_conntrack_entries | Current conntrack entries. **Root** (NodeHighNumberConntrackEntriesUsed: entries / limit > 0.75) |
| node_nf_conntrack_entries_limit | nf_conntrack_max. **Root** |
| node_nf_conntrack_stat_found, node_nf_conntrack_stat_invalid, node_nf_conntrack_stat_ignore, node_nf_conntrack_stat_insert, node_nf_conntrack_stat_insert_failed, node_nf_conntrack_stat_drop, node_nf_conntrack_stat_early_drop, node_nf_conntrack_stat_search_restart | Per-kernel conntrack stat counters, summed over CPUs. insert_failed and drop mean the conntrack table is full or racing. |

#### softnet (Def). Label `cpu`

node_softnet_processed_total (C), node_softnet_dropped_total (C; backlog overflow drops), node_softnet_times_squeezed_total (C; NAPI budget exhausted), node_softnet_cpu_collision_total (C), node_softnet_received_rps_total (C), node_softnet_flow_limit_count_total (C), node_softnet_backlog_len (G).

#### arp (Def)

node_arp_entries (G, label device): ARP table entries per device.

#### udp_queues (Def)

node_udp_queues (G, labels queue, ip): total rx_queue/tx_queue lengths from /proc/net/udp{,6}.

#### bonding (Def)

node_bonding_slaves (G, label master) and node_bonding_active (G, label master) give configured and active slaves. The node-mixin NodeBondingDegraded alert uses them.

#### ethtool (NOT default; `--collector.ethtool`). Label `device`

| Metric | Type | Meaning |
|---|---|---|
| node_ethtool_info | G | Labels are bus_info, device, driver, expansion_rom_version, firmware_version, version. |
| node_ethtool_received_bytes_total, node_ethtool_received_dropped_total, node_ethtool_received_errors_total, node_ethtool_received_packets_total | C | Mapped standard driver stats. |
| node_ethtool_transmitted_bytes_total, node_ethtool_transmitted_errors_total, node_ethtool_transmitted_packets_total | C | Mapped standard driver stats. |
| `node_ethtool_<driver_stat>` | U | Every other `ethtool -S` stat, with the name sanitized and lowercased (rx→received, tx→transmitted). Driver-specific; filter with `--collector.ethtool.metrics-include`. |
| node_network_supported_port_info (device, type), node_network_supported_speed_bytes (device, duplex, mode), node_network_advertised_speed_bytes (device, duplex, mode), node_network_autonegotiate, node_network_autonegotiate_supported, node_network_autonegotiate_advertised, node_network_pause_supported, node_network_pause_advertised, node_network_asymmetricpause_supported, node_network_asymmetricpause_advertised | G | Link settings. |

### A.8 File descriptors, entropy, processes

| Metric | Type | Labels | Collector | Meaning |
|---|---|---|---|---|
| node_filefd_allocated | G | - | filefd, Def | Allocated FDs system-wide. **Root** (NodeFileDescriptorLimit: allocated/maximum > 70% warning, > 90% critical) |
| node_filefd_maximum | G | - | filefd, Def | fs.file-max. |
| node_entropy_available_bits | G | - | entropy, Def | Available entropy. |
| node_entropy_pool_size_bits | G | - | entropy, Def | Entropy pool size. |
| node_processes_threads | G | - | processes, NOT default | Allocated threads. |
| node_processes_max_threads | G | - | processes, NOT default | Thread limit (kernel.threads-max). |
| node_processes_threads_state | G | thread_state | processes, NOT default | Threads per state. |
| node_processes_state | G | state | processes, NOT default | Processes per state (R, S, D, Z, ...). Useful for D-state and zombie detection. |
| node_processes_pids | G | - | processes, NOT default | PIDs in use. |
| node_processes_max_processes | G | - | processes, NOT default | pid_max. PID exhaustion is pids / max_processes. |

### A.9 systemd (NOT default; `--collector.systemd`, needs dbus)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| node_systemd_unit_state | G | name, state (active, activating, deactivating, inactive, failed), type | 1 for the current state. **Root** (NodeSystemdServiceFailed: state="failed" == 1). Covers kubelet, containerd, and similar units. |
| node_systemd_units | G | state | Count of units per state. |
| node_systemd_system_running | G | - | 1 if `systemctl is-system-running` reports running. |
| node_systemd_service_restart_total | C | name | Restart triggers. Needs `--collector.systemd.enable-restarts-metrics`. Used by NodeSystemdServiceCrashlooping. |
| node_systemd_unit_start_time_seconds | G | name | Needs `--collector.systemd.enable-start-time-metrics`. |
| node_systemd_unit_tasks_current / node_systemd_unit_tasks_max | G | name | Needs `--collector.systemd.enable-task-metrics`. |
| node_systemd_timer_last_trigger_seconds | G | name | Last timer trigger. |
| node_systemd_socket_accepted_connections_total, node_systemd_socket_refused_connections_total | C | name | Socket units. |
| node_systemd_socket_current_connections | G | name | Socket units. |
| node_systemd_version | G | version | systemd version. |
| node_systemd_virtualization_info | G | virtualization_type | Detected virtualization. |

The default unit-exclude is `.+\.(automount|device|mount|scope|slice)`.

### A.10 Time / clock: `time` (Def), `timex` (Def)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| node_time_seconds | G | - | System time (compare with the scrape timestamp for skew). |
| node_time_zone_offset_seconds | G | time_zone | TZ offset. |
| node_time_clocksource_available_info / node_time_clocksource_current_info | G | device, clocksource | Clocksources. |
| node_timex_offset_seconds | G | - | Clock offset. **Root** (NodeClockSkewDetected) |
| node_timex_sync_status | G | - | 1 if the clock is synchronised. **Root** (NodeClockNotSynchronising) |
| node_timex_maxerror_seconds | G | - | Max error (used in NodeClockNotSynchronising). |
| node_timex_estimated_error_seconds, node_timex_frequency_adjustment_ratio, node_timex_status, node_timex_loop_time_constant, node_timex_tick_seconds, node_timex_tai_offset_seconds | G | - | adjtimex fields. |
| node_timex_pps_frequency_hertz, node_timex_pps_jitter_seconds, node_timex_pps_shift_seconds, node_timex_pps_stability_hertz | G | - | PPS stats. |
| node_timex_pps_jitter_total, node_timex_pps_calibration_total, node_timex_pps_error_total, node_timex_pps_stability_exceeded_total | C | - | PPS counters. |

### A.11 Hardware health: `edac` (Def), `hwmon` (Def), `thermal_zone` (Def), `mdadm` (Def)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| node_edac_correctable_errors_total | C | controller | Correctable ECC errors. |
| node_edac_uncorrectable_errors_total | C | controller | Uncorrectable ECC errors. **Root candidate** |
| node_edac_csrow_correctable_errors_total / node_edac_csrow_uncorrectable_errors_total | C | controller, csrow | Per csrow. |
| node_edac_channel_correctable_errors_total / node_edac_channel_uncorrectable_errors_total | C | controller, csrow, channel, dimm_label | Per channel/DIMM. |
| node_thermal_zone_temp | G | zone, type | Thermal zone temperature (Celsius). |
| node_cooling_device_cur_state / node_cooling_device_max_state | G | name, type | Cooling device throttle state. |
| node_hwmon_temp_celsius (and `node_hwmon_temp_<element>_celsius` such as crit, max) | G | chip, sensor | Temperature sensors. Names are built dynamically: `node_hwmon_<type>[_<element>]<unit suffix>`. |
| node_hwmon_in_volts (and `node_hwmon_in_<element>_volts`), node_hwmon_curr_amps, node_hwmon_power_watt, node_hwmon_energy_joule_total, node_hwmon_fan_rpm (and `node_hwmon_fan_<min/max/target>_rpm`) | G/C | chip, sensor | Other sensors (dynamic). |
| `node_hwmon_<type>_alarm`, `node_hwmon_<type>_fault` | G | chip, sensor | Sensor alarm/fault flags (dynamic). |
| node_hwmon_chip_names | G | chip, chip_name | Chip name mapping. |
| node_hwmon_sensor_label | G | chip, sensor, label | Sensor labels. |
| node_hwmon_beep_enabled, node_hwmon_voltage_regulator_version, node_hwmon_update_interval_seconds | G | chip, sensor | Misc. |
| node_md_state | G | device, state (active, inactive, recovering, resync, check) | MD array state. |
| node_md_disks | G | device, state | Disks per state. **Root** (NodeRAIDDiskFailure: state="failed" > 0) |
| node_md_disks_required | G | device | Required disks. **Root** (NodeRAIDDegraded: required - active > 0) |
| node_md_blocks, node_md_blocks_synced, node_md_raid_disks, node_md_degraded | G | device | Sync progress / degraded disks. |

### A.12 Host identity / info: `uname` (Def), `os` (Def)

- node_uname_info (G): labels sysname, release, version, machine, nodename, domainname. Kernel version join key.
- node_os_info (G): labels include build_id, id, id_like, image_id, image_version, name, pretty_name (and more). OS release.
- node_os_version (G): labels id, id_like, name. Numeric OS version.
- node_os_support_end_timestamp_seconds (G): OS support end date.

### A.13 NFS client: `nfs` (Def)

node_nfs_packets_total (C, protocol), node_nfs_connections_total (C), node_nfs_rpcs_total (C), node_nfs_rpc_retransmissions_total (C), node_nfs_rpc_authentication_refreshes_total (C), node_nfs_requests_total (C, labels proto, method).

### A.14 XFS / ZFS: `xfs` (Def, label `device`), `zfs` (Def, dynamic)

XFS, all C, `node_xfs_<name>`: extent_allocation_extents_allocated_total, extent_allocation_blocks_allocated_total, extent_allocation_extents_freed_total, extent_allocation_blocks_freed_total, allocation_btree_lookups_total, allocation_btree_compares_total, allocation_btree_records_inserted_total, allocation_btree_records_deleted_total, block_mapping_reads_total, block_mapping_writes_total, block_mapping_unmaps_total, block_mapping_extent_list_insertions_total, block_mapping_extent_list_deletions_total, block_mapping_extent_list_lookups_total, block_mapping_extent_list_compares_total, block_map_btree_lookups_total, block_map_btree_compares_total, block_map_btree_records_inserted_total, block_map_btree_records_deleted_total, directory_operation_lookup_total, directory_operation_create_total, directory_operation_remove_total, directory_operation_getdents_total, inode_operation_attempts_total, inode_operation_found_total, inode_operation_recycled_total, inode_operation_missed_total, inode_operation_duplicates_total, inode_operation_reclaims_total, inode_operation_attribute_changes_total, read_calls_total, write_calls_total, vnode_active_total, vnode_allocate_total, vnode_get_total, vnode_hold_total, vnode_release_total, vnode_reclaim_total, vnode_remove_total.

ZFS names are generated from kstat files under /proc/spl/kstat/zfs as `node_zfs_<subsystem>_<kstat>`, plus `node_zfs_zpool_*` and `node_zfs_zpool_dataset_*`. They are not enumerable from source; they are only present on ZFS hosts.

### A.15 textfile (Def)

node_textfile_mtime_seconds (G, file) and node_textfile_scrape_error (G). NodeTextFileCollectorScrapeError fires on scrape_error == 1.

### A.16 node-mixin usage (good root candidates)

Alerts (`docs/node-mixin/alerts/alerts.libsonnet`) and the metrics each uses:

| Alert | Metrics / threshold (config.libsonnet defaults) |
|---|---|
| NodeFilesystemSpaceFillingUp (warn/crit) | node_filesystem_avail_bytes, node_filesystem_size_bytes, node_filesystem_readonly; predict_linear, avail < 40% / 20% |
| NodeFilesystemAlmostOutOfSpace | same; avail < 5% / 3% |
| NodeFilesystemFilesFillingUp / NodeFilesystemAlmostOutOfFiles | node_filesystem_files_free, node_filesystem_files, node_filesystem_readonly |
| NodeNetworkReceiveErrs / NodeNetworkTransmitErrs | node_network_receive_errs_total / node_network_receive_packets_total (and transmit) |
| NodeHighNumberConntrackEntriesUsed | node_nf_conntrack_entries / node_nf_conntrack_entries_limit |
| NodeTextFileCollectorScrapeError | node_textfile_scrape_error |
| NodeClockSkewDetected | node_timex_offset_seconds |
| NodeClockNotSynchronising | node_timex_sync_status, node_timex_maxerror_seconds |
| NodeRAIDDegraded / NodeRAIDDiskFailure | node_md_disks_required, node_md_disks |
| NodeFileDescriptorLimit (warn/crit) | node_filefd_allocated, node_filefd_maximum |
| NodeCPUHighUsage | node_cpu_seconds_total (mode!~"idle\|iowait") > 90% for 15m |
| NodeSystemSaturation | node_load1 / count(node_cpu_seconds_total{mode="idle"}) > 2 for 15m |
| NodeMemoryMajorPagesFaults | rate(node_vmstat_pgmajfault[5m]) > 500 |
| NodeMemoryHighUtilization | node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes, > 90% used |
| NodeDiskIOSaturation | rate(node_disk_io_time_weighted_seconds_total[5m]) > 10 for 30m |
| NodeSystemdServiceFailed | node_systemd_unit_state{state="failed"} |
| NodeSystemdServiceCrashlooping | node_systemd_service_restart_total |
| NodeBondingDegraded | node_bonding_slaves, node_bonding_active |

Recording rules (`rules/rules.libsonnet`): instance:node_num_cpu:sum, instance:node_cpu_utilisation:rate5m, instance:node_load1_per_cpu:ratio, instance:node_memory_utilisation:ratio (uses MemAvailable, MemTotal, and the MemFree/Buffers/Cached/Slab fallback), instance:node_vmstat_pgmajfault:rate5m, instance_device:node_disk_io_time_seconds:rate5m, instance_device:node_disk_io_time_weighted_seconds:rate5m, instance:node_network_{receive,transmit}_{bytes,drop}_excluding_lo:rate5m, instance:node_network_{receive,transmit}_{bytes,drop}_physical:rate5m.

Dashboards (node.libsonnet, use.libsonnet, lib/prom-mixin.libsonnet) additionally reference node_load5, node_load15, node_disk_read_bytes_total, node_disk_written_bytes_total, node_network_receive_bytes_total, node_network_transmit_bytes_total, node_time_seconds, node_uname_info, node_exporter_build_info. They also reference macOS-only memory names (node_memory_total_bytes, node_memory_available_bytes, node_memory_wired_bytes, and others), which are not Linux.

The kubernetes-mixin `rules/node.libsonnet` also uses node_cpu_seconds_total, node_memory_MemAvailable_bytes (with the MemFree/Buffers/Cached/Slab fallback), and node_memory_MemTotal_bytes. Its records are node:node_num_cpu:sum, :node_memory_MemAvailable_bytes:sum, node:node_cpu_utilization:ratio_rate5m, and cluster:node_cpu:ratio_rate5m.

---------------------------------------------------------------------------------------------------

## PART B. cAdvisor (via kubelet `/metrics/cadvisor`)

### B.0 What the kubelet actually exposes

From `pkg/kubelet/server/server.go`, the kubelet builds cAdvisor's collector with ONLY these metric kinds:

`cpu`, `memory`, `diskIO`, `disk`, `network`, `app`, `process`, `oom_event`, plus `pressure` if the **KubeletPSI** feature gate is enabled AND the node supports PSI. KubeletPSI is alpha/off in 1.33, beta/on in 1.34, and GA/locked-on in 1.36.

So the kubelet does NOT expose these kinds at all: `cpuLoad` (container_cpu_load_average_10s, container_cpu_load_d_average_10s, container_tasks_state), `sched` (container_cpu_schedstat_*), `percpu` (so container_cpu_usage_seconds_total has only `cpu="total"`), `hugetlb`, `memory_numa`, `cpuset` (container_memory_migrate), `tcp`, `udp`, `advtcp`, `perf_event`, `referenced_memory`, `resctrl`, and `cpu_topology` (machine_cpu_cache_capacity_bytes, machine_thread_siblings_count, machine_node_*). You cannot re-enable them through kubelet flags; you need a standalone cAdvisor DaemonSet.

Note: if the `PodAndContainerStatsFromCRI` feature gate is on, the kubelet serves CRI-provided metrics on this endpoint instead of the cAdvisor collector, so the set may differ.

Standalone cAdvisor defaults (`cmd/cadvisor.go` ignoreMetrics, i.e. `-disable_metrics` default): memory_numa, tcp, udp, advtcp, sched, process, hugetlb, referenced_memory, cpu_topology, resctrl, cpuset. `perf_event` also needs a libpfm build and config.

### B.1 Identifying labels

Every `container_*` series carries the labels from kubelet `containerPrometheusLabelsFunc`:

- `id`: cgroup path (for example `/kubepods.slice/.../cri-containerd-<id>.scope`). `id="/"` is the root cgroup, which is the whole node.
- `name`: runtime container name (first alias).
- `image`: container image. The kubelet leaves this empty for pod-level and sandbox cgroups, so the mixins filter `image!=""`.
- `pod`, `namespace`: from CRI labels, or resolved from the pod cgroup.
- `container`: Kubernetes container name. It is empty for the pod cgroup, and `POD` or empty for the sandbox depending on the runtime, so the mixins filter `container!=""`.

The Prometheus scrape usually adds `instance`, `node`, `job="cadvisor"` (kubernetes-mixin `cadvisorSelector`), and `metrics_path="/metrics/cadvisor"`.

Hierarchy: `id="/"` is the node, then `/kubepods.slice` (QoS slices), then the pod cgroup (pod set, container=""), then the container. That gives node-level and pod-level aggregates within cAdvisor itself. Beware double counting when you sum without filtering `container!=""`.

Machine metrics carry the labels `machine_id`, `system_uuid`, `boot_id`.

### B.2 Container metrics

The "Kind" column is the `-disable_metrics` kind. "Kubelet" is Yes if the kubelet exposes the metric by default. Extra labels are on top of the B.1 labels. Names marked (src) are in `lib/metrics/prometheus.go` but missing from docs/storage/prometheus.md.

#### CPU (kind `cpu`, kubelet: Yes)

| Metric | Type | Extra labels | Meaning |
|---|---|---|---|
| container_cpu_usage_seconds_total | C | cpu ("total" on kubelet) | Cumulative CPU time. **Root** |
| container_cpu_user_seconds_total | C | - | User CPU time. |
| container_cpu_system_seconds_total | C | - | System CPU time. |
| container_cpu_cfs_periods_total | C | - | Elapsed CFS enforcement periods. **Root** (CPUThrottlingHigh denominator) |
| container_cpu_cfs_throttled_periods_total | C | - | Throttled periods. **Root** (CPUThrottlingHigh: throttled/periods > 25% for 15m) |
| container_cpu_cfs_throttled_seconds_total | C | - | Total throttled time. |
| container_cpu_cfs_burst_periods_total (src) | C | - | Periods where CFS burst occurred. |
| container_cpu_cfs_burst_seconds_total (src) | C | - | Total burst time. |

#### CPU load / scheduler (NOT on kubelet)

| Metric | Type | Kind | Meaning |
|---|---|---|---|
| container_cpu_load_average_10s | G | cpuLoad | 10s load average of the container. |
| container_cpu_load_d_average_10s (src) | G | cpuLoad | 10s average of uninterruptible (D-state) load. |
| container_tasks_state | G | cpuLoad | Label state (sleeping, running, stopped, uninterruptible, iowaiting). Tasks per state. |
| container_cpu_schedstat_run_seconds_total | C | sched | Time on CPU. |
| container_cpu_schedstat_runqueue_seconds_total | C | sched | Time waiting on the runqueue. |
| container_cpu_schedstat_run_periods_total | C | sched | Times scheduled. |

#### Memory (kind `memory`, kubelet: Yes)

| Metric | Type | Meaning |
|---|---|---|
| container_memory_working_set_bytes | G | Working set (usage minus inactive file). This is what the kubelet evicts on and what the OOM decision tracks. **Root** |
| container_memory_usage_bytes | G | Total usage including page cache. |
| container_memory_rss | G | RSS (anon). **Root candidate** (mixin rule) |
| container_memory_cache | G | Page cache. |
| container_memory_swap | G | Swap usage. |
| container_memory_mapped_file | G | Memory-mapped files. |
| container_memory_kernel_usage (src) | G | Kernel memory. |
| container_memory_max_usage_bytes | G | Peak usage recorded. |
| container_memory_failcnt | C | Times the usage hit the limit (cgroup v1). |
| container_memory_failures_total | C | Labels failure_type (pgfault/pgmajfault) and scope (container/hierarchy). Page fault counts. |
| container_memory_events_high_total (src) | C | memory.high throttle events (cgroup v2). |
| container_memory_events_max_total (src) | C | memory.max limit hits (cgroup v2). **Root candidate** (leading indicator of OOM) |
| container_memory_total_active_file_bytes / container_memory_total_inactive_file_bytes (src) | G | Active/inactive file cache. |
| container_memory_file_dirty_bytes / container_memory_file_writeback_bytes | G | Dirty / writeback file cache (cgroup v2). |
| container_memory_pgscan_total / container_memory_pgsteal_total | C | Reclaim scanned / reclaimed pages (cgroup v2). |
| container_memory_workingset_refault_file_total / container_memory_workingset_refault_anon_total | C | Refaults (thrashing) (cgroup v2). |

#### OOM (kind `oom_event`, kubelet: Yes)

container_oom_events_total (C): OOM events observed for the container. **Root**

#### Pressure / PSI (kind `pressure`, kubelet: only with KubeletPSI, default on from 1.34) (src; not in the docs table)

container_pressure_cpu_waiting_seconds_total, container_pressure_cpu_stalled_seconds_total, container_pressure_memory_waiting_seconds_total, container_pressure_memory_stalled_seconds_total, container_pressure_io_waiting_seconds_total, container_pressure_io_stalled_seconds_total. All are C. "waiting" is PSI some and "stalled" is PSI full, per cgroup. They are strong per-container saturation roots where available.

#### Filesystem usage (kind `disk`, kubelet: Yes). Extra label `device`

container_fs_usage_bytes (G, bytes used on the fs), container_fs_limit_bytes (G, fs capacity usable), container_fs_inodes_free (G), container_fs_inodes_total (G).

On containerd/CRI nodes the kubelet often reports these only for some cgroups, such as the root cgroup, and rootfs usage per container may be missing. Treat per-container fs_usage coverage as runtime-dependent. The kubelet's /metrics/resource and summary API are the alternative.

#### Disk I/O (kind `diskIO`, kubelet: Yes). Extra label `device`

| Metric | Type | Meaning |
|---|---|---|
| container_fs_reads_bytes_total / container_fs_writes_bytes_total | C | Bytes read/written. **Root candidate** (used in mixin dashboards) |
| container_fs_reads_total / container_fs_writes_total | C | IOPS. (mixin dashboards) |
| container_fs_read_seconds_total / container_fs_write_seconds_total | C | Time reading/writing. |
| container_fs_reads_merged_total / container_fs_writes_merged_total | C | Merged ops. |
| container_fs_sector_reads_total / container_fs_sector_writes_total | C | Sectors. |
| container_fs_io_current | G | I/Os in flight. |
| container_fs_io_time_seconds_total / container_fs_io_time_weighted_seconds_total | C | Busy time / weighted. |
| container_fs_io_cost_usage_seconds_total, container_fs_io_cost_wait_seconds_total, container_fs_io_cost_indebt_seconds_total, container_fs_io_cost_indelay_seconds_total (src) | C | cgroup v2 blk-iocost stats. |
| container_blkio_device_usage_total | C | Labels device, major, minor, operation. Blkio bytes per device/op. |

The per-device fs_*_seconds and sector metrics are populated from blkio stats and are often zero under cgroup v2.

#### Network (kind `network`, kubelet: Yes). Extra label `interface`

The values are for the pod network namespace, so they are reported on the pod-level/sandbox cgroup (container="" or "POD"), not per app container. Aggregate by pod.

container_network_receive_bytes_total, container_network_transmit_bytes_total (**Root**), container_network_receive_packets_total, container_network_transmit_packets_total, container_network_receive_packets_dropped_total, container_network_transmit_packets_dropped_total (**Root candidate**), container_network_receive_errors_total, container_network_transmit_errors_total. All are C.

NOT on kubelet: container_network_tcp_usage_total and container_network_tcp6_usage_total (G, tcp_state; kinds tcp), container_network_udp_usage_total and container_network_udp6_usage_total (G, udp_state; udp), and container_network_advance_tcp_stats_total (G, tcp_state; advtcp).

#### Processes (kind `process`, kubelet: Yes; standalone cAdvisor: off by default)

container_processes (G), container_threads (G), container_threads_max (G; 0 = unlimited), container_file_descriptors (G), container_sockets (G), container_ulimits_soft (G, label ulimit).

#### Spec / lifecycle (always emitted, no kind)

| Metric | Type | Meaning |
|---|---|---|
| container_spec_cpu_quota / container_spec_cpu_period | G | CFS quota/period. quota/period = CPU limit in cores. quota is only emitted when a quota is set. |
| container_spec_cpu_shares | G | cpu.shares/weight (CPU request proxy). |
| container_spec_memory_limit_bytes | G | Memory limit (0 = unlimited). |
| container_spec_memory_reservation_limit_bytes | G | Memory soft limit. |
| container_spec_memory_swap_limit_bytes | G | Swap limit. |
| container_start_time_seconds | G | Start time (restart detection). |
| container_creation_time_seconds (src) | G | Creation time. |
| container_last_seen | G | Last time the exporter saw the container. |
| container_health_state | G | Health-check result (Docker health checks). |

#### Other kinds (NOT on kubelet)

- hugetlb: container_hugetlb_usage_bytes, container_hugetlb_max_usage_bytes, container_hugetlb_failcnt (label pagesize).
- memory_numa: container_memory_numa_pages (type, scope, node).
- cpuset: container_memory_migrate.
- referenced_memory: container_referenced_bytes.
- resctrl: container_memory_bandwidth_bytes, container_memory_bandwidth_local_bytes, container_llc_occupancy_bytes.
- perf_event: container_perf_events_total, container_perf_events_scaling_ratio (cpu, event), container_perf_uncore_events_total, container_perf_uncore_events_scaling_ratio (socket, event, pmu).

### B.3 Machine metrics (`lib/metrics/prometheus_machine.go`)

Registered by the kubelet on /metrics/cadvisor. Labels are machine_id, system_uuid, boot_id.

| Metric | Type | Kubelet | Meaning |
|---|---|---|---|
| machine_cpu_cores | G | Yes | Logical CPU cores. |
| machine_cpu_physical_cores | G | Yes | Physical cores. |
| machine_cpu_sockets | G | Yes | Sockets. |
| machine_cpu_books / machine_cpu_drawers (src) | G | Yes | CPU books/drawers (s390x topology). |
| machine_memory_bytes | G | Yes | Installed memory. |
| machine_swap_bytes | G | Yes | Swap available. |
| machine_dimm_count / machine_dimm_capacity_bytes | G | Yes | Label type. From EDAC sysfs. |
| machine_nvm_capacity / machine_nvm_avg_power_budget_watts | G | Yes (needs libipmctl build) | NVM. |
| machine_cpu_cache_capacity_bytes, machine_thread_siblings_count, machine_node_memory_capacity_bytes, machine_node_hugepages_count, machine_node_distance | G | NO (cpu_topology) | NUMA/topology. |

The docs also list `cadvisor_version_info` (labels kernelVersion, osVersion, dockerVersion, cadvisorVersion, cadvisorRevision). It is defined in prometheus.go.

### B.4 kubernetes-mixin usage of cAdvisor metrics (root candidates)

All queries use `job="cadvisor"` and typically `container!=""` and `image!=""`.

Recording rules (`rules/apps.libsonnet`):

- node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate5m and ...:sum_irate come from container_cpu_usage_seconds_total{image!=""} joined to kube_pod_info for `node`.
- node_namespace_pod_container:container_memory_working_set_bytes
- node_namespace_pod_container:container_memory_rss
- node_namespace_pod_container:container_memory_cache
- node_namespace_pod_container:container_memory_swap

Alerts (`alerts/resource_alerts.libsonnet`):

- CPUThrottlingHigh: increase(container_cpu_cfs_throttled_periods_total{container!=""}[5m]) / increase(container_cpu_cfs_periods_total[5m]) > 25% (`cpuThrottlingPercent: 25`), for 15m.
- Other resource alerts (KubeCPUOvercommit, KubeMemoryOvercommit, and similar) rely on kube-state-metrics and the recording rules above.

Dashboards (resources/*, network-usage/*, by number of references): container_network_receive_bytes_total and container_network_transmit_bytes_total (23 each), container_network_receive_packets_total (18), container_cpu_usage_seconds_total (17), container_network_receive_packets_dropped_total (14), container_memory_working_set_bytes (14), container_network_transmit_packets_total and container_network_transmit_packets_dropped_total (13 each), container_fs_reads_total, container_fs_writes_total, container_fs_reads_bytes_total and container_fs_writes_bytes_total (10 each), container_memory_rss (8), container_memory_swap and container_memory_cache (3 each), container_memory_usage_bytes, container_cpu_cfs_throttled_periods_total and container_cpu_cfs_periods_total (1 each). The network dashboards also use node_netstat_Tcp_RetransSegs, node_netstat_Tcp_OutSegs, and node_netstat_TcpExt_TCPSynRetrans.

---------------------------------------------------------------------------------------------------

## PART C. Suggested root vs child split (derived from the mixins above)

The metrics chosen here are the mixin usage above plus cheap saturation signals. Everything else in Parts A and B is a child metric.

Node roots (node_exporter, all default-enabled except systemd):

- CPU: node_cpu_seconds_total, node_load1, node_pressure_cpu_waiting_seconds_total
- Memory: node_memory_MemAvailable_bytes, node_memory_MemTotal_bytes, node_vmstat_pgmajfault, node_vmstat_oom_kill, node_pressure_memory_stalled_seconds_total
- Disk: node_disk_io_time_seconds_total, node_disk_io_time_weighted_seconds_total, node_pressure_io_stalled_seconds_total
- Filesystem: node_filesystem_avail_bytes, node_filesystem_size_bytes, node_filesystem_files_free, node_filesystem_files, node_filesystem_readonly
- Network: node_network_receive_bytes_total, node_network_transmit_bytes_total, node_network_receive_errs_total, node_network_transmit_errs_total, node_network_receive_drop_total, node_network_transmit_drop_total, node_network_up
- Conntrack: node_nf_conntrack_entries, node_nf_conntrack_entries_limit
- File descriptors: node_filefd_allocated, node_filefd_maximum
- Clock: node_timex_offset_seconds, node_timex_sync_status
- Hardware: node_md_disks, node_md_disks_required, node_edac_uncorrectable_errors_total
- Services: node_systemd_unit_state (needs the systemd collector)
- Exporter health: node_scrape_collector_success, node_boot_time_seconds

Container roots (kubelet cAdvisor): container_cpu_usage_seconds_total, container_cpu_cfs_throttled_periods_total, container_cpu_cfs_periods_total, container_memory_working_set_bytes, container_oom_events_total, container_spec_memory_limit_bytes, container_spec_cpu_quota, container_spec_cpu_period, container_network_receive_bytes_total, container_network_transmit_bytes_total, container_network_receive_packets_dropped_total, container_network_transmit_packets_dropped_total, container_fs_reads_bytes_total, container_fs_writes_bytes_total, container_start_time_seconds. On 1.34+ also add container_pressure_{cpu,memory,io}_*_seconds_total.

## Caveats

- Names were read from `master` source on 2026-09-30. Older node_exporter or cAdvisor versions may lack newer metrics such as node_pressure_irq_stalled_seconds_total, node_kernel_hung_tasks_total, container_pressure_*, container_memory_events_*, container_fs_io_cost_*, and container_cpu_cfs_burst_*.
- These families are dynamic, so their exact member set depends on the kernel, hardware, and flags: meminfo, vmstat, netstat, sockstat, hwmon, ethtool driver stats, and zfs. Only field keys visible in source, or selected by the default regexes, are listed.
- The docs/storage/prometheus.md table is incomplete compared with source. Metrics found only in source are marked (src).
- The perf collector was skipped as requested.
