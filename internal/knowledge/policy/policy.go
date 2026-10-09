// Package policy implements knowledge.EscalationPolicy: the dwell ladder.
//
// The ladder is operator configuration, not knowledge about metrics, so it lives in a small YAML
// file (a ConfigMap in the cluster) next to the operator:
//
//	clearFor: 5m
//	levels:
//	  - {after: 0s,  depth: 1, hops: 1, scrapeInterval: 10s}
//	  - {after: 5m,  depth: 2, hops: 1, scrapeInterval: 5s, enableEBPF: true}
//	  - {after: 10m, depth: 3, hops: 2, scrapeInterval: 5s, enableEBPF: true}
//
// The metric trees and the dependency graph come from the zoomlens.dev resources (package crd).
package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/zoomlens/zoomlens/internal/knowledge"
)

// DefaultClearFor is used when the file does not set clearFor.
const DefaultClearFor = 5 * time.Minute

// File is the on-disk format of the ladder file.
type File struct {
	// ClearFor is how long an alert must stay resolved before collection reverts to the baseline.
	ClearFor Duration    `json:"clearFor"`
	Levels   []LevelSpec `json:"levels"`
}

// LevelSpec is one ladder rung, reached once the alert has fired for After.
type LevelSpec struct {
	After          Duration `json:"after"`
	Depth          int      `json:"depth"`
	Hops           int      `json:"hops"`
	ScrapeInterval Duration `json:"scrapeInterval"`
	EnableEBPF     bool     `json:"enableEBPF"`
}

// Duration is a time.Duration that unmarshals from strings like "5m".
type Duration struct{ time.Duration }

// UnmarshalJSON implements json.Unmarshaler.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a string like \"5m\": %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// MarshalJSON implements json.Marshaler.
func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// Policy is a knowledge.EscalationPolicy backed by a static ladder.
type Policy struct {
	levels   []LevelSpec
	clearFor time.Duration
}

var _ knowledge.EscalationPolicy = (*Policy)(nil)

// Load reads and validates a ladder file.
func Load(path string) (*Policy, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ladder file: %w", err)
	}
	return Parse(raw)
}

// Parse parses and validates ladder YAML.
func Parse(raw []byte) (*Policy, error) {
	var f File
	if err := yaml.UnmarshalStrict(raw, &f); err != nil {
		return nil, fmt.Errorf("parse ladder file: %w", err)
	}
	if len(f.Levels) == 0 {
		return nil, fmt.Errorf("ladder: levels must not be empty")
	}
	for i, l := range f.Levels {
		if l.Depth < 0 || l.Hops < 0 {
			return nil, fmt.Errorf("ladder: levels[%d]: depth and hops must be >= 0", i)
		}
		if l.ScrapeInterval.Duration < time.Second {
			return nil, fmt.Errorf("ladder: levels[%d]: scrapeInterval must be >= 1s", i)
		}
	}
	sort.SliceStable(f.Levels, func(i, j int) bool { return f.Levels[i].After.Duration < f.Levels[j].After.Duration })
	cf := f.ClearFor.Duration
	if cf <= 0 {
		cf = DefaultClearFor
	}
	return &Policy{levels: f.Levels, clearFor: cf}, nil
}

// LevelFor returns the highest level whose After has elapsed.
func (p *Policy) LevelFor(dwell time.Duration) knowledge.Level {
	var out knowledge.Level
	for i, l := range p.levels {
		if dwell < l.After.Duration {
			break
		}
		out = knowledge.Level{
			Index:          i + 1,
			Depth:          l.Depth,
			Hops:           l.Hops,
			ScrapeInterval: l.ScrapeInterval.Duration,
			EnableEBPF:     l.EnableEBPF,
		}
	}
	return out
}

// ClearFor implements knowledge.EscalationPolicy.
func (p *Policy) ClearFor() time.Duration { return p.clearFor }
