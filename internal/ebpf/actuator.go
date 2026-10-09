// Package ebpf starts and stops ebpf_exporter pods on specific nodes.
//
// ebpf_exporter selects its eBPF programs at start-up (--config.names), so the
// operator runs one pod per node with exactly the programs that node needs.
// Creating the pod loads the programs into that node's kernel; deleting it
// unloads them.
package ebpf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

// Labels and annotations on managed pods.
const (
	LabelName       = "app.kubernetes.io/name"
	LabelComponent  = "app.kubernetes.io/component"
	LabelManagedBy  = "app.kubernetes.io/managed-by"
	AnnotationNode  = "zoomlens.dev/node"
	AnnotationProgs = "zoomlens.dev/programs"

	valueName      = "zoomlens-ebpf-exporter"
	valueComponent = "ebpf-exporter"
	valueManagedBy = "zoomlens"
)

// Options configure the pods the actuator creates.
type Options struct {
	Namespace string
	Image     string
	Port      int32
	// DeadlineSeconds is a safety net: the pod is terminated by Kubernetes
	// even if the operator dies before reverting.
	DeadlineSeconds int64
	// ConfigDir is where the image keeps its example configs.
	ConfigDir string
}

// PodInfo describes a managed pod for the debug endpoint.
type PodInfo struct {
	Name     string   `json:"name"`
	Node     string   `json:"node"`
	Programs []string `json:"programs"`
	Phase    string   `json:"phase"`
}

// Actuator reconciles ebpf_exporter pods.
type Actuator struct {
	client kubernetes.Interface
	opt    Options
	log    *slog.Logger
}

// New returns an Actuator.
func New(client kubernetes.Interface, opt Options, log *slog.Logger) *Actuator {
	return &Actuator{client: client, opt: opt, log: log}
}

func selector() string {
	return labels.SelectorFromSet(labels.Set{LabelComponent: valueComponent, LabelManagedBy: valueManagedBy}).String()
}

// Sync makes the set of managed pods match desired (node -> programs). Nodes
// with no programs get no pod.
func (a *Actuator) Sync(ctx context.Context, desired map[string][]string) ([]PodInfo, error) {
	want := map[string]string{} // node -> programs key
	for node, progs := range desired {
		if key := programsKey(progs); key != "" {
			want[node] = key
		}
	}

	list, err := a.client.CoreV1().Pods(a.opt.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector()})
	if err != nil {
		return nil, fmt.Errorf("list ebpf pods: %w", err)
	}

	var errs []error
	have := map[string]bool{}
	var infos []PodInfo
	for _, p := range list.Items {
		node := p.Annotations[AnnotationNode]
		key := p.Annotations[AnnotationProgs]
		finished := p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed
		if want[node] == key && !finished && p.DeletionTimestamp == nil {
			have[node] = true
			infos = append(infos, PodInfo{Name: p.Name, Node: node, Programs: strings.Split(key, ","), Phase: string(p.Status.Phase)})
			continue
		}
		if p.DeletionTimestamp != nil {
			continue
		}
		a.log.Info("deleting ebpf_exporter pod", "pod", p.Name, "node", node, "programs", key)
		if err := a.client.CoreV1().Pods(a.opt.Namespace).Delete(ctx, p.Name, metav1.DeleteOptions{
			GracePeriodSeconds: ptr.To[int64](5),
		}); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("delete pod %s: %w", p.Name, err))
		}
	}

	nodes := make([]string, 0, len(want))
	for n := range want {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		if have[node] {
			continue
		}
		pod := a.pod(node, want[node])
		a.log.Info("creating ebpf_exporter pod", "pod", pod.Name, "node", node, "programs", want[node])
		created, err := a.client.CoreV1().Pods(a.opt.Namespace).Create(ctx, pod, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			errs = append(errs, fmt.Errorf("create pod on %s: %w", node, err))
			continue
		}
		name := pod.Name
		if created != nil {
			name = created.Name
		}
		infos = append(infos, PodInfo{Name: name, Node: node, Programs: strings.Split(want[node], ","), Phase: string(corev1.PodPending)})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Node < infos[j].Node })
	return infos, errors.Join(errs...)
}

func (a *Actuator) pod(node, key string) *corev1.Pod {
	hostPath := func(name, path string) corev1.Volume {
		return corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: path}}}
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName(node, key),
			Namespace: a.opt.Namespace,
			Labels: map[string]string{
				LabelName: valueName, LabelComponent: valueComponent, LabelManagedBy: valueManagedBy,
			},
			Annotations: map[string]string{AnnotationNode: node, AnnotationProgs: key},
		},
		Spec: corev1.PodSpec{
			NodeName:                      node,
			RestartPolicy:                 corev1.RestartPolicyOnFailure,
			ActiveDeadlineSeconds:         ptr.To(a.opt.DeadlineSeconds),
			TerminationGracePeriodSeconds: ptr.To[int64](5),
			AutomountServiceAccountToken:  ptr.To(false),
			Tolerations:                   []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
			Containers: []corev1.Container{{
				Name:  "ebpf-exporter",
				Image: a.opt.Image,
				Args: []string{
					"--config.dir=" + a.opt.ConfigDir,
					"--config.names=" + key,
					fmt.Sprintf("--web.listen-address=:%d", a.opt.Port),
				},
				Ports: []corev1.ContainerPort{{Name: "metrics", ContainerPort: a.opt.Port}},
				SecurityContext: &corev1.SecurityContext{
					// Loading eBPF programs needs CAP_BPF + CAP_PERFMON (or
					// CAP_SYS_ADMIN on older kernels); privileged covers both.
					Privileged: ptr.To(true),
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("20m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
					Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "cgroup", MountPath: "/sys/fs/cgroup", ReadOnly: true},
					{Name: "debugfs", MountPath: "/sys/kernel/debug"},
				},
			}},
			Volumes: []corev1.Volume{hostPath("cgroup", "/sys/fs/cgroup"), hostPath("debugfs", "/sys/kernel/debug")},
		},
	}
}

// programsKey is the canonical, comma-separated, sorted program list.
func programsKey(progs []string) string {
	seen := map[string]bool{}
	var out []string
	for _, p := range progs {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func podName(node, key string) string {
	sum := sha256.Sum256([]byte(node + "|" + key))
	return "zl-ebpf-" + hex.EncodeToString(sum[:])[:12]
}
