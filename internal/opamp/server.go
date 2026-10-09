// Package opamp runs the OpAMP server the collectors' Supervisors connect to,
// tracks which agent runs on which node, and pushes per-node remote configs.
package opamp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	clienttypes "github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server"
	"github.com/open-telemetry/opamp-go/server/types"

	"github.com/zoomlens/zoomlens/internal/metrics"
)

// NodeAttribute is the agent-description attribute that carries the
// Kubernetes node name. The Supervisor config sets it from K8S_NODE_NAME.
const NodeAttribute = "k8s.node.name"

// ConfigFileName is the key of the remote config inside the AgentConfigMap.
const ConfigFileName = "zoomlens.yaml"

const serverCapabilities = uint64(protobufs.ServerCapabilities_ServerCapabilities_AcceptsStatus |
	protobufs.ServerCapabilities_ServerCapabilities_OffersRemoteConfig |
	protobufs.ServerCapabilities_ServerCapabilities_AcceptsEffectiveConfig)

// AgentInfo describes a connected agent for the debug endpoint.
type AgentInfo struct {
	InstanceUID  string    `json:"instanceUid"`
	Node         string    `json:"node"`
	Host         string    `json:"host,omitempty"`
	ConfigStatus string    `json:"configStatus"`
	ConfigError  string    `json:"configError,omitempty"`
	InSync       bool      `json:"inSync"`
	LastSeen     time.Time `json:"lastSeen"`
}

type agent struct {
	uid         []byte
	conn        types.Connection
	node        string
	host        string
	sentHash    []byte
	appliedHash []byte
	status      protobufs.RemoteConfigStatuses
	statusMsg   string
	lastSeen    time.Time
}

// Server is the Zoom-Lens OpAMP server.
type Server struct {
	log  *slog.Logger
	m    *metrics.Metrics
	srv  server.OpAMPServer
	noop []byte

	mu      sync.Mutex
	agents  map[string]*agent // by hex instance UID
	desired map[string][]byte // node -> config body
	// known is false until the first SetDesired call. Until then no configs
	// are pushed, so a restarted operator does not wipe overlays that
	// collectors are still running for ongoing incidents.
	known bool
}

// New returns a Server. noopBody is sent to nodes without an overlay.
func New(log *slog.Logger, m *metrics.Metrics, noopBody []byte) *Server {
	return &Server{
		log:     log,
		m:       m,
		srv:     server.New(&logAdapter{log: log}),
		noop:    noopBody,
		agents:  map[string]*agent{},
		desired: map[string][]byte{},
	}
}

// Start listens for agents on addr (e.g. ":4320") at /v1/opamp.
func (s *Server) Start(addr string) error {
	return s.srv.Start(server.StartSettings{
		ListenEndpoint: addr,
		Settings: server.Settings{
			Callbacks: types.Callbacks{
				OnConnecting: func(*http.Request) types.ConnectionResponse {
					return types.ConnectionResponse{
						Accept: true,
						ConnectionCallbacks: types.ConnectionCallbacks{
							OnMessage:         s.onMessage,
							OnConnectionClose: s.onClose,
						},
					}
				},
			},
		},
	})
}

// Stop shuts the server down.
func (s *Server) Stop(ctx context.Context) error { return s.srv.Stop(ctx) }

// SetDesired replaces the desired config for every node and pushes changes.
// Nodes missing from bodies get the no-op config.
func (s *Server) SetDesired(bodies map[string][]byte) {
	s.mu.Lock()
	s.known = true
	s.desired = make(map[string][]byte, len(bodies))
	for n, b := range bodies {
		s.desired[n] = b
	}
	var sends []pending
	for _, a := range s.agents {
		if p, ok := s.pendingFor(a); ok {
			sends = append(sends, p)
		}
	}
	s.mu.Unlock()

	for _, p := range sends {
		msg := &protobufs.ServerToAgent{InstanceUid: p.uid, Capabilities: serverCapabilities, RemoteConfig: p.cfg}
		err := p.conn.Send(context.Background(), msg)
		result := "ok"
		if err != nil {
			result = "error"
			s.log.Error("push remote config failed", "node", p.node, "err", err)
			s.mu.Lock()
			if a, ok := s.agents[hex.EncodeToString(p.uid)]; ok {
				a.sentHash = nil // retry on the next message or change
			}
			s.mu.Unlock()
		} else {
			s.log.Info("pushed remote config", "node", p.node, "hash", hex.EncodeToString(p.cfg.ConfigHash)[:12])
		}
		s.m.ConfigPushes.WithLabelValues(p.node, result).Inc()
	}
}

// Agents returns the connected agents, sorted by node.
func (s *Server) Agents() []AgentInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AgentInfo, 0, len(s.agents))
	for id, a := range s.agents {
		out = append(out, AgentInfo{
			InstanceUID:  id,
			Node:         a.node,
			Host:         a.host,
			ConfigStatus: a.status.String(),
			ConfigError:  a.statusMsg,
			InSync: a.status == protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED &&
				bytes.Equal(a.appliedHash, hashOf(s.bodyFor(a.node))),
			LastSeen: a.lastSeen,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

type pending struct {
	uid  []byte
	node string
	conn types.Connection
	cfg  *protobufs.AgentRemoteConfig
}

// pendingFor returns the config to send to a, if any. Caller holds s.mu.
func (s *Server) pendingFor(a *agent) (pending, bool) {
	if !s.known || a.node == "" || a.conn == nil {
		return pending{}, false
	}
	body := s.bodyFor(a.node)
	h := hashOf(body)
	if bytes.Equal(a.appliedHash, h) || bytes.Equal(a.sentHash, h) {
		return pending{}, false
	}
	a.sentHash = h
	return pending{uid: a.uid, node: a.node, conn: a.conn, cfg: &protobufs.AgentRemoteConfig{
		Config: &protobufs.AgentConfigMap{ConfigMap: map[string]*protobufs.AgentConfigObject{
			ConfigFileName: {Body: body, ContentType: "text/yaml"},
		}},
		ConfigHash: h,
	}}, true
}

func (s *Server) bodyFor(node string) []byte {
	if b, ok := s.desired[node]; ok {
		return b
	}
	return s.noop
}

func (s *Server) onMessage(_ context.Context, conn types.Connection, msg *protobufs.AgentToServer) *protobufs.ServerToAgent {
	id := hex.EncodeToString(msg.GetInstanceUid())
	resp := &protobufs.ServerToAgent{InstanceUid: msg.GetInstanceUid(), Capabilities: serverCapabilities}

	s.mu.Lock()
	defer s.mu.Unlock()

	a, ok := s.agents[id]
	if !ok {
		a = &agent{uid: msg.GetInstanceUid()}
		s.agents[id] = a
		s.m.AgentsConnected.Set(float64(len(s.agents)))
	}
	if a.conn != conn {
		a.conn = conn
		a.sentHash = nil // new connection: re-evaluate what the agent has
	}
	a.lastSeen = time.Now()

	if d := msg.GetAgentDescription(); d != nil {
		node, host := describe(d)
		if node != "" && node != a.node {
			s.log.Info("agent identified", "instance_uid", id, "node", node, "host", host)
		}
		a.node, a.host = node, host
	}
	if st := msg.GetRemoteConfigStatus(); st != nil {
		if st.GetStatus() != a.status || st.GetErrorMessage() != a.statusMsg {
			s.log.Info("agent remote config status", "node", a.node, "status", st.GetStatus().String(), "error", st.GetErrorMessage())
		}
		if a.node != "" {
			s.m.RemoteConfig.DeletePartialMatch(map[string]string{"node": a.node})
			s.m.RemoteConfig.WithLabelValues(a.node, st.GetStatus().String()).Set(1)
		}
		a.status, a.statusMsg, a.appliedHash = st.GetStatus(), st.GetErrorMessage(), st.GetLastRemoteConfigHash()
		if st.GetStatus() == protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED {
			a.sentHash = nil // allow a retry once the desired config changes
		}
	}

	if a.node == "" {
		// We cannot target an agent whose node we do not know: ask for its
		// full state, which includes the agent description.
		resp.Flags = uint64(protobufs.ServerToAgentFlags_ServerToAgentFlags_ReportFullState)
		return resp
	}
	if p, ok := s.pendingFor(a); ok {
		resp.RemoteConfig = p.cfg
		s.m.ConfigPushes.WithLabelValues(a.node, "ok").Inc()
		s.log.Info("sending remote config in reply", "node", a.node, "hash", hex.EncodeToString(p.cfg.ConfigHash)[:12])
	}
	return resp
}

func (s *Server) onClose(conn types.Connection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, a := range s.agents {
		if a.conn == conn {
			s.log.Info("agent disconnected", "instance_uid", id, "node", a.node)
			delete(s.agents, id)
		}
	}
	s.m.AgentsConnected.Set(float64(len(s.agents)))
}

func describe(d *protobufs.AgentDescription) (node, host string) {
	for _, kvs := range [][]*protobufs.KeyValue{d.GetIdentifyingAttributes(), d.GetNonIdentifyingAttributes()} {
		for _, kv := range kvs {
			switch kv.GetKey() {
			case NodeAttribute:
				node = kv.GetValue().GetStringValue()
			case "host.name":
				host = kv.GetValue().GetStringValue()
			}
		}
	}
	return node, host
}

func hashOf(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

type logAdapter struct{ log *slog.Logger }

var _ clienttypes.Logger = (*logAdapter)(nil)

func (l *logAdapter) Debugf(_ context.Context, format string, v ...any) {
	l.log.Debug(fmt.Sprintf(format, v...), "component", "opamp")
}

func (l *logAdapter) Errorf(_ context.Context, format string, v ...any) {
	l.log.Error(fmt.Sprintf(format, v...), "component", "opamp")
}
