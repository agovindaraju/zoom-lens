package topology

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/zoomlens/zoomlens/internal/knowledge"
)

func TestGraphRelatedToPod(t *testing.T) {
	running := corev1.PodStatus{Phase: corev1.PodRunning}
	client := fake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "demo"}, Spec: corev1.PodSpec{NodeName: "n1"}, Status: running},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "noisy", Namespace: "demo"}, Spec: corev1.PodSpec{NodeName: "n1"}, Status: running},
	)
	g := NewGraph(client)
	ctx := context.Background()

	one, err := g.Related(ctx, knowledge.Entity{Kind: knowledge.KindPod, Name: "app", Namespace: "demo"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 2 || one[1].Kind != knowledge.KindNode || one[1].Name != "n1" {
		t.Fatalf("hops=1: got %+v, want pod + node n1", one)
	}
	// The fake clientset does not evaluate field selectors, so hops=2 returns
	// every running pod; the important part is that the pod itself is not
	// duplicated.
	two, err := g.Related(ctx, knowledge.Entity{Kind: knowledge.KindPod, Name: "app", Namespace: "demo"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(two) != 3 {
		t.Fatalf("hops=2: got %+v, want pod + node + noisy", two)
	}
}
