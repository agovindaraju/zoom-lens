// Package alerts parses Alertmanager webhook payloads into Zoom-Lens signals.
package alerts

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/zoomlens/zoomlens/internal/knowledge"
)

// Label names the operator reads from alerts. Alert rules opt in to Zoom-Lens
// by setting LabelRoot.
const (
	// LabelRoot is the root metric the alert rule evaluates.
	LabelRoot = "zoomlens_root"
	// LabelEntity is the entity kind the alert is about (Pod, Node).
	LabelEntity = "zoomlens_entity"

	labelPod       = "pod"
	labelNamespace = "namespace"
	labelNode      = "node"
	labelAlertName = "alertname"
)

// Alertmanager statuses.
const (
	StatusFiring   = "firing"
	StatusResolved = "resolved"
)

// Webhook is the Alertmanager webhook payload (version 4).
type Webhook struct {
	Version  string  `json:"version"`
	GroupKey string  `json:"groupKey"`
	Status   string  `json:"status"`
	Receiver string  `json:"receiver"`
	Alerts   []Alert `json:"alerts"`
}

// Alert is a single alert inside a webhook payload.
type Alert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
	Fingerprint string            `json:"fingerprint"`
}

// Signal is what the incident manager consumes.
type Signal struct {
	Fingerprint string
	AlertName   string
	Firing      bool
	StartsAt    time.Time
	EndsAt      time.Time
	Root        string
	Entity      knowledge.Entity
}

// Decode reads a webhook payload.
func Decode(r io.Reader) (*Webhook, error) {
	var w Webhook
	dec := json.NewDecoder(io.LimitReader(r, 4<<20))
	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("decode alertmanager webhook: %w", err)
	}
	return &w, nil
}

// Signals converts the payload. Alerts that do not carry the Zoom-Lens labels
// are returned in skipped so callers can count them.
func (w *Webhook) Signals() (signals []Signal, skipped int) {
	for _, a := range w.Alerts {
		s, ok := toSignal(a)
		if !ok {
			skipped++
			continue
		}
		signals = append(signals, s)
	}
	return signals, skipped
}

func toSignal(a Alert) (Signal, bool) {
	root := a.Labels[LabelRoot]
	if root == "" || a.Fingerprint == "" {
		return Signal{}, false
	}
	entity, ok := entityFrom(a.Labels)
	if !ok {
		return Signal{}, false
	}
	return Signal{
		Fingerprint: a.Fingerprint,
		AlertName:   a.Labels[labelAlertName],
		Firing:      a.Status == StatusFiring,
		StartsAt:    a.StartsAt,
		EndsAt:      a.EndsAt,
		Root:        root,
		Entity:      entity,
	}, true
}

func entityFrom(l map[string]string) (knowledge.Entity, bool) {
	kind := knowledge.EntityKind(l[LabelEntity])
	if kind == "" {
		// Infer from the most specific identifying label.
		switch {
		case l[labelPod] != "":
			kind = knowledge.KindPod
		case l[labelNode] != "":
			kind = knowledge.KindNode
		}
	}
	switch kind {
	case knowledge.KindPod:
		if l[labelPod] == "" || l[labelNamespace] == "" {
			return knowledge.Entity{}, false
		}
		return knowledge.Entity{Kind: kind, Name: l[labelPod], Namespace: l[labelNamespace], Node: l[labelNode]}, true
	case knowledge.KindNode:
		if l[labelNode] == "" {
			return knowledge.Entity{}, false
		}
		return knowledge.Entity{Kind: kind, Name: l[labelNode], Node: l[labelNode]}, true
	default:
		return knowledge.Entity{}, false
	}
}
