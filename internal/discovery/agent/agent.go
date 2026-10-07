package agent

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"github.com/k20ku/proglog/internal/auth"
	"github.com/k20ku/proglog/internal/discovery"
	"github.com/k20ku/proglog/internal/discovery/replicate"
	"github.com/k20ku/proglog/internal/log"
	"github.com/k20ku/proglog/internal/server"
)

// agentはすべてのインスタンスの上で動作をする．
// 異なるすべてのコンポーネントをセットアップして，接続する．
type Agent struct {
	Config

	// コンポーネントはlogやserverなどである
	// START: components
	Logger     *slog.Logger
	log        *log.Log
	server     *grpc.Server
	membership *discovery.Membership
	replicator *replicate.Replicator
	// END: conponents

	shutdown      bool
	shutdownChans chan struct{}
	shutdownLock  sync.Mutex
}

type Config struct {
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

func (c Config) RPCAddr() (string, error) {
	host, _, err := net.SplitHostPort(c.BindAddr)
	if err != nil {
		return "", fmt.Errorf("host of BindAddr(%s): %w", c.BindAddr, err)
	}
	return fmt.Sprintf("%s:%d", host, c.RPCPort), nil
}

func New(config Config) (*Agent, error) {
	a := &Agent{
		Config:        config,
		shutdownChans: make(chan struct{}),
	}
	setup := []func() error{
		a.setupLogger,
		a.setupLog,
		a.setupServer,
		a.setupMembership,
	}
	for _, fn := range setup {
		if err := fn(); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func (a *Agent) setupLogger() error {
	a.Logger = slog.Default().WithGroup(fmt.Sprintf("%s-agent", a.Config.NodeName))
	return nil
}

func (a *Agent) setupLog() error {
	var err error
	if a.log, err = log.NewLog(
		a.Config.DataDir,
		log.NewConfig(),
	); err != nil {
		return fmt.Errorf("agent setup log to %s: %w", a.Config.DataDir, err)
	}
	return nil
}

func (a *Agent) setupServer() error {
	var err error
	authorizer, err := auth.New(
		a.Config.ACLModelFile,
		a.Config.ACLPolicyFile,
	)
	if err != nil {
		return fmt.Errorf("agent new aclAuthrizer: %w", err)
	}
	aclAuth := server.NewACLAuthorizer(authorizer)
	clog := server.NewWalCommitLog(a.log)
	serverConfig := &server.Config{
		Logger:     a.Logger.WithGroup("server"),
		CommitLog:  clog,
		Authorizer: aclAuth,
	}

	// TLS
	var opts []grpc.ServerOption
	if a.Config.ServerTLSConfig != nil {
		creds := credentials.NewTLS(a.Config.ServerTLSConfig)
		opts = append(opts, grpc.Creds(creds))
	}
	a.server, err = server.NewGRPCServer(serverConfig, opts...)
	if err != nil {
		return fmt.Errorf("agent new gRPC server: %w", err)
	}
	rpcAddr, err := a.RPCAddr()
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

func (a *Agent) setupMembership() error {
	rpcAddr, err := a.Config.RPCAddr()
	if err != nil {
		return err
	}
	var opts []grpc.DialOption
	if a.Config.PeerTLSConfig != nil {
		opts = append(opts, grpc.WithTransportCredentials(
			credentials.NewTLS(a.Config.PeerTLSConfig),
		),
		)
	}
	conn, err := grpc.NewClient(rpcAddr, opts...)
	if err != nil {
		return err
	}
	client := api.NewLogServiceClient(conn)
	a.replicator = &replicate.Replicator{
		DialOptions: opts,
		LocalServer: client,
		Logger:      a.Logger.WithGroup("replicator"),
	}
	a.membership, err = discovery.NewMembership(a.replicator, discovery.Config{
		NodeName: a.Config.NodeName,
		BindAddr: a.Config.BindAddr,
		Tags: map[discovery.Tag]string{
			discovery.RPC_ADDR: rpcAddr,
		},
		StartJoinAddrs: a.Config.StartJoinAddrs,
	})
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
			return fmt.Errorf("Node Name %s agent shutdown failed: %w", a.Config.NodeName, err)
		}
	}
	return nil
}
