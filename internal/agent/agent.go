package agent

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

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
	conn       *grpc.ClientConn
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

func newGRPCServer(aconf AgentConfig, logger *slog.Logger, alog *log.Log) (*grpc.Server, error) {
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

func newReplicator(aconf AgentConfig, logger *slog.Logger) (*handlers.Replicator, *grpc.ClientConn, error) {
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

func (a *Agent) Start() error {
	return nil
}

func (a *Agent) Shutdown() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state == StateStopped {
		return nil
	}
	a.state = StateStopped

	var errs []error
	if a.membership != nil {
		if err := a.membership.Shutdown(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.handler != nil {
	}
	shutdown := []func() error{
		a.membership.Leave,
		a.replicator.Close,
		func() error {
			a.server.GracefulStop()
			return nil
		},
		a.log.Close,
	}
	for _, fn := range shutdown {
		if err := fn(); err != nil {
			return fmt.Errorf("node name %s agent shutdown failed: %w", a.NodeName, err)
		}
	}
	return nil
}
