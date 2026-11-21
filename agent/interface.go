package agent

import (
	"io"

	"github.com/k20ku/epilog/discovery"
)

type AgentHandler interface {
	discovery.Handler
	io.Closer
}
