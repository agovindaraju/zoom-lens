package config

import (
	"testing"
	"time"
)

func TestDefaultsAndEnvOverride(t *testing.T) {
	t.Setenv("ZOOMLENS_RECONCILE_INTERVAL", "5s")
	c, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.ReconcileInterval != 5*time.Second || c.OpAMPAddr != ":4320" || len(c.CollectorProcessors) != 2 {
		t.Fatalf("unexpected config %+v", c)
	}
}

func TestFlagsWinAndValidation(t *testing.T) {
	c, err := Load([]string{"-collector-processors=batch", "-ebpf-port=9999"})
	if err != nil || c.EBPFPort != 9999 || len(c.CollectorProcessors) != 1 {
		t.Fatalf("flags not applied: %+v, %v", c, err)
	}
	if _, err := Load([]string{"-log-format=xml"}); err == nil {
		t.Fatal("expected validation error")
	}
}
