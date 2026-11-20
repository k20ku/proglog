package agent

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"github.com/k20ku/proglog/internal/auth"
	"github.com/k20ku/proglog/internal/discovery"
	"github.com/k20ku/proglog/internal/discovery/handlers"
	"github.com/k20ku/proglog/internal/log"
	"github.com/k20ku/proglog/internal/server"
)

// Agent はすべてのインスタンスの上で動作をする．
// 異なるすべてのコンポーネントをセットアップして，接続する．
type Agent struct {
	AgentConfig

	Logger *slog.Logger

	// START: components
	log        *log.Log
	server     *grpc.Server
	conn       *grpc.ClientConn // local client used by the handler
	membership *discovery.Membership
	handler    AgentHandler
	// END: conponents

	mu          sync.Mutex // locks state, shutdownErr and Start/Shutdown Bodies
	state       AgentState
	shutdownErr error
}

type AgentConfig struct {
	ServerTLSConfig *tls.Config
	PeerTLSConfig   *tls.Config
	DataDir         string
	BindAddr        string
	RPCPort         int
	NodeName        string
	StartJoinAddrs  []string
	ACLModelFile    string
	ACLPolicyFile   string
}

func (c AgentConfig) RPCAddr() (string, error) {
	host, _, err := net.SplitHostPort(c.BindAddr)
	if err != nil {
		return "", fmt.Errorf("host of BindAddr(%s): %w", c.BindAddr, err)
	}
	return net.JoinHostPort(host, fmt.Sprint(c.RPCPort)), nil
}

func New(aconf AgentConfig, logger *slog.Logger) (*Agent, error) {
	if logger == nil {
		logger = slog.Default().WithGroup(fmt.Sprintf("agent-%s", aconf.NodeName))
	}
	alog, err := log.NewLog(aconf.DataDir, log.NewConfig())
	if err != nil {
		return nil, fmt.Errorf("new %s agent: %w", aconf.DataDir, err)
	}
	srv, err := newGRPCServer(aconf, logger, alog)
	if err != nil {
		// Do not leak the Log we just opened
		return nil, errors.Join(
			fmt.Errorf("agent %s: new server: %w", aconf.NodeName, err),
			alog.Close(),
		)
	}
	a := &Agent{
		AgentConfig: aconf,
		Logger:      logger,
		log:         alog,
		server:      srv,
	}
	return a, nil
}

func newGRPCServer(aconf AgentConfig, logger *slog.Logger, alog server.CommitLog) (*grpc.Server, error) {
	authorizer, err := auth.New(
		aconf.ACLModelFile,
		aconf.ACLPolicyFile,
	)
	if err != nil {
		return nil, fmt.Errorf("agent new aclAuthrizer: %w", err)
	}
	aclAuth := server.NewACLAuthorizer(authorizer)
	serverConfig := &server.Config{
		Logger:     logger.WithGroup("server"),
		CommitLog:  alog,
		Authorizer: aclAuth,
	}

	// TLS
	var opts []grpc.ServerOption
	if aconf.ServerTLSConfig != nil {
		creds := credentials.NewTLS(aconf.ServerTLSConfig)
		opts = append(opts, grpc.Creds(creds))
	}
	srv, err := server.NewGRPCServer(serverConfig, opts...)
	if err != nil {
		return nil, fmt.Errorf("agent new gRPC server: %w", err)
	}
	return srv, nil
}

func (a *Agent) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch a.state {
	case StateStarted:
		return errors.New("agent already started")
	case StateStopped:
		return errors.New("agent already stopped")
	}

	rpcAddr, err := a.RPCAddr()
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", rpcAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", rpcAddr, err)
	}

	// Assign right after each constructor
	handler, conn, err := newReplicator(a.AgentConfig, a.Logger)
	if err != nil {
		return fmt.Errorf("agent %s new handler: %w")
	}
	a.handler = handler

	return nil
}

func newReplicator(aconf AgentConfig, logger *slog.Logger) (AgentHandler, *grpc.ClientConn, error) {
	rpcAddr, err := aconf.RPCAddr()
	if err != nil {
		return nil, nil, err
	}

	var opts []grpc.DialOption
	if aconf.PeerTLSConfig != nil {
		opts = append(opts, grpc.WithTransportCredentials(
			credentials.NewTLS(aconf.PeerTLSConfig),
		))
	}
	conn, err := grpc.NewClient(rpcAddr, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("new local client for %s: %w", rpcAddr, err)
	}

	return &handlers.Replicator{
		DialOptions: opts, // peers are dialed with the same credentials
		LocalServer: api.NewLogServiceClient(conn),
		Logger:      logger.WithGroup("replicator"),
	}, conn, nil
}

func newMembership(
	aconf AgentConfig,
	handler discovery.Handler,
	logger *slog.Logger,
) (*discovery.Membership, error) {
	rpcAddr, err := aconf.RPCAddr()
	if err != nil {
		return nil, err
	}

	m, err := discovery.NewMembership(discovery.Config{
		NodeName: aconf.NodeName,
		BindAddr: aconf.BindAddr,
		Tags: map[discovery.MembershipTag]string{
			discovery.M_RPC_ADDR: rpcAddr,
		},
		StartJoinAddrs: aconf.StartJoinAddrs,
	}, handler, logger)
	if err != nil {
		return nil, fmt.Errorf("new membership: %w", err)
	}
	return m, nil
}

func (a *Agent) Shutdown() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state == StateStopped {
		return nil
	}
	a.state = StateStopped
	a.shutdownErr = a.shutdown()
	return a.shutdownErr
}

// shutdown membership, handlers, stop server, close commit log.
//
// NOTE: Users of this method MUST get the mutex.
func (a *Agent) shutdown() error {
	var errs []error

	if a.membership != nil {
		errs = append(errs, a.membership.Shutdown())
	}

	if a.handler != nil {
		errs = append(errs, a.handler.Close())
	}

	a.stopServer()

	if a.conn != nil {
		errs = append(errs, a.conn.Close())
	}

	errs = append(errs, a.log.Close())

	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("agent %s shutdown: %w", a.NodeName, err)
	}
	return nil
}

const GracefulStopTimeout = 15 * time.Second

func (a *Agent) stopServer() {
	done := make(chan struct{})
	go func() {
		// GracefulStop blocks until all the pending RPC finished
		a.server.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(GracefulStopTimeout):
		a.server.Stop()
		<-done
	}
}
