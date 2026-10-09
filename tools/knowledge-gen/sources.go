package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SourceEntry is what the upstream-derived catalogues in sources/*.md say
// about one metric.
type SourceEntry struct {
	Type      string `json:"type,omitempty"`
	Stability string `json:"stability,omitempty"`
	Labels    string `json:"labels,omitempty"`
	Help      string `json:"help,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	Collector string `json:"collector,omitempty"`
	Section   string `json:"section,omitempty"`
	// Evidence is "table" (a row of a metric table), "prose" (a list in the
	// text) or "mentioned" (named in the text).
	Evidence string `json:"evidence"`
}

var (
	tokenRE    = regexp.MustCompile(`[a-zA-Z_:][a-zA-Z0-9_:]*`)
	tickRE     = regexp.MustCompile("`([^`]+)`")
	sepRE      = regexp.MustCompile(`^\|\s*:?-{2,}`)
	markerRE   = regexp.MustCompile(`\*\*R\*\*|\*\*C\*\*|\(src\)|\(derived\)`)
	meminfoRE  = regexp.MustCompile("Field keys found in `meminfo_linux.go`:\\s*\\n\\s*\\n?(.+?)\\n")
	fieldRE    = regexp.MustCompile(`([A-Za-z0-9_()]+)(?:,|\s\()`)
	prefixRE   = regexp.MustCompile("`([a-z_]+)_<name>`:\\s*(.+)")
	suffixRE   = regexp.MustCompile(`([a-z0-9_]+_total)`)
	mentionRE  = regexp.MustCompile(`\b([a-z][a-z0-9]*(?:_[A-Za-z0-9]+)+)\b`)
	metricHead = map[string]bool{"metric": true, "metric (derived)": true}
)

// ParseSources reads every sources/*.md catalogue.
func ParseSources(dir string) (map[string]map[string]SourceEntry, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	sort.Strings(files)
	out := map[string]map[string]SourceEntry{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		text := strings.ReplaceAll(string(b), "\r\n", "\n")
		out[filepath.Base(f)] = parseSource(text)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no catalogues in %s", dir)
	}
	return out, nil
}

func parseSource(text string) map[string]SourceEntry {
	entries := map[string]SourceEntry{}
	lines := strings.Split(text, "\n")
	section := ""
	var header []string
	for i, line := range lines {
		if strings.HasPrefix(line, "#") {
			section = strings.TrimSpace(strings.TrimLeft(line, "#"))
			header = nil
			continue
		}
		if !strings.HasPrefix(line, "|") {
			header = nil
			continue
		}
		cells := splitRow(line)
		if header == nil {
			if i+1 < len(lines) && sepRE.MatchString(lines[i+1]) {
				header = make([]string, len(cells))
				for j, c := range cells {
					header[j] = strings.ToLower(c)
				}
			}
			continue
		}
		if len(cells) > 0 && strings.HasPrefix(strings.TrimLeft(cells[0], ":"), "--") {
			continue
		}
		col := -1
		for j, h := range header {
			if metricHead[h] {
				col = j
				break
			}
		}
		if col < 0 || col >= len(cells) {
			continue
		}
		row := map[string]string{}
		for j, h := range header {
			if j < len(cells) {
				row[h] = cells[j]
			}
		}
		for _, n := range namesInCell(cells[col]) {
			if _, ok := entries[n]; ok {
				continue
			}
			entries[n] = SourceEntry{
				Type:      row["type"],
				Stability: first(row["stability"], row["status"]),
				Labels:    first(row["key labels"], row["labels"], row["extra labels"]),
				Help:      first(row["meaning"], row["help"]),
				Endpoint:  row["endpoint"],
				Collector: first(row["collector / gate"], row["collector"], row["kind"], row["example"]),
				Section:   section,
				Evidence:  "table",
			}
		}
	}
	if m := meminfoRE.FindStringSubmatch(text); m != nil {
		for _, f := range fieldRE.FindAllStringSubmatch(m[1], -1) {
			n := "node_memory_" + f[1] + "_bytes"
			if _, ok := entries[n]; !ok {
				entries[n] = SourceEntry{Type: "G", Help: "/proc/meminfo " + f[1], Collector: "meminfo", Section: "meminfo fields", Evidence: "prose"}
			}
		}
	}
	for _, m := range prefixRE.FindAllStringSubmatch(text, -1) {
		for _, s := range suffixRE.FindAllStringSubmatch(m[2], -1) {
			n := m[1] + "_" + s[1]
			if _, ok := entries[n]; !ok {
				entries[n] = SourceEntry{Type: "C", Collector: m[1], Section: "prefix list", Evidence: "prose"}
			}
		}
	}
	for _, m := range mentionRE.FindAllStringSubmatch(text, -1) {
		if _, ok := entries[m[1]]; !ok {
			entries[m[1]] = SourceEntry{Evidence: "mentioned"}
		}
	}
	return entries
}

func splitRow(line string) []string {
	line = strings.ReplaceAll(line, `\|`, "│")
	line = strings.Trim(strings.TrimSpace(line), "|")
	parts := strings.Split(line, "|")
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(strings.TrimSpace(p), "│", "|")
	}
	return parts
}

func namesInCell(cell string) []string {
	raw := markerRE.ReplaceAllString(cell, " ")
	var ticks []string
	for _, m := range tickRE.FindAllStringSubmatch(raw, -1) {
		ticks = append(ticks, m[1])
	}
	if len(ticks) == 0 {
		ticks = []string{raw}
	}
	var names []string
	prev := ""
	for _, t := range ticks {
		t = strings.TrimSpace(t)
		// "`containerd_cri_container_create_seconds`, `..._container_start_seconds`"
		if strings.HasPrefix(t, "..._") && prev != "" {
			tail := t[4:]
			if cut := strings.Index(prev, "_"+strings.SplitN(tail, "_", 2)[0]+"_"); cut >= 0 {
				t = prev[:cut+1] + tail
			}
		}
		for _, m := range tokenRE.FindAllString(t, -1) {
			if strings.Contains(m, "_") && !strings.HasPrefix(m, "_") {
				names = append(names, m)
				prev = m
			}
		}
	}
	return names
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// sourceFiles lists which catalogue documents each source (first wins).
var sourceFiles = map[string][]string{
	"node-exporter":           {"node-exporter-cadvisor.md"},
	"cadvisor":                {"node-exporter-cadvisor.md"},
	"kubelet":                 {"k8s-components.md", "kube-state-metrics.md"},
	"kube-state-metrics":      {"kube-state-metrics.md"},
	"kube-apiserver":          {"k8s-components.md"},
	"kube-scheduler":          {"k8s-components.md"},
	"kube-controller-manager": {"k8s-components.md"},
	"kube-proxy":              {"k8s-components.md"},
	"etcd":                    {"addons-etcd-coredns-ebpf-cni-csi.md", "k8s-components.md"},
	"coredns":                 {"addons-etcd-coredns-ebpf-cni-csi.md", "k8s-components.md"},
	"ebpf":                    {"addons-etcd-coredns-ebpf-cni-csi.md"},
	"containerd":              {"addons-etcd-coredns-ebpf-cni-csi.md"},
	"csi":                     {"addons-etcd-coredns-ebpf-cni-csi.md"},
	"cilium":                  {"addons-etcd-coredns-ebpf-cni-csi.md"},
	"calico":                  {"addons-etcd-coredns-ebpf-cni-csi.md"},
}

// Lookup finds a metric in the catalogues, preferring the source's own file
// and table rows over prose.
func (k *Knowledge) Lookup(source, name string) (string, SourceEntry, bool) {
	files := append([]string{}, sourceFiles[source]...)
	var rest []string
	for f := range k.Sources {
		if !contains(files, f) {
			rest = append(rest, f)
		}
	}
	sort.Strings(rest)
	files = append(files, rest...)
	bestFile, best, found := "", SourceEntry{}, false
	for _, f := range files {
		e, ok := k.Sources[f][name]
		if !ok {
			continue
		}
		if !found || (best.Evidence != "table" && e.Evidence == "table") {
			bestFile, best, found = f, e, true
		}
		if e.Evidence == "table" {
			break
		}
	}
	return bestFile, best, found
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
