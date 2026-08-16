package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eclipse-pcs/pcs-service/internal/config"
	"github.com/eclipse-pcs/pcs-service/internal/server"
)

func main() {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	for _, w := range cfg.SecurityWarnings() {
		log.Printf("warning: %s", w)
	}
	srv := server.New(cfg)
	if cfg.MaxObjectSize > 0 {
		log.Printf("pcs-service listening on %s (max-object-size=%d, max-sessions=%d)", cfg.Listen, cfg.MaxObjectSize, cfg.MaxConcurrentSessions)
	} else {
		log.Printf("pcs-service listening on %s (max-object-size=unlimited, max-sessions=%d)", cfg.Listen, cfg.MaxConcurrentSessions)
	}

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Printf("pcs-service shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	if err := srv.Serve(ln); err != nil {
		log.Fatalf("server: %v", err)
	}
}
