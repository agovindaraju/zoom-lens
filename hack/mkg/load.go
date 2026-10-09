package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
	"sigs.k8s.io/yaml"

	v1 "github.com/zoomlens/zoomlens/api/v1alpha1"
)

// Knowledge is everything under the knowledge directory.
type Knowledge struct {
	Dir        string
	Packs      []v1.MetricTreePack
	PackFile   map[string]string // pack name -> file
	Relations  []v1.MetricRelations
	Graphs     []v1.EntityGraph
	RuleFiles  map[string]*rulefmt.RuleGroups
	RuleErrors map[string][]error
	Sources    map[string]map[string]SourceEntry // file -> metric -> entry
	Parser     parser.Parser
}

var docSep = regexp.MustCompile(`(?m)^---\s*$`)

// Load reads and strictly decodes every object. Unknown fields are errors.
func Load(dir string) (*Knowledge, error) {
	k := &Knowledge{Dir: dir, PackFile: map[string]string{}, RuleFiles: map[string]*rulefmt.RuleGroups{},
		RuleErrors: map[string][]error{}, Parser: parser.NewParser(parser.Options{})}
	files, _ := filepath.Glob(filepath.Join(dir, "packs", "*.yaml"))
	files = append(files, filepath.Join(dir, "relations.yaml"), filepath.Join(dir, "entity-graph.yaml"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for i, doc := range docSep.Split(string(b), -1) {
			if strings.TrimSpace(stripComments(doc)) == "" {
				continue
			}
			var tm struct {
				Kind string `json:"kind"`
			}
			if err := yaml.Unmarshal([]byte(doc), &tm); err != nil {
				return nil, fmt.Errorf("%s doc %d: %w", f, i, err)
			}
			switch tm.Kind {
			case "MetricTreePack":
				var p v1.MetricTreePack
				if err := yaml.UnmarshalStrict([]byte(doc), &p); err != nil {
					return nil, fmt.Errorf("%s doc %d: %w", f, i, err)
				}
				k.Packs = append(k.Packs, p)
				k.PackFile[p.Name] = f
			case "MetricRelations":
				var r v1.MetricRelations
				if err := yaml.UnmarshalStrict([]byte(doc), &r); err != nil {
					return nil, fmt.Errorf("%s doc %d: %w", f, i, err)
				}
				k.Relations = append(k.Relations, r)
			case "EntityGraph":
				var g v1.EntityGraph
				if err := yaml.UnmarshalStrict([]byte(doc), &g); err != nil {
					return nil, fmt.Errorf("%s doc %d: %w", f, i, err)
				}
				k.Graphs = append(k.Graphs, g)
			default:
				return nil, fmt.Errorf("%s doc %d: unexpected kind %q", f, i, tm.Kind)
			}
		}
	}
	rules, _ := filepath.Glob(filepath.Join(dir, "rules", "*.rules.yml"))
	sort.Strings(rules)
	for _, f := range rules {
		rg, errs := rulefmt.ParseFile(f, false, model.UTF8Validation, k.Parser, slog.New(slog.DiscardHandler))
		k.RuleFiles[f] = rg
		k.RuleErrors[f] = errs
	}
	var err error
	k.Sources, err = ParseSources(filepath.Join(dir, "sources"))
	return k, err
}

func stripComments(s string) string {
	var out bytes.Buffer
	for _, l := range strings.Split(s, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			out.WriteString(l + "\n")
		}
	}
	return out.String()
}

// Role of a name inside its pack.
type Role string

const (
	RoleRoot      Role = "root"
	RoleCompanion Role = "companion"
	RoleChild     Role = "child"
)

// Member is one name in one pack.
type Member struct {
	Pack   *v1.MetricTreePack
	Name   string
	Role   Role
	Parent string
	Depth  int
	Domain v1.Domain // of the root it belongs to
}

// Members lists every name of a pack in document order.
func Members(p *v1.MetricTreePack) []Member {
	var out []Member
	depth := Depths(p)
	rootOf := RootOf(p)
	dom := map[string]v1.Domain{}
	for _, r := range p.Spec.Roots {
		dom[r.Name] = r.Domain
	}
	for _, r := range p.Spec.Roots {
		out = append(out, Member{Pack: p, Name: r.Name, Role: RoleRoot, Domain: r.Domain})
		for _, c := range r.Companions {
			out = append(out, Member{Pack: p, Name: c, Role: RoleCompanion, Parent: r.Name, Domain: r.Domain})
		}
	}
	for _, parent := range orderedParents(p) {
		for _, c := range p.Spec.Children[parent] {
			out = append(out, Member{Pack: p, Name: c, Role: RoleChild, Parent: parent, Depth: depth[c], Domain: dom[rootOf[c]]})
		}
	}
	return out
}

// orderedParents walks the tree breadth-first from the roots so output is stable.
func orderedParents(p *v1.MetricTreePack) []string {
	seen := map[string]bool{}
	var order []string
	queue := []string{}
	for _, r := range p.Spec.Roots {
		queue = append(queue, r.Name)
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if seen[n] {
			continue
		}
		seen[n] = true
		if _, ok := p.Spec.Children[n]; ok {
			order = append(order, n)
			queue = append(queue, p.Spec.Children[n]...)
		}
	}
	var rest []string
	for parent := range p.Spec.Children {
		if !seen[parent] {
			rest = append(rest, parent)
		}
	}
	sort.Strings(rest)
	return append(order, rest...)
}

// Depths maps every name reachable from a root to its depth (roots = 0).
func Depths(p *v1.MetricTreePack) map[string]int {
	d := map[string]int{}
	var frontier []string
	for _, r := range p.Spec.Roots {
		d[r.Name] = 0
		frontier = append(frontier, r.Name)
	}
	for len(frontier) > 0 {
		var next []string
		for _, n := range frontier {
			for _, c := range p.Spec.Children[n] {
				if _, ok := d[c]; !ok {
					d[c] = d[n] + 1
					next = append(next, c)
				}
			}
		}
		frontier = next
	}
	return d
}

// RootOf maps every child to its root.
func RootOf(p *v1.MetricTreePack) map[string]string {
	out := map[string]string{}
	for _, r := range p.Spec.Roots {
		stack := []string{r.Name}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, c := range p.Spec.Children[n] {
				if _, ok := out[c]; !ok {
					out[c] = r.Name
					stack = append(stack, c)
				}
			}
		}
	}
	return out
}

// Index gives fast lookups across packs.
type Index struct {
	PackByRef map[string]*v1.MetricTreePack // "source/Entity" -> pack
	Member    map[string]Member             // "source/Entity/name" -> member
	L0        map[string]bool               // names collected at L0 anywhere
	All       map[string]bool               // every metric name in any pack
	Entities  map[string]v1.EntityClass
	Programs  map[string]v1.EBPFProgram
}

func packRef(source, entity string) string { return source + "/" + entity }

// BuildIndex indexes the loaded knowledge.
func BuildIndex(k *Knowledge) *Index {
	ix := &Index{PackByRef: map[string]*v1.MetricTreePack{}, Member: map[string]Member{}, L0: map[string]bool{},
		All: map[string]bool{}, Entities: map[string]v1.EntityClass{}, Programs: map[string]v1.EBPFProgram{}}
	for i := range k.Packs {
		p := &k.Packs[i]
		ix.PackByRef[packRef(p.Spec.Source.Name, p.Spec.Entity)] = p
		for _, m := range Members(p) {
			key := packRef(p.Spec.Source.Name, p.Spec.Entity) + "/" + m.Name
			if _, dup := ix.Member[key]; !dup {
				ix.Member[key] = m
			}
			if p.Spec.Source.NameKind == "" || p.Spec.Source.NameKind == v1.NameKindMetric {
				ix.All[m.Name] = true
				if m.Role != RoleChild {
					ix.L0[m.Name] = true
				}
			}
		}
		for _, pg := range p.Spec.EBPFPrograms {
			ix.Programs[pg.Name] = pg
		}
	}
	for _, g := range k.Graphs {
		for _, e := range g.Spec.Entities {
			ix.Entities[e.Name] = e
		}
	}
	return ix
}
