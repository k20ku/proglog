package discovery_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/consul/sdk/v2/freeport"
	"github.com/hashicorp/serf/serf"
	. "github.com/k20ku/proglog/internal/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMembership(t *testing.T) {
	memberships := make([]*Membership, 3)
	leaveJoinHandler := &handler{
		joins:  make(chan map[string]string, 3),
		leaves: make(chan string, 3),
	}
	ports := freeport.GetN(t, 3)
	addrs := make([]string, 3)
	for i, port := range ports {
		addrs[i] = fmt.Sprintf("%s:%d", "127.0.0.1", port)
	}
	leaderId := 0
	for id := range 3 {
		cfg := Config{
			NodeName: fmt.Sprintf("node%d", id),
			BindAddr: addrs[id],
			Tags: map[Tag]string{
				RPC_ADDR: addrs[id],
			},
		}
		if id != leaderId {
			cfg.StartJoinAddrs = []string{
				addrs[leaderId],
			}
		}
		m, err := NewMembership(leaveJoinHandler, cfg)
		require.NoErrorf(t, err, "new membership at id(%d) failed", id)
		memberships[id] = m
	}

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		require.Equal(c,
			3, len(memberships[leaderId].Members()),
			"incollect num of all members",
		)
		require.Equal(c,
			3, len(leaveJoinHandler.joins),
			"incollect num of joined members",
		)
		require.Equal(c,
			0,
			len(leaveJoinHandler.leaves),
			"incollect num of left members",
		)
	}, 1500*time.Millisecond, 250*time.Millisecond)

	leftId := 3 - 1
	require.NoErrorf(t,
		memberships[leftId].Leave(),
		"node%d failed to leave",
		leftId,
	)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		require.Equalf(c,
			serf.StatusLeft.String(),
			memberships[leaderId].Members()[leftId].Status.String(),
			"node%d is not StatusLeft",
			leftId,
		)
		require.Equal(c,
			3, len(memberships[leaderId].Members()),
			"incollect num of all members",
		)
		require.Equal(c,
			3, len(leaveJoinHandler.joins),
			"incollect num of joined members",
		)
		require.Equal(c,
			1,
			len(leaveJoinHandler.leaves),
			"incollect num of left members",
		)
	}, 3000*time.Millisecond, 250*time.Millisecond)

	require.Equal(t,
		fmt.Sprintf("node%d", leftId),
		<-leaveJoinHandler.leaves,
		"left id is incollect",
	)
}

var _ Handler = &handler{}

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
