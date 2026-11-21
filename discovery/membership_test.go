package discovery_test

import (
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/hashicorp/consul/sdk/v2/freeport"
	"github.com/hashicorp/serf/serf"
	. "github.com/k20ku/proglog/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMembership(t *testing.T) {
	n := 3
	// n copy of membership that exists in different nodes
	memberships := make([]*Membership, n)
	leaveJoinHandler := &handler{
		joins:  make(chan map[string]string, n),
		leaves: make(chan string, n),
	}
	ports := freeport.GetN(t, n)
	addrs := make([]string, n)
	for i, port := range ports {
		addrs[i] = fmt.Sprintf("%s:%d", "127.0.0.1", port)
	}
	leaderID := 0
	for id := range n {
		mconf := Config{
			NodeName: fmt.Sprintf("node%d", id),
			BindAddr: addrs[id],
			Tags: map[MembershipTag]string{
				M_RPC_ADDR: addrs[id],
			},
		}
		if id != leaderID {
			mconf.StartJoinAddrs = []string{
				addrs[leaderID],
			}
		}
		m, err := NewMembership(mconf, leaveJoinHandler, slog.Default().WithGroup("membership"))
		require.NoErrorf(t, err, "new membership at %s failed", m.NodeName)
		require.NoErrorf(t, m.Start(), "start membership at %s failed", m.NodeName)
		memberships[id] = m
	}

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		require.Equal(c,
			n, len(memberships[leaderID].Members()),
			"incollect num of all members",
		)
		require.Equal(c,
			n, len(leaveJoinHandler.joins),
			"incollect num of joined members",
		)
		require.Equal(c,
			0,
			len(leaveJoinHandler.leaves),
			"incollect num of left members",
		)
	}, 1500*time.Millisecond, 250*time.Millisecond)

	leftID := n - 1
	require.NoErrorf(t,
		memberships[leftID].Leave(),
		"node%d failed to leave",
		leftID,
	)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		require.Equalf(c,
			serf.StatusLeft.String(),
			memberships[leaderID].Members()[leftID].Status.String(),
			"node%d is not StatusLeft",
			leftID,
		)
		require.Equal(c,
			n, len(memberships[leaderID].Members()),
			"incollect num of all members",
		)
		require.Equal(c,
			n, len(leaveJoinHandler.joins),
			"incollect num of joined members",
		)
		require.Equal(c,
			1,
			len(leaveJoinHandler.leaves),
			"incollect num of left members",
		)
	}, 3000*time.Millisecond, 250*time.Millisecond)

	require.Equal(t,
		fmt.Sprintf("node%d", leftID),
		<-leaveJoinHandler.leaves,
		"left id is incollect",
	)
}

var _ Handler = (*handler)(nil)

type handler struct {
	joins  chan map[string]string
	leaves chan string
}

func (h *handler) Join(id, addr string) error {
	if h.joins != nil {
		h.joins <- map[string]string{
			"id":   id,
			"addr": addr,
		}
	}
	return nil
}

func (h *handler) Leave(id string) error {
	if h.leaves != nil {
		h.leaves <- id
	}
	return nil
}
