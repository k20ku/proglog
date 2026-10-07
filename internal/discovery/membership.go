package discovery

import (
	"net"

	"log/slog"

	"github.com/hashicorp/serf/serf"
)

type Tag string

const (
	RPC_ADDR Tag = "rpc_addr"
)

type Membership struct {
	Config
	handler Handler
	serf    *serf.Serf
	events  chan serf.Event
	logger  *slog.Logger
}

// oh my gosh NewMembership start serf's gossip!
func NewMembership(handler Handler, config Config) (*Membership, error) {
	membership := &Membership{
		Config:  config,
		handler: handler,
		logger:  slog.Default().WithGroup("membership"),
	}
	if err := membership.startSerf(); err != nil {
		return nil, err
	}
	return membership, nil
}

type Config struct {
	NodeName       string
	BindAddr       string
	Tags           map[Tag]string
	StartJoinAddrs []string
}

func (m *Membership) startSerf() (err error) {
	addr, err := net.ResolveTCPAddr("tcp", m.BindAddr)
	if err != nil {
		return err
	}
	// ---- setup serf config
	config := serf.DefaultConfig()
	config.Init()
	config.MemberlistConfig.BindAddr = addr.IP.String()
	config.MemberlistConfig.BindPort = addr.Port

	events := make(chan serf.Event)
	config.EventCh = events
	tags := make(map[string]string, len(m.Tags))
	for tag, val := range m.Tags {
		tags[string(tag)] = val
	}
	config.Tags = tags
	config.NodeName = m.Config.NodeName

	// ---- membership setup
	m.serf, err = serf.Create(config)
	m.events = events
	if err != nil {
		return err
	}
	go m.handleEvents()
	if m.StartJoinAddrs != nil {
		_, err = m.serf.Join(m.StartJoinAddrs, true)
		if err != nil {
			return err
		}
	}
	return nil
}

type Handler interface {
	Join(name, addr string) error
	Leave(name string) error
}

func (m *Membership) handleEvents() {
	// event loop
	for e := range m.events {
		switch e.EventType() {
		case serf.EventMemberJoin:
			// untangle serf's coalease event
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					continue
				}
				m.handleJoin(member)
				m.logDebug("handleJoin", member)
			}
		case serf.EventMemberLeave, serf.EventMemberFailed:
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					return
				}
				m.handleLeave(member)
				m.logDebug("handleLeave", member)
			}
		}
	}
}

func (m *Membership) handleJoin(member serf.Member) {
	if err := m.handler.Join(
		member.Name,
		member.Tags["rpc_addr"],
	); err != nil {
		m.logError(err, "failed to join", member)
	}
}

func (m *Membership) handleLeave(member serf.Member) {
	if err := m.handler.Leave(
		member.Name,
	); err != nil {
		m.logError(err, "failed to leave", member)
	}
}

func (m *Membership) isLocal(member serf.Member) bool {
	return m.serf.LocalMember().Name == member.Name
}

func (m *Membership) Members() []serf.Member {
	return m.serf.Members()
}

func (m *Membership) Leave() error {
	return m.serf.Leave()
}

func (m *Membership) logError(err error, msg string, member serf.Member) {
	m.logger.Error(
		msg,
		slog.String("name", member.Name),
		slog.String("rpc_addr", member.Tags["rpc_addr"]),
		slog.String("error", err.Error()),
	)
}

func (m *Membership) logDebug(msg string, member serf.Member) {
	m.logger.Debug(
		msg,
		slog.String("name", member.Name),
		slog.String("rpc_addr", member.Tags["rpc_addr"]),
	)
}
