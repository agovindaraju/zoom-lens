// Package v1alpha1 contains the Zoom-Lens knowledge API: the metric trees
// (DOWN axis), the links between metric sources, and the Kubernetes entity
// graph (OUT axis) that the operator uses to expand and contract collection.
//
// +kubebuilder:object:generate=true
// +groupName=zoomlens.dev
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is the API group and version of the Zoom-Lens knowledge types.
var GroupVersion = schema.GroupVersion{Group: "zoomlens.dev", Version: "v1alpha1"}

var (
	// SchemeBuilder registers the knowledge types with a runtime.Scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme adds the knowledge types to a scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion,
		&MetricTreePack{}, &MetricTreePackList{},
		&MetricRelations{}, &MetricRelationsList{},
		&EntityGraph{}, &EntityGraphList{},
	)
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}
