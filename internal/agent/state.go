package agent

type AgentState int

const (
	StateNew AgentState = iota
	StateStarted
	StateStopped
)
