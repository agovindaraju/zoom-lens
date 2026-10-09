package topology

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"

	"github.com/zoomlens/zoomlens/internal/knowledge"
)

// Graph is a knowledge.DependencyGraph for the edges that come from the Kubernetes API itself. It knows two edges:
//
//	Pod  --runs-on-->   Node         (hop 1 from a Pod)
//	Node --runs-->      Pods on it   (hop 1 from a Node, hop 2 from a Pod)
//
// It queries the API server directly on every call instead of keeping an
// informer-backed graph.
type Graph struct {
	client kubernetes.Interface
}

var _ knowledge.DependencyGraph = (*Graph)(nil)

// NewGraph returns a Graph that reads from the given client.
func NewGraph(client kubernetes.Interface) *Graph { return &Graph{client: client} }

// Related implements knowledge.DependencyGraph.
func (g *Graph) Related(ctx context.Context, e knowledge.Entity, hops int) ([]knowledge.Entity, error) {
	switch e.Kind {
	case knowledge.KindPod:
		return g.relatedToPod(ctx, e, hops)
	case knowledge.KindNode:
		return g.relatedToNode(ctx, e, hops)
	default:
		return nil, fmt.Errorf("topology graph: unsupported entity kind %q", e.Kind)
	}
}

func (g *Graph) relatedToPod(ctx context.Context, e knowledge.Entity, hops int) ([]knowledge.Entity, error) {
	pod, err := g.client.CoreV1().Pods(e.Namespace).Get(ctx, e.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) && e.Node != "" {
			// The pod is gone but the alert still names its node: keep going.
			pod = &corev1.Pod{Spec: corev1.PodSpec{NodeName: e.Node}}
		} else {
			return nil, fmt.Errorf("get pod %s/%s: %w", e.Namespace, e.Name, err)
		}
	}
	node := pod.Spec.NodeName
	self := knowledge.Entity{Kind: knowledge.KindPod, Name: e.Name, Namespace: e.Namespace, Node: node}
	out := []knowledge.Entity{self}
	if hops < 1 || node == "" {
		return out, nil
	}
	out = append(out, nodeEntity(node))
	if hops < 2 {
		return out, nil
	}
	pods, err := g.podsOnNode(ctx, node)
	if err != nil {
		return nil, err
	}
	for _, p := range pods {
		if p.Namespace == e.Namespace && p.Name == e.Name {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (g *Graph) relatedToNode(ctx context.Context, e knowledge.Entity, hops int) ([]knowledge.Entity, error) {
	out := []knowledge.Entity{nodeEntity(e.Name)}
	if hops < 1 {
		return out, nil
	}
	pods, err := g.podsOnNode(ctx, e.Name)
	if err != nil {
		return nil, err
	}
	return append(out, pods...), nil
}

func (g *Graph) podsOnNode(ctx context.Context, node string) ([]knowledge.Entity, error) {
	list, err := g.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", node).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("list pods on node %s: %w", node, err)
	}
	out := make([]knowledge.Entity, 0, len(list.Items))
	for _, p := range list.Items {
		if p.Status.Phase != corev1.PodRunning {
			continue
		}
		out = append(out, knowledge.Entity{Kind: knowledge.KindPod, Name: p.Name, Namespace: p.Namespace, Node: node})
	}
	return out, nil
}

func nodeEntity(name string) knowledge.Entity {
	return knowledge.Entity{Kind: knowledge.KindNode, Name: name, Node: name}
}
