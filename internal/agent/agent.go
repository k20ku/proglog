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

	// コンポーネントはlogやserverなどである
	// START: components
	Logger     *slog.Logger
	log        *log.Log
	server     *grpc.Server
	membership *discovery.Membership
	handler    discovery.Handler
	// END: conponents

	shutdown      bool
	shutdownChans chan struct{}
	shutdownLock  sync.Mutex
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
	return fmt.Sprintf("%s:%d", host, c.RPCPort), nil
}

func New(aconf AgentConfig, logger *slog.Logger) (*Agent, error) {
	if logger == nil {
		logger = slog.Default().WithGroup(fmt.Sprintf("agent-%s", aconf.NodeName))
	}
	alog, err := log.NewLog(aconf.DataDir, log.NewConfig())
	if err != nil {
		return nil, fmt.Errorf("new %s agent: %w", aconf.DataDir, err)
	}
	serv, err := newLogServiceServer(aconf, logger, alog)
	if err != nil {
		errClose := alog.Close()
		return nil, fmt.Errorf("new %s agent init server: %w", errors.Join(err, errClose))
	}
	a := &Agent{
		AgentConfig:   aconf,
		Logger:        logger,
		shutdownChans: make(chan struct{}),
		log:           alog,
		server:        serv,
	}
	return a, nil
}

func (a *Agent) Start() error {
	return nil
}

func newLogServiceServer(aconf AgentConfig, logger *slog.Logger, alog *log.Log) (*grpc.Server, error) {
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

func newMembership(aconf AgentConfig, logger *slog.Logger) (*discovery.Membership, error) {
	rpcAddr, err := aconf.RPCAddr()
	if err != nil {
		return nil, err
	}
	var opts []grpc.DialOption
	if aconf.PeerTLSConfig != nil {
		opts = append(opts, grpc.WithTransportCredentials(
			credentials.NewTLS(aconf.PeerTLSConfig),
		))
	}
	conn, err := grpc.NewClient(rpcAddr, opts...)
	if err != nil {
		return nil, err
	}
	client := api.NewLogServiceClient(conn)
	replicator = &handlers.Replicator{
		DialOptions: opts,
		LocalServer: client,
		Logger:      logger.WithGroup("replicator"),
	}
	membership, err = discovery.NewMembership(discovery.Config{
		NodeName: a.NodeName,
		BindAddr: a.BindAddr,
		Tags: map[discovery.MembershipTag]string{
			discovery.M_RPC_ADDR: rpcAddr,
		},
		StartJoinAddrs: a.StartJoinAddrs,
	}, a.replicator, a.Logger)
	if err != nil {
		return fmt.Errorf("agent new membership: %w", err)
	}
	return nil
}

/*
*

	func (a *Agent) setupServer() error {
		rpcAddr, err := aconf.RPCAddr()
		if err != nil {
			return err
		}

		ln, err := net.Listen("tcp", rpcAddr)
		if err != nil {
			return err
		}
		go func() {
			if err := a.server.Serve(ln); err != nil {
				_ = a.Shutdown()
			}
		}()
		return err
	}
*/
func (a *Agent) setupMembership() error {
	rpcAddr, err := a.RPCAddr()
	if err != nil {
		return err
	}
	var opts []grpc.DialOption
	if a.PeerTLSConfig != nil {
		opts = append(opts, grpc.WithTransportCredentials(
			credentials.NewTLS(a.PeerTLSConfig),
		),
		)
	}
	conn, err := grpc.NewClient(rpcAddr, opts...)
	if err != nil {
		return err
	}
	client := api.NewLogServiceClient(conn)
	a.handler = &replicate.Replicator{
		DialOptions: opts,
		LocalServer: client,
		Logger:      a.Logger.WithGroup("replicator"),
	}
	a.membership, err = discovery.NewMembership(discovery.Config{
		NodeName: a.NodeName,
		BindAddr: a.BindAddr,
		Tags: map[discovery.MembershipTag]string{
			discovery.M_RPC_ADDR: rpcAddr,
		},
		StartJoinAddrs: a.StartJoinAddrs,
	}, a.handler, a.Logger)
	if err != nil {
		return fmt.Errorf("agent new membership: %w", err)
	}
	return nil
}

func (a *Agent) Shutdown() error {
	a.shutdownLock.Lock()
	defer a.shutdownLock.Unlock()
	if a.shutdown {
		return nil
	}
	a.shutdown = true
	close(a.shutdownChans)

	var errs []error
	if a.membership != nil {
		if err := a.membership.Shutdown(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.replicator != nil {

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
