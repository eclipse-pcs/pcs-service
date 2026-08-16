package test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eclipse-pcs/pcs"
	"github.com/eclipse-pcs/pcs/stream"

	"github.com/eclipse-pcs/pcs-service/internal/client"
	"github.com/eclipse-pcs/pcs-service/internal/config"
	"github.com/eclipse-pcs/pcs-service/internal/protocol"
	"github.com/eclipse-pcs/pcs-service/internal/server"
	"github.com/eclipse-pcs/pcs-service/internal/store"
)

func startTestServerWithConfig(t *testing.T, cfg *config.Config) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Listen = ln.Addr().String()
	if cfg.Token == "" {
		cfg.Token = "TEST_TOKEN"
	}
	if cfg.SessionTimeout == 0 {
		cfg.SessionTimeout = time.Minute
	}
	if cfg.ChunkSize == 0 {
		cfg.ChunkSize = 7
	}
	srv := server.New(cfg)
	done := make(chan struct{})
	go func() {
		_ = srv.Serve(ln)
		close(done)
	}()
	return cfg.Listen, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		<-done
	}
}

func startTestServer(t *testing.T) (addr string, stop func()) {
	t.Helper()
	return startTestServerWithConfig(t, &config.Config{
		MaxObjectSize: 1 << 20,
		ChunkSize:     7,
	})
}

func TestRoundTripStreaming(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()
	secret := []byte("integration round trip streaming")
	cfg := testClientConfig(addr, "TEST_TOKEN")
	result, err := client.SplitSession(cfg, secret)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := client.MergeSession(cfg, result.Particles)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secret, got) {
		t.Fatalf("round trip mismatch:\nwant %q\ngot  %q", secret, got)
	}
	if result.Trailer.BytesProcessed != int64(len(secret)) {
		t.Fatalf("bytes processed %d", result.Trailer.BytesProcessed)
	}
}

func testClientConfig(addr, token string) *client.Config {
	cfg, err := client.ConfigFromAddr(addr, token)
	if err != nil {
		panic(err)
	}
	return cfg
}

func TestRandomLengthsRoundTrip(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()
	cfg := testClientConfig(addr, "TEST_TOKEN")
	for i := 0; i < 20; i++ {
		n := (i * 13) % 200
		secret := make([]byte, n)
		rand.Read(secret)
		result, err := client.SplitSession(cfg, secret)
		if err != nil {
			t.Fatal(err)
		}
		got, _, err := client.MergeSession(cfg, result.Particles)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(secret, got) {
			t.Fatalf("len %d mismatch", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSplitSizeLimitReportsServerError(t *testing.T) {
	const maxSize = 32
	addr, stop := startTestServerWithConfig(t, &config.Config{MaxObjectSize: maxSize})
	defer stop()
	secret := make([]byte, maxSize+16)
	rand.Read(secret)

	cfg := testClientConfig(addr, "TEST_TOKEN")
	sess, err := cfg.OpenSession(protocol.ModeSplit)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	done := make(chan error, 1)
	go func() { done <- sess.DrainParticles() }()

	if err := sess.UploadDocument(bytes.NewReader(secret)); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	_, err = client.ReadTrailer(sess)
	if err == nil {
		t.Fatal("expected server error")
	}
	var sessionErr *protocol.SessionError
	if !errors.As(err, &sessionErr) {
		t.Fatalf("got %T: %v", err, err)
	}
	if !bytes.Contains([]byte(sessionErr.Message), []byte("object exceeds max size")) {
		t.Fatalf("message %q", sessionErr.Message)
	}
}

func TestTCPParityRecoveryMerge(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()
	secret := []byte("parity recovery TCP")
	cfg := testClientConfig(addr, "TEST_TOKEN")
	result, err := client.SplitSession(cfg, secret)
	if err != nil {
		t.Fatal(err)
	}
	core := result.Particles
	delete(core, pcs.EvenCypher)
	got, trailer, err := client.MergeSession(cfg, core)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secret, got) {
		t.Fatalf("got %q want %q", got, secret)
	}
	if len(trailer.Recoveries) == 0 {
		t.Fatal("expected recovery note in trailer")
	}
}

func TestGoldenFooterLayout(t *testing.T) {
	dir := t.TempDir()
	secret := []byte("Golden file comparison")
	base := "Hello.txt"
	addr, stop := startTestServer(t)
	defer stop()
	cfg := testClientConfig(addr, "TEST_TOKEN")
	splitResult, err := client.SplitSession(cfg, secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureStorageDirs(dir); err != nil {
		t.Fatal(err)
	}
	for kind, data := range splitResult.Particles {
		path := filepath.Join(dir, store.ParticleRelPath(base, kind))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	particles := make(map[pcs.ParticleKind][]byte, len(splitResult.Particles))
	for kind, data := range splitResult.Particles {
		particles[kind] = data
	}
	got, _, err := stream.DecodeCollect(particles)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secret, got) {
		t.Fatalf("decode mismatch: %q vs %q", secret, got)
	}
}
