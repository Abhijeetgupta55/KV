// Command server runs a single raftkv node: a gRPC server exposing the
// kv.v1 API backed by an in-memory store. Persistence and replication
// arrive in later milestones.
package main

import (
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"github.com/Abhijeetgupta55/raftkv/internal/server"
	"github.com/Abhijeetgupta55/raftkv/internal/storage"
	kvv1 "github.com/Abhijeetgupta55/raftkv/proto/kv/v1"
)

func main() {
	listenAddr := flag.String("listen", "127.0.0.1:5001", "address to serve the KV gRPC API on")
	flag.Parse()

	if err := run(*listenAddr); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}

func run(listenAddr string) error {
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}

	grpcServer := grpc.NewServer()
	kvv1.RegisterKVServer(grpcServer, server.New(storage.NewMemStore()))

	// Shut down gracefully on Ctrl-C / SIGTERM: stop accepting new RPCs,
	// let in-flight ones finish, then return.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-stop
		slog.Info("shutting down", "signal", sig.String())
		grpcServer.GracefulStop()
	}()

	slog.Info("serving KV API", "addr", lis.Addr().String())
	return grpcServer.Serve(lis)
}
