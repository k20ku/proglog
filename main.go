package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/k20ku/proglog/internal/server"
)

func run(ctx context.Context, l net.Listener) error {
	srv := server.NewLogServer()
	if err := srv.Serve(l); err != nil {
		return fmt.Errorf("failed to run server: %v", err)
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("need port number")
	}
	p := os.Args[1]

	// listen
	l, err := net.Listen("tcp", ":"+p)
	if err != nil {
		log.Fatalf("failed to listen port %s: %v", p, err)
	}

	ctx := context.TODO()
	if err := run(ctx, l); err != nil {
		log.Fatalf("failed to terminate server: %v", err)
	}
}
