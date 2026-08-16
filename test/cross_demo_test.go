package test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pcs-service/internal/client"
	"pcs-service/internal/store"
)

func TestCrossDemoRoundTrip(t *testing.T) {
	// Absolute paths: the commands run with cmd.Dir set to temp dirs, so
	// relative binary paths would resolve against those, not this package.
	encodeBin, err := filepath.Abs(filepath.Join("..", "..", "pcs-demo", "pcs-encode"))
	if err != nil {
		t.Fatal(err)
	}
	decodeBin, err := filepath.Abs(filepath.Join("..", "..", "pcs-demo", "pcs-decode"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(encodeBin); err != nil {
		t.Skip("pcs-encode not built; run: (cd ../pcs-demo && go build -o pcs-encode ./cmd/pcs-encode)")
	}
	if _, err := os.Stat(decodeBin); err != nil {
		t.Skip("pcs-decode not built")
	}

	dir := t.TempDir()
	secret := []byte("cross-demo round trip")
	base := "cross.txt"
	secretPath := filepath.Join(dir, base)
	if err := os.WriteFile(secretPath, secret, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(encodeBin, secretPath)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pcs-encode: %v: %s", err, out)
	}

	addr, stop := startTestServer(t)
	defer stop()
	cfg := testClientConfig(addr, "TEST_TOKEN")
	profile, err := client.PrepareMergeFromFiles(dir, base)
	if err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, store.ReconstructedFileName(base))
	outFile, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.MergeFromFiles(cfg, dir, base, profile, outFile); err != nil {
		outFile.Close()
		t.Fatal(err)
	}
	outFile.Close()
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secret, got) {
		t.Fatalf("pcs-merge after pcs-encode mismatch")
	}

	dir2 := t.TempDir()
	base2 := "rev.txt"
	secretPath2 := filepath.Join(dir2, base2)
	if err := os.WriteFile(secretPath2, secret, 0o644); err != nil {
		t.Fatal(err)
	}
	host, port := splitHostPort(addr)
	splitBin, err := filepath.Abs(filepath.Join("..", "..", "pcs-service", "bin", "pcs-split"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(splitBin); err != nil {
		if err := buildSplitBin(t, splitBin); err != nil {
			t.Skip(err)
		}
	}
	splitCmd := exec.Command(splitBin,
		"-host", host, "-port", port, "-token", "TEST_TOKEN",
		"-f", secretPath2, "-o", dir2)
	if out, err := splitCmd.CombinedOutput(); err != nil {
		t.Fatalf("pcs-split: %v: %s", err, out)
	}
	decCmd := exec.Command(decodeBin, "-y", base2)
	decCmd.Dir = dir2
	if out, err := decCmd.CombinedOutput(); err != nil {
		t.Fatalf("pcs-decode: %v: %s", err, out)
	}
	got2, err := os.ReadFile(filepath.Join(dir2, store.ReconstructedFileName(base2)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secret, got2) {
		t.Fatalf("pcs-decode after pcs-split mismatch")
	}
}

func splitHostPort(addr string) (host, port string) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return "127.0.0.1", addr
	}
	return addr[:i], addr[i+1:]
}

func buildSplitBin(t *testing.T, path string) error {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-o", path, "./cmd/pcs-split")
	cmd.Dir = filepath.Join("..", "..", "pcs-service")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("build pcs-split: %v: %s", err, out)
	}
	return nil
}
