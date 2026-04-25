package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	// TODO: Update this before sharing
	whinev1 "whine/gen/whine/v1"
	"whine/internal/engine"
	"whine/internal/server"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:41139", "gRPC listen address")
	flag.Parse()

	eng := engine.New()
	if err := eng.Start(); err != nil {
		log.Fatalf("engine start: %v", err)
	}

	defer eng.Stop()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	whinev1.RegisterWhineControlServer(grpcServer, server.New(eng))

	reflection.Register(grpcServer)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("received %s, fading out and shutting down...", sig)
		eng.Pause(300)
		time.Sleep(400 * time.Millisecond)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		done := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(done)
		}()

		select {
		case <-done:
		case <-shutdownCtx.Done():
			log.Println("graceful stop timed out, forcing")
			grpcServer.Stop()
		}
	}()

	log.Printf("whined: listening on %s", *addr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
	log.Printf("whined: stopped")
}
