package policy

import (
	"testing"
	"time"
)

const sample = `
clearFor: 2m
levels:
  - {after: 5m, depth: 2, hops: 1, scrapeInterval: 5s, enableEBPF: true}
  - {after: 0s, depth: 1, hops: 1, scrapeInterval: 10s}
`

func TestParseSortsLevels(t *testing.T) {
	p, err := Parse([]byte(sample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := p.ClearFor(); got != 2*time.Minute {
		t.Errorf("ClearFor = %v, want 2m", got)
	}
	if l := p.LevelFor(30 * time.Second); l.Index != 1 || l.ScrapeInterval != 10*time.Second {
		t.Errorf("LevelFor(30s) = %+v, want L1 @10s", l)
	}
	if l := p.LevelFor(6 * time.Minute); l.Index != 2 || !l.EnableEBPF {
		t.Errorf("LevelFor(6m) = %+v, want L2 with eBPF", l)
	}
}

func TestDefaultsAndValidation(t *testing.T) {
	p, err := Parse([]byte(`levels: [{after: 0s, depth: 1, hops: 1, scrapeInterval: 10s}]`))
	if err != nil || p.ClearFor() != DefaultClearFor {
		t.Fatalf("default clearFor: %v %v", p, err)
	}
	for name, doc := range map[string]string{
		"no levels":       `clearFor: 1m`,
		"scrape too fast": `levels: [{after: 0s, depth: 1, hops: 1, scrapeInterval: 100ms}]`,
		"negative depth":  `levels: [{after: 0s, depth: -1, hops: 1, scrapeInterval: 5s}]`,
		"unknown field":   `levels: [{after: 0s, depth: 1, hops: 1, scrapeInterval: 5s, bogus: 1}]`,
	} {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// The ladder shipped in deploy/ must always be valid.
func TestShippedLadderLoads(t *testing.T) {
	p, err := Load("../../../deploy/operator/ladder.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if l := p.LevelFor(time.Hour); l.Index < 2 {
		t.Fatalf("shipped ladder looks incomplete: %+v", l)
	}
}
