package client_test

import (
	"bytes"
	"testing"

	"github.com/eclipse-pcs/pcs"

	"github.com/eclipse-pcs/pcs-service/internal/client"
)

func TestSplitCSVMergeKinds(t *testing.T) {
	got := client.SplitCSV("ec,oc")
	if len(got) != 2 || got[0] != "ec" || got[1] != "oc" {
		t.Fatalf("got %v", got)
	}
}

func TestConfigFromAddr(t *testing.T) {
	cfg, err := client.ConfigFromAddr("127.0.0.1:4567", "TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "127.0.0.1" || cfg.Port != 4567 || cfg.Token != "TOKEN" {
		t.Fatalf("cfg %+v", cfg)
	}
}

func TestModuleEncodeDecode(t *testing.T) {
	secret := []byte("client memory upload")
	result, err := pcs.Encode(secret)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pcs.DecodeFromParticles(result.EvenCypher, result.OddCypher, result.EvenNoise, result.OddNoise)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secret, got) {
		t.Fatalf("decode mismatch")
	}
}
