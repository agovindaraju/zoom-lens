package crd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"github.com/zoomlens/zoomlens/api/v1alpha1"
)

// GroupVersionResources of the knowledge resources.
var (
	PacksGVR     = v1alpha1.GroupVersion.WithResource("metrictreepacks")
	RelationsGVR = v1alpha1.GroupVersion.WithResource("metricrelations")
	GraphsGVR    = v1alpha1.GroupVersion.WithResource("entitygraphs")
)

// APILoader lists the cluster-scoped knowledge resources from the API server.
type APILoader struct {
	Client dynamic.Interface
}

// Load lists all three resource types and builds a Snapshot.
func (l APILoader) Load(ctx context.Context) (*Snapshot, error) {
	var packs []v1alpha1.MetricTreePack
	var rels []v1alpha1.MetricRelations
	var graphs []v1alpha1.EntityGraph
	if err := list(ctx, l.Client, PacksGVR, &packs); err != nil {
		return nil, err
	}
	if err := list(ctx, l.Client, RelationsGVR, &rels); err != nil {
		return nil, err
	}
	if err := list(ctx, l.Client, GraphsGVR, &graphs); err != nil {
		return nil, err
	}
	return Build(packs, rels, graphs)
}

func list[T any](ctx context.Context, c dynamic.Interface, gvr schema.GroupVersionResource, out *[]T) error {
	ul, err := c.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list %s (are deploy/crds and knowledge/ applied?): %w", gvr.Resource, err)
	}
	for _, u := range ul.Items {
		var obj T
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &obj); err != nil {
			return fmt.Errorf("decode %s %s: %w", gvr.Resource, u.GetName(), err)
		}
		*out = append(*out, obj)
	}
	return nil
}

// Run reloads the snapshot every interval until ctx ends. A failed reload
// keeps the previous snapshot.
func Run(ctx context.Context, l APILoader, st *Store, interval time.Duration, log *slog.Logger) {
	t := time.NewTicker(interval)
	defer t.Stop()
	last := st.Get()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		sn, err := l.Load(ctx)
		if err != nil {
			log.Warn("knowledge reload failed, keeping the previous resources", "error", err)
			continue
		}
		st.Set(sn)
		if last == nil || sn.Stats != last.Stats {
			log.Info("knowledge reloaded", "stats", sn.Stats)
		}
		last = sn
	}
}

// LoadFiles reads knowledge resources from YAML files or directories (walked
// recursively, *.yaml and *.yml). Documents of other kinds, such as a
// kustomization, are ignored.
func LoadFiles(paths ...string) (*Snapshot, error) {
	var packs []v1alpha1.MetricTreePack
	var rels []v1alpha1.MetricRelations
	var graphs []v1alpha1.EntityGraph
	for _, root := range paths {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if ext := filepath.Ext(p); ext != ".yaml" && ext != ".yml" {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return decodeDocs(p, raw, &packs, &rels, &graphs)
		})
		if err != nil {
			return nil, err
		}
	}
	return Build(packs, rels, graphs)
}

func decodeDocs(path string, raw []byte, packs *[]v1alpha1.MetricTreePack, rels *[]v1alpha1.MetricRelations, graphs *[]v1alpha1.EntityGraph) error {
	r := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(raw)))
	for {
		doc, err := r.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		var tm metav1.TypeMeta
		if err := yaml.Unmarshal(doc, &tm); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if tm.APIVersion != v1alpha1.GroupVersion.String() {
			continue
		}
		var derr error
		switch tm.Kind {
		case "MetricTreePack":
			derr = appendDoc(doc, packs)
		case "MetricRelations":
			derr = appendDoc(doc, rels)
		case "EntityGraph":
			derr = appendDoc(doc, graphs)
		}
		if derr != nil {
			return fmt.Errorf("%s: %s: %w", path, tm.Kind, derr)
		}
	}
}

func appendDoc[T any](doc []byte, out *[]T) error {
	var obj T
	if err := yaml.UnmarshalStrict(doc, &obj); err != nil {
		return err
	}
	*out = append(*out, obj)
	return nil
}
