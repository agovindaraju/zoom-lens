package alerts

import (
	"strings"
	"testing"

	"github.com/zoomlens/zoomlens/internal/knowledge"
)

const payload = `{
  "version": "4", "status": "firing", "receiver": "zoomlens",
  "alerts": [
    {"status": "firing", "fingerprint": "abc123", "startsAt": "2026-10-01T10:00:00Z",
     "labels": {"alertname": "ZoomLensAppLatencyHigh", "zoomlens_root": "http_server_request_duration_seconds",
                "zoomlens_entity": "Pod", "pod": "app-1", "namespace": "demo", "node": "n1"}},
    {"status": "resolved", "fingerprint": "def456", "startsAt": "2026-10-01T09:00:00Z",
     "labels": {"alertname": "ZoomLensNodeDiskBusy", "zoomlens_root": "node_disk_io_time_seconds_total", "node": "n2"}},
    {"status": "firing", "fingerprint": "zzz", "labels": {"alertname": "Watchdog"}},
    {"status": "firing", "fingerprint": "yyy", "labels": {"alertname": "Broken", "zoomlens_root": "x", "zoomlens_entity": "Pod"}}
  ]}`

func TestSignals(t *testing.T) {
	w, err := Decode(strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	sigs, skipped := w.Signals()
	if skipped != 2 {
		t.Errorf("skipped = %d, want 2 (no root label, pod entity without pod label)", skipped)
	}
	if len(sigs) != 2 {
		t.Fatalf("got %d signals, want 2", len(sigs))
	}
	pod := sigs[0]
	if !pod.Firing || pod.Entity.Kind != knowledge.KindPod || pod.Entity.Name != "app-1" || pod.Entity.Node != "n1" {
		t.Errorf("pod signal = %+v", pod)
	}
	node := sigs[1]
	if node.Firing || node.Entity.Kind != knowledge.KindNode || node.Entity.Name != "n2" {
		t.Errorf("node signal (kind inferred from labels) = %+v", node)
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	if _, err := Decode(strings.NewReader("{not json")); err == nil {
		t.Fatal("expected error")
	}
}
