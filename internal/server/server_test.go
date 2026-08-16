package server_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/eclipse-pcs/pcs-service/internal/client"
	"github.com/eclipse-pcs/pcs-service/internal/config"
	"github.com/eclipse-pcs/pcs-service/internal/protocol"
	"github.com/eclipse-pcs/pcs-service/internal/server"
)

func startServer(t *testing.T, cfg *config.Config) (*server.Server, string) {
	t.Helper()
	if cfg.Token == "" {
		cfg.Token = "TEST_TOKEN"
	}
	if cfg.ChunkSize == 0 {
		cfg.ChunkSize = 4096
	}
	if cfg.SessionTimeout == 0 {
		cfg.SessionTimeout = time.Minute
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(cfg)
	go func() {
		_ = srv.Serve(ln)
	}()
	return srv, ln.Addr().String()
}

// TestGracefulShutdownWaitsForSession verifies Shutdown blocks until an in-flight split session completes.
func TestGracefulShutdownWaitsForSession(t *testing.T) {
	cfg := &config.Config{MaxObjectSize: 1 << 20}
	srv, addr := startServer(t, cfg)
	secret := []byte("shutdown drain test")
	cli, err := client.ConfigFromAddr(addr, cfg.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SplitSession(cli, secret); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

// TestMaxConcurrentSessionsRejects closes extra connections when MaxConcurrentSessions is reached.
func TestMaxConcurrentSessionsRejects(t *testing.T) {
	cfg := &config.Config{
		MaxConcurrentSessions: 1,
		MaxObjectSize:         1 << 20,
	}
	srv, addr := startServer(t, cfg)
	defer func() {
		_ = srv.Shutdown(context.Background())
	}()

	hold, release := holdSplitSession(t, addr, cfg.Token)
	defer release()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	n, err := conn.Read(buf)
	if n != 0 || err == nil {
		t.Fatalf("expected immediate close, read n=%d err=%v", n, err)
	}
	_ = hold
}

// holdSplitSession starts a split and blocks until release is called.
func holdSplitSession(t *testing.T, addr, token string) (*client.Session, func()) {
	t.Helper()
	cli, err := client.ConfigFromAddr(addr, token)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := cli.OpenSession(protocol.ModeSplit)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- sess.DrainParticles() }()
	if _, err := sess.Data.Doc.Write([]byte("block")); err != nil {
		t.Fatal(err)
	}
	return sess, func() {
		sess.Data.Doc.Close()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		_, _ = client.ReadTrailer(sess)
		sess.Close()
	}
}
