package opamp

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"

	"github.com/open-telemetry/opamp-go/protobufs"

	"github.com/zoomlens/zoomlens/internal/metrics"
)

type fakeConn struct {
	mu   sync.Mutex
	sent []*protobufs.ServerToAgent
}

func (c *fakeConn) Connection() net.Conn { return nil }
func (c *fakeConn) Disconnect() error    { return nil }
func (c *fakeConn) Send(_ context.Context, m *protobufs.ServerToAgent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, m)
	return nil
}

func newServer() *Server {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), metrics.New(), []byte("noop"))
}

func hello(uid string, node string) *protobufs.AgentToServer {
	msg := &protobufs.AgentToServer{InstanceUid: []byte(uid)}
	if node != "" {
		msg.AgentDescription = &protobufs.AgentDescription{NonIdentifyingAttributes: []*protobufs.KeyValue{{
			Key: NodeAttribute, Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: node}},
		}}}
	}
	return msg
}

func TestUnknownNodeAsksForFullState(t *testing.T) {
	s := newServer()
	resp := s.onMessage(context.Background(), &fakeConn{}, hello("a1", ""))
	if resp.Flags&uint64(protobufs.ServerToAgentFlags_ServerToAgentFlags_ReportFullState) == 0 || resp.RemoteConfig != nil {
		t.Fatalf("unidentified agent should get ReportFullState and no config, got %+v", resp)
	}
}

func TestNoopOnConnectThenPushOnChange(t *testing.T) {
	s := newServer()
	conn := &fakeConn{}
	if resp := s.onMessage(context.Background(), conn, hello("a1", "n1")); resp.RemoteConfig != nil {
		t.Fatal("no config may be pushed before the desired state is known")
	}
	s.SetDesired(nil) // desired state known: nothing active
	if len(conn.sent) != 1 || string(conn.sent[0].RemoteConfig.Config.ConfigMap[ConfigFileName].Body) != "noop" {
		t.Fatalf("known-empty desired state should push noop, sent=%d", len(conn.sent))
	}
	conn.sent = nil
	resp := s.onMessage(context.Background(), &fakeConn{}, hello("a2", "n2"))
	if resp.RemoteConfig == nil || string(resp.RemoteConfig.Config.ConfigMap[ConfigFileName].Body) != "noop" {
		t.Fatalf("a newly connecting agent should receive the noop config in the reply, got %+v", resp.RemoteConfig)
	}
	s.onClose(nil) // no-op for unknown connections
	// Same message again: nothing new to send.
	if again := s.onMessage(context.Background(), conn, hello("a1", "")); again.RemoteConfig != nil {
		t.Fatal("config must not be re-sent while unchanged")
	}

	s.SetDesired(map[string][]byte{"n1": []byte("overlay"), "n2": []byte("other")})
	if len(conn.sent) != 1 || string(conn.sent[0].RemoteConfig.Config.ConfigMap[ConfigFileName].Body) != "overlay" {
		t.Fatalf("expected one push of the n1 overlay, got %d", len(conn.sent))
	}

	// Agent reports it applied the overlay: in sync, no more pushes.
	applied := hello("a1", "")
	applied.RemoteConfigStatus = &protobufs.RemoteConfigStatus{
		LastRemoteConfigHash: hashOf([]byte("overlay")),
		Status:               protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
	}
	s.onMessage(context.Background(), conn, applied)
	s.SetDesired(map[string][]byte{"n1": []byte("overlay")})
	if len(conn.sent) != 1 {
		t.Fatalf("no push expected when the agent already applied the config, got %d", len(conn.sent))
	}
	if a := s.Agents(); len(a) != 2 || !a[0].InSync || a[0].Node != "n1" {
		t.Fatalf("agents = %+v", a)
	}

	// Revert: noop pushed.
	s.SetDesired(nil)
	if len(conn.sent) != 2 || string(conn.sent[1].RemoteConfig.Config.ConfigMap[ConfigFileName].Body) != "noop" {
		t.Fatalf("revert should push noop, sent=%d", len(conn.sent))
	}

	failed := hello("a1", "")
	failed.RemoteConfigStatus = &protobufs.RemoteConfigStatus{
		LastRemoteConfigHash: hashOf([]byte("noop")),
		Status:               protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED,
		ErrorMessage:         "boom",
	}
	s.onMessage(context.Background(), conn, failed)
	if a := s.Agents(); a[0].InSync || a[0].ConfigError != "boom" {
		t.Fatalf("a FAILED config must not count as in sync: %+v", a[0])
	}

	s.onClose(conn)
	if len(s.Agents()) != 1 {
		t.Fatal("agent a1 should be removed on close, a2 kept")
	}
}
