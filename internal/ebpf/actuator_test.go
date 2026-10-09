package ebpf

import (
	"context"
	"io"
	"log/slog"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newActuator() (*Actuator, *fake.Clientset) {
	c := fake.NewSimpleClientset()
	a := New(c, Options{Namespace: "zoomlens", Image: "img", Port: 9435, DeadlineSeconds: 60, ConfigDir: "/examples"},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	return a, c
}

func count(t *testing.T, c *fake.Clientset) int {
	t.Helper()
	l, err := c.CoreV1().Pods("zoomlens").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return len(l.Items)
}

func TestSyncCreatesKeepsAndDeletes(t *testing.T) {
	a, c := newActuator()
	ctx := context.Background()

	if _, err := a.Sync(ctx, map[string][]string{"n1": {"biolatency", "biolatency"}, "n2": nil}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, c); n != 1 {
		t.Fatalf("want 1 pod (n2 has no programs), got %d", n)
	}
	pods, _ := c.CoreV1().Pods("zoomlens").List(ctx, metav1.ListOptions{})
	p := pods.Items[0]
	if p.Spec.NodeName != "n1" || p.Annotations[AnnotationProgs] != "biolatency" || *p.Spec.ActiveDeadlineSeconds != 60 {
		t.Errorf("pod spec = node %q progs %q", p.Spec.NodeName, p.Annotations[AnnotationProgs])
	}

	// Same desired state: nothing changes.
	if _, err := a.Sync(ctx, map[string][]string{"n1": {"biolatency"}}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, c); n != 1 {
		t.Fatalf("want pod kept, got %d pods", n)
	}

	// Program set changes: old pod replaced.
	if _, err := a.Sync(ctx, map[string][]string{"n1": {"biolatency", "tcp-retransmit"}}); err != nil {
		t.Fatal(err)
	}
	pods, _ = c.CoreV1().Pods("zoomlens").List(ctx, metav1.ListOptions{})
	if len(pods.Items) != 1 || pods.Items[0].Annotations[AnnotationProgs] != "biolatency,tcp-retransmit" {
		t.Fatalf("want one pod with both programs, got %d", len(pods.Items))
	}

	// Revert: everything deleted.
	if _, err := a.Sync(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if n := count(t, c); n != 0 {
		t.Fatalf("want 0 pods after revert, got %d", n)
	}
}

func TestPodNameIsStableAndValid(t *testing.T) {
	a := podName("kind-worker", "biolatency")
	if a != podName("kind-worker", "biolatency") || len(a) > 63 {
		t.Fatalf("pod name %q must be stable and <= 63 chars", a)
	}
	if a == podName("kind-worker2", "biolatency") {
		t.Fatal("different nodes must get different pod names")
	}
}
