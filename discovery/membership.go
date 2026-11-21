package discovery

import (
	"errors"
	"fmt"
	"net"
	"sync"

	"log/slog"

	"github.com/hashicorp/serf/serf"
)

type MembershipTag string

const (
	M_RPC_ADDR MembershipTag = "rpc_addr"
)

type Config struct {
	NodeName       string
	BindAddr       string
	Tags           map[MembershipTag]string
	StartJoinAddrs []string
}

type Membership struct {
	Config
	handler     Handler
	serf        *serf.Serf
	events      chan serf.Event
	logger      *slog.Logger
	closeEvents sync.Once
}

// oh my gosh NewMembership start serf's gossip!
func NewMembership(mconf Config, handler Handler, logger *slog.Logger) (*Membership, error) {
	var err error

	if logger == nil {
		logger = slog.Default().WithGroup("membership")
	}

	// serf config
	sconf := serf.DefaultConfig()

	sconf.NodeName = mconf.NodeName

	var addr *net.TCPAddr
	addr, err = net.ResolveTCPAddr("tcp", mconf.BindAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve addr=%s: %w", mconf.BindAddr, err)
	}
	sconf.MemberlistConfig.BindAddr = addr.IP.String()
	sconf.MemberlistConfig.BindPort = addr.Port

	tags := make(map[string]string, len(mconf.Tags))
	for tag, val := range mconf.Tags {
		tags[string(tag)] = val
	}
	sconf.Tags = tags

	events := make(chan serf.Event)
	sconf.EventCh = events
	defer func() {
		if err != nil {
			close(events)
		}
	}()

	sconf.Init()

	// serf
	var _serf *serf.Serf
	_serf, err = serf.Create(sconf)
	if err != nil {
		return nil, fmt.Errorf("create serf: %w", err)
	}
	m := &Membership{
		Config:  mconf,
		handler: handler,
		logger:  logger,
		events:  events,
		serf:    _serf,
	}
	return m, nil
}

// Start starts event loop.
func (m *Membership) Start() error {
	var errs []error
	// serfのEventChの消費者
	go m.eventloop()

	if len(m.StartJoinAddrs) > 0 {
		// serfのEventChの生産者
		if _, errJoin := m.serf.Join(m.StartJoinAddrs, true); errJoin != nil {
			errs = append(errs, errJoin)
		}
	}
	if len(errs) > 0 {
		errShutdown := m.Shutdown()
		errs = append(errs, errShutdown)
		return fmt.Errorf("start membership at %s: %w", m.NodeName, errors.Join(errs...))
	}

	return nil
}

// Shutdown gracefully this membership.
// It is safe to call this multiple times.
// Internally, Leave is called when this is called.
func (m *Membership) Shutdown() error {
	if err := m.Leave(); err != nil {
		return fmt.Errorf("membership-%s shutdown: %w", m.NodeName, err)
	}
	if err := m.serf.Shutdown(); err != nil {
		return fmt.Errorf("membership-%s shutdown: %w", m.NodeName, err)
	}
	m.closeEvents.Do(func() { close(m.events) })
	return nil
}

func (m *Membership) eventloop() {
	// event loop
	for e := range m.events {
		switch e.EventType() {
		case serf.EventMemberJoin:
			// untangle serf's coalease event
			memberEvent, ok := e.(serf.MemberEvent)
			if !ok {
				continue
			}
			for _, member := range memberEvent.Members {
				if m.isLocal(member) {
					continue
				}
				m.handleJoin(member)
				m.logDebug("handleJoin", member)
			}
		case serf.EventMemberLeave, serf.EventMemberFailed:
			memberEvent, ok := e.(serf.MemberEvent)
			if !ok {
				continue
			}
			for _, member := range memberEvent.Members {
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
		member.Tags[string(M_RPC_ADDR)],
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

// Leave gracefully exits Membership.
// It is safe to call this multiple times.
func (m *Membership) Leave() error {
	return m.serf.Leave()
}

func (m *Membership) logError(err error, msg string, member serf.Member) {
	m.logger.Error(
		msg,
		slog.String("name", member.Name),
		slog.String(string(M_RPC_ADDR), member.Tags[string(M_RPC_ADDR)]),
		slog.String("error", err.Error()),
	)
}

func (m *Membership) logDebug(msg string, member serf.Member) {
	m.logger.Debug(
		msg,
		slog.String("name", member.Name),
		slog.String(string(M_RPC_ADDR), member.Tags[string(M_RPC_ADDR)]),
	)
}
