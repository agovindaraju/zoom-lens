package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/prometheus/prometheus/promql/parser"

	v1 "github.com/zoomlens/zoomlens/api/v1alpha1"
)

var (
	metricNameRE = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	programRE    = regexp.MustCompile(`^[a-z0-9_-]+$`)
	histSuffixRE = regexp.MustCompile(`_(bucket|sum|count)$`)
	upperSplitRE = regexp.MustCompile(`([a-z0-9])([A-Z])|([A-Z])([A-Z][a-z])`)
	sourceNameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
)

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

// HostmetricsScrapers are the scrapers of the OpenTelemetry hostmetrics receiver.
var HostmetricsScrapers = map[string]string{
	"cpu": "CPU utilisation metrics", "disk": "Disk I/O metrics", "load": "CPU load metrics",
	"filesystem": "File System utilisation metrics", "memory": "Memory utilisation metrics",
	"network": "Network interface I/O metrics & TCP connection metrics", "paging": "Paging/Swap space utilisation and I/O metrics",
	"processes": "Process count metrics", "process": "Per process CPU, Memory, and Disk I/O metrics", "system": "Miscellaneous system metrics",
}

// RequiredRuleLabels are the labels the operator reads from every alert.
var RequiredRuleLabels = []string{"severity", "zoomlens_source", "zoomlens_root", "zoomlens_entity"}

func kebab(s string) string {
	return strings.ToLower(upperSplitRE.ReplaceAllString(upperSplitRE.ReplaceAllString(s, "$1$3-$2$4"), "$1$3-$2$4"))
}

type report struct{ errors, warnings, infos []string }

func (r *report) errf(f string, a ...any)  { r.errors = append(r.errors, fmt.Sprintf(f, a...)) }
func (r *report) warnf(f string, a ...any) { r.warnings = append(r.warnings, fmt.Sprintf(f, a...)) }

// Validate checks every invariant of the knowledge and prints a report.
func Validate(k *Knowledge, verbose bool) int {
	r := &report{}
	ix := BuildIndex(k)
	domains := map[v1.Domain]bool{}
	for _, d := range v1.Domains {
		domains[d] = true
	}
	checkDomain := func(where string, d v1.Domain) {
		if !domains[d] {
			r.errf("%s: unknown domain %q", where, d)
		}
	}

	// --- entity graph -----------------------------------------------------
	if len(k.Graphs) != 1 {
		r.errf("expected exactly one EntityGraph, found %d", len(k.Graphs))
	}
	usedEntities := map[string]bool{}
	for _, g := range k.Graphs {
		for _, e := range g.Spec.Edges {
			where := fmt.Sprintf("EntityGraph %s -%s-> %s", e.From, e.Relation, e.To)
			for _, side := range []string{e.From, e.To} {
				usedEntities[side] = true
				if _, ok := ix.Entities[side]; !ok {
					r.errf("%s: entity %q is not defined in spec.entities", where, side)
				}
			}
			for _, d := range e.Domains {
				checkDomain(where, d)
			}
			if len(e.Domains) == 0 {
				r.errf("%s: needs at least one domain", where)
			}
			if !oneOf(string(e.Discovery), "informer", "rule", "annotation", "metrics") {
				r.errf("%s: discovery %q must be informer, rule, annotation or metrics", where, e.Discovery)
			}
		}
	}

	// --- packs --------------------------------------------------------------
	type stat struct {
		name                string
		roots, total, depth int
	}
	var stats []stat
	proseOnly := map[string][]string{}
	bySource := map[string]map[string]string{} // source -> name -> pack
	ebpfText := readSource(k, "addons-etcd-coredns-ebpf-cni-csi.md")
	for i := range k.Packs {
		p := &k.Packs[i]
		s := p.Spec
		where := "MetricTreePack " + p.Name
		usedEntities[s.Entity] = true
		if want := s.Source.Name + "-" + kebab(s.Entity); p.Name != want {
			r.errf("%s: name must be %q (<source>-<entity>)", where, want)
		}
		if p.Labels["zoomlens.dev/source"] != s.Source.Name {
			r.errf("%s: label zoomlens.dev/source must be %q", where, s.Source.Name)
		}
		if _, ok := ix.Entities[s.Entity]; !ok {
			r.errf("%s: entity %q is not defined in the EntityGraph", where, s.Entity)
		}
		// Same constraints as the CRD schema (deploy/crds), checked offline.
		if !sourceNameRE.MatchString(s.Source.Name) {
			r.errf("%s: source name %q must match %s", where, s.Source.Name, sourceNameRE)
		}
		if !oneOf(string(s.Source.Scope), "per-node", "cluster", "control-plane") {
			r.errf("%s: scope %q must be per-node, cluster or control-plane", where, s.Source.Scope)
		}
		if !oneOf(string(s.Source.NameKind), "", "metric", "scraper", "ebpf-program") {
			r.errf("%s: nameKind %q must be metric, scraper or ebpf-program", where, s.Source.NameKind)
		}
		nameKind := s.Source.NameKind
		if nameKind == "" {
			nameKind = v1.NameKindMetric
		}
		if bySource[s.Source.Name] == nil {
			bySource[s.Source.Name] = map[string]string{}
		}
		for _, m := range Members(p) {
			switch nameKind {
			case v1.NameKindMetric:
				if !metricNameRE.MatchString(m.Name) {
					r.errf("%s: invalid metric name %q", where, m.Name)
				}
				if _, e, ok := k.Lookup(s.Source.Name, m.Name); !ok {
					r.errf("%s: %q is not in any sources/*.md catalogue", where, m.Name)
				} else if e.Evidence != "table" {
					proseOnly[s.Source.Name] = append(proseOnly[s.Source.Name], m.Name)
				}
			case v1.NameKindScraper:
				if _, ok := HostmetricsScrapers[m.Name]; !ok {
					r.errf("%s: unknown hostmetrics scraper %q", where, m.Name)
				}
			}
			if prev, dup := bySource[s.Source.Name][m.Name]; dup {
				r.errf("%s: %q appears twice in source %s (also in %s); incident jobs would duplicate series", where, m.Name, s.Source.Name, prev)
			}
			bySource[s.Source.Name][m.Name] = p.Name
		}
		for _, rt := range s.Roots {
			checkDomain(where+" root "+rt.Name, rt.Domain)
		}
		depth := Depths(p)
		for parent := range s.Children {
			if _, ok := depth[parent]; !ok {
				r.errf("%s: children key %q is not reachable from a root", where, parent)
			}
		}
		roots := map[string]bool{}
		for _, rt := range s.Roots {
			roots[rt.Name] = true
		}
		maxD := 0
		for n, d := range depth {
			if d > 3 {
				r.errf("%s: %q is at depth %d; the dwell ladder stops at depth 3", where, n, d)
			}
			if d > maxD {
				maxD = d
			}
		}
		for parent, kids := range s.Children {
			for _, c := range kids {
				if roots[c] {
					r.errf("%s: root %q listed as a child of %q", where, c, parent)
				}
			}
		}
		if cyc := findCycle(p); cyc != "" {
			r.errf("%s: cycle %s", where, cyc)
		}
		if nameKind == v1.NameKindEBPFProgram {
			if len(s.Roots) > 0 {
				r.errf("%s: eBPF programs must never be roots", where)
			}
			for _, pg := range s.EBPFPrograms {
				if !programRE.MatchString(pg.Name) {
					r.errf("%s: invalid program name %q", where, pg.Name)
				}
				if !regexp.MustCompile(`\b` + regexp.QuoteMeta(pg.Name) + `\b`).MatchString(ebpfText) {
					r.errf("%s: program %q is not in the ebpf_exporter example list", where, pg.Name)
				}
				checkDomain(where+" program "+pg.Name, pg.Domain)
				for _, m := range pg.Metrics {
					if _, _, ok := k.Lookup(s.Source.Name, m); !ok {
						r.errf("%s: program %q metric %q is not in sources", where, pg.Name, m)
					}
				}
			}
		} else if len(s.EBPFPrograms) > 0 {
			r.errf("%s: ebpfPrograms is only allowed with nameKind ebpf-program", where)
		}
		stats = append(stats, stat{p.Name, len(s.Roots), len(Members(p)), maxD})
	}
	for name := range ix.Entities {
		if !usedEntities[name] {
			r.warnf("EntityGraph: entity %q is not used by any pack or edge", name)
		}
	}

	// --- relations ----------------------------------------------------------
	resolve := func(where string, ref v1.MetricRef, want ...Role) {
		if _, ok := ix.PackByRef[packRef(ref.Source, ref.Entity)]; !ok {
			r.errf("%s: no MetricTreePack for source %q entity %q", where, ref.Source, ref.Entity)
			return
		}
		m, ok := ix.Member[packRef(ref.Source, ref.Entity)+"/"+ref.Metric]
		if !ok {
			r.errf("%s: %s/%s has no metric %q", where, ref.Source, ref.Entity, ref.Metric)
			return
		}
		for _, w := range want {
			if m.Role == w {
				return
			}
		}
		r.errf("%s: %s/%s/%s is a %s, expected %v", where, ref.Source, ref.Entity, ref.Metric, m.Role, want)
	}
	nSib, nExp, nEbpf := 0, 0, 0
	for _, rel := range k.Relations {
		for _, g := range rel.Spec.Siblings {
			nSib++
			where := "MetricRelations sibling " + g.Name
			checkDomain(where, g.Domain)
			packs := map[string]bool{}
			for _, m := range g.Members {
				resolve(where, m, RoleRoot)
				packs[packRef(m.Source, m.Entity)] = true
			}
			if len(packs) < 2 {
				r.errf("%s: members must come from at least two packs", where)
			}
		}
		for _, e := range rel.Spec.Explains {
			nExp++
			where := fmt.Sprintf("MetricRelations explains %s/%s", e.Root.Source, e.Root.Metric)
			resolve(where, e.Root, RoleRoot)
			for _, t := range e.Targets {
				resolve(where, t.MetricRef, RoleChild)
				if t.Depth < 1 || t.Depth > 3 {
					r.errf("%s: depth %d outside 1..3", where, t.Depth)
				}
			}
		}
		for _, e := range rel.Spec.EBPF {
			nEbpf++
			where := fmt.Sprintf("MetricRelations ebpf %s/%s", e.Root.Source, e.Root.Metric)
			resolve(where, e.Root, RoleRoot)
			for _, pg := range e.Programs {
				if _, ok := ix.Programs[pg.Name]; !ok {
					r.errf("%s: unknown eBPF program %q", where, pg.Name)
				}
				if pg.Depth < 2 || pg.Depth > 3 {
					r.errf("%s: program %q depth %d outside 2..3", where, pg.Name, pg.Depth)
				}
			}
		}
	}

	// --- rules ----------------------------------------------------------------
	alertedRoots := map[string]bool{}
	seenAlert := map[string]string{}
	nRules := 0
	var ruleFiles []string
	for f := range k.RuleFiles {
		ruleFiles = append(ruleFiles, f)
	}
	sort.Strings(ruleFiles)
	for _, f := range ruleFiles {
		for _, e := range k.RuleErrors[f] {
			r.errf("%s: %v", f, e)
		}
		rg := k.RuleFiles[f]
		if rg == nil {
			continue
		}
		for _, g := range rg.Groups {
			for _, rule := range g.Rules {
				if rule.Alert == "" {
					continue
				}
				nRules++
				where := fmt.Sprintf("%s: alert %s", filepath.Base(f), rule.Alert)
				if prev, dup := seenAlert[rule.Alert]; dup {
					r.errf("%s: duplicate alert name (also in %s)", where, prev)
				}
				seenAlert[rule.Alert] = filepath.Base(f)
				for _, l := range RequiredRuleLabels {
					if rule.Labels[l] == "" {
						r.errf("%s: missing label %s", where, l)
					}
				}
				src, root := rule.Labels["zoomlens_source"], rule.Labels["zoomlens_root"]
				var pack *v1.MetricTreePack
				for i := range k.Packs {
					p := &k.Packs[i]
					if p.Spec.Source.Name != src {
						continue
					}
					for _, rt := range p.Spec.Roots {
						if rt.Name == root {
							pack = p
						}
					}
				}
				if pack == nil {
					r.errf("%s: zoomlens_root %q is not a root of any %s pack", where, root, src)
				} else {
					alertedRoots[packRef(src, pack.Spec.Entity)+"/"+root] = true
					if want := ix.Entities[pack.Spec.Entity].OperatorKindOf(); rule.Labels["zoomlens_entity"] != want {
						r.errf("%s: zoomlens_entity must be %q for entity %s", where, want, pack.Spec.Entity)
					}
				}
				expr, err := k.Parser.ParseExpr(rule.Expr)
				if err != nil {
					continue // already reported by rulefmt
				}
				if expr.Type() != parser.ValueTypeVector {
					r.errf("%s: expression returns %s, alerts need an instant vector", where, expr.Type())
				}
				for _, name := range selectorNames(expr) {
					base := histSuffixRE.ReplaceAllString(name, "")
					switch {
					case ix.L0[name] || ix.L0[base]:
					case ix.All[name] || ix.All[base]:
						r.errf("%s: uses %q, which is only collected during incidents; make it a root or companion", where, name)
					default:
						r.errf("%s: uses %q, which is in no MetricTreePack", where, name)
					}
				}
			}
		}
	}
	var silent []string
	for i := range k.Packs {
		p := &k.Packs[i]
		for _, rt := range p.Spec.Roots {
			if !alertedRoots[packRef(p.Spec.Source.Name, p.Spec.Entity)+"/"+rt.Name] {
				silent = append(silent, packRef(p.Spec.Source.Name, p.Spec.Entity)+"/"+rt.Name)
			}
		}
	}

	// --- report ---------------------------------------------------------------
	sort.Slice(stats, func(i, j int) bool { return stats[i].name < stats[j].name })
	fmt.Printf("%-48s %5s %6s %5s\n", "MetricTreePack", "roots", "names", "depth")
	tr, tn := 0, 0
	for _, s := range stats {
		fmt.Printf("%-48s %5d %6d %5d\n", s.name, s.roots, s.total, s.depth)
		tr += s.roots
		tn += s.total
	}
	fmt.Printf("\npacks=%d names=%d roots=%d eBPFprograms=%d entities=%d edges=%d siblings=%d explains=%d ebpfLinks=%d alertRules=%d\n",
		len(k.Packs), tn, tr, len(ix.Programs), len(ix.Entities), countEdges(k), nSib, nExp, nEbpf, nRules)
	if len(silent) > 0 {
		fmt.Printf("\nroots without an alert rule (collected; used when an OUT hop reaches the entity): %d\n", len(silent))
		for _, s := range silent {
			fmt.Println("  -", s)
		}
	}
	if len(proseOnly) > 0 {
		fmt.Println("\nnames verified from prose or bullet lists in sources/ rather than a metric table (MKG_VERBOSE=1 lists them):")
		var keys []string
		for s := range proseOnly {
			keys = append(keys, s)
		}
		sort.Strings(keys)
		for _, s := range keys {
			line := fmt.Sprintf("  %s: %d", s, len(proseOnly[s]))
			if verbose {
				line += ": " + strings.Join(proseOnly[s], ", ")
			}
			fmt.Println(line)
		}
	}
	for _, w := range r.warnings {
		fmt.Println("WARN ", w)
	}
	for _, e := range r.errors {
		fmt.Println("ERROR", e)
	}
	fmt.Printf("\n%d error(s), %d warning(s)\n", len(r.errors), len(r.warnings))
	if len(r.errors) > 0 {
		return 1
	}
	return 0
}

func selectorNames(e parser.Expr) []string {
	var out []string
	parser.Inspect(e, func(n parser.Node, _ []parser.Node) error {
		if vs, ok := n.(*parser.VectorSelector); ok && vs.Name != "" {
			out = append(out, vs.Name)
		}
		return nil
	})
	return out
}

func findCycle(p *v1.MetricTreePack) string {
	state := map[string]int{}
	var found string
	var dfs func(n string, path []string)
	dfs = func(n string, path []string) {
		state[n] = 1
		for _, c := range p.Spec.Children[n] {
			if found != "" {
				return
			}
			if state[c] == 1 {
				found = strings.Join(append(path, c), " -> ")
				return
			}
			if state[c] == 0 {
				dfs(c, append(path, c))
			}
		}
		state[n] = 2
	}
	for _, r := range p.Spec.Roots {
		if state[r.Name] == 0 {
			dfs(r.Name, []string{r.Name})
		}
	}
	return found
}

func countEdges(k *Knowledge) int {
	n := 0
	for _, g := range k.Graphs {
		n += len(g.Spec.Edges)
	}
	return n
}

func readSource(k *Knowledge, name string) string {
	b, _ := os.ReadFile(filepath.Join(k.Dir, "sources", name))
	return string(b)
}
