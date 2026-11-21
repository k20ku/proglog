package agent

import (
	"io"

	"github.com/k20ku/proglog/discovery"
)

type AgentHandler interface {
	discovery.Handler
	io.Closer
}
