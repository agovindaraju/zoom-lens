// Command mkg validates the Zoom-Lens knowledge (MetricTreePacks, MetricRelations,
// EntityGraph and Prometheus rule files) and generates derived artifacts.
//
//	go run ./hack/mkg validate   [dir]   # exit 1 on errors
//	go run ./hack/mkg catalog    [dir]   # sources/*.md -> generated/catalog/<source>.json
//	go run ./hack/mkg tree       [dir]   # -> generated/TREE.md
//	go run ./hack/mkg prometheusrule [dir] # rules/*.rules.yml -> generated/prometheusrule.yaml
//	go run ./hack/mkg all        [dir]
//
// dir defaults to ./knowledge. Run from the repository root.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	dir := "knowledge"
	if len(os.Args) > 2 {
		dir = os.Args[2]
	}
	verbose := os.Getenv("MKG_VERBOSE") != ""
	k, err := Load(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
	switch os.Args[1] {
	case "validate":
		os.Exit(Validate(k, verbose))
	case "catalog":
		must(WriteCatalog(k))
	case "tree":
		must(WriteTree(k))
	case "prometheusrule":
		must(WritePrometheusRule(k))
	case "all":
		must(WriteCatalog(k))
		rc := Validate(k, verbose)
		must(WriteTree(k))
		must(WritePrometheusRule(k))
		os.Exit(rc)
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: mkg validate|catalog|tree|prometheusrule|all [knowledge-dir]")
	os.Exit(2)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
