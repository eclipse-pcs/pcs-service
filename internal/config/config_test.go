package config

import (
	"strings"
	"testing"
)

// TestEnvMaxObjectSizeAndChunkSize reads PCS_MAX_OBJECT_SIZE and PCS_CHUNK_SIZE from the environment.
func TestEnvMaxObjectSizeAndChunkSize(t *testing.T) {
	t.Setenv("PCS_MAX_OBJECT_SIZE", "1048576")
	t.Setenv("PCS_CHUNK_SIZE", "8192")
	cfg, err := Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxObjectSize != 1<<20 {
		t.Fatalf("max-object-size: got %d want %d", cfg.MaxObjectSize, 1<<20)
	}
	if cfg.ChunkSize != 8192 {
		t.Fatalf("chunk-size: got %d want 8192", cfg.ChunkSize)
	}
}

// TestSecurityWarningDefaultTokenNonLoopback warns when the default token is used on a non-loopback bind.
func TestSecurityWarningDefaultTokenNonLoopback(t *testing.T) {
	cfg := &Config{Token: defaultToken, Listen: "0.0.0.0:4567"}
	warnings := cfg.SecurityWarnings()
	if len(warnings) != 1 {
		t.Fatalf("warnings: %v", warnings)
	}
	if !strings.Contains(warnings[0], "SECRET_TOKEN") {
		t.Fatalf("unexpected warning: %q", warnings[0])
	}
}

// TestSecurityWarningLoopback suppresses the default-token warning for loopback listen addresses.
func TestSecurityWarningLoopback(t *testing.T) {
	for _, listen := range []string{"127.0.0.1:4567", "localhost:4567", "[::1]:4567"} {
		cfg := &Config{Token: defaultToken, Listen: listen}
		if len(cfg.SecurityWarnings()) != 0 {
			t.Fatalf("listen %s: unexpected warnings", listen)
		}
	}
}

// TestIsLoopbackListen classifies listen addresses as loopback or externally reachable.
func TestIsLoopbackListen(t *testing.T) {
	if isLoopbackListen("0.0.0.0:4567") {
		t.Fatal("0.0.0.0 is not loopback")
	}
	if !isLoopbackListen("127.0.0.1:0") {
		t.Fatal("127.0.0.1 is loopback")
	}
}
