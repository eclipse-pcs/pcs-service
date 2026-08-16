package protocol_test

import (
	"bytes"
	"testing"

	"github.com/eclipse-pcs/pcs"
	"github.com/eclipse-pcs/pcs/stream"

	"pcs-service/internal/protocol"
)

func TestBuildTrailerFromEncodeMeta(t *testing.T) {
	secret := []byte("trailer meta test")
	particles, meta, err := stream.EncodeCollect(secret, bytes.Repeat([]byte{0x11}, len(secret)), 7)
	if err != nil {
		t.Fatal(err)
	}
	_ = particles
	got := protocol.BuildTrailerFromEncodeMeta(protocol.ModeSplit, meta)
	if got.SecretSHA256 == "" {
		t.Fatal("missing secret_sha256")
	}
	if got.BytesProcessed != int64(len(secret)) {
		t.Fatalf("bytes %d", got.BytesProcessed)
	}
	if len(got.FingerprintShard) != 6 {
		t.Fatalf("fingerprint_shard len %d", len(got.FingerprintShard))
	}
	if len(got.Footer) != 6 {
		t.Fatalf("footer len %d", len(got.Footer))
	}
	if got.WriteID == "" {
		t.Fatal("missing write_id")
	}
	if err := got.VerifyWriteIDsFromTrailer(); err != nil {
		t.Fatal(err)
	}
}

func TestMergeProfileFormatParse(t *testing.T) {
	cases := []uint8{0, protocol.MissingEC, protocol.MissingOC | protocol.MissingEN, protocol.MissingEC | protocol.MissingOC | protocol.MissingEN | protocol.MissingON}
	for _, mask := range cases {
		p := protocol.MergeProfile{MissingMask: mask}
		line := p.FormatProfileLine()
		got, err := protocol.ParseProfileLine(line)
		if err != nil {
			t.Fatalf("mask %#x: %v", mask, err)
		}
		if got.MissingMask != mask {
			t.Fatalf("mask %#x: got %#x", mask, got.MissingMask)
		}
	}
}

func TestMergeProfileMissingMap(t *testing.T) {
	p := protocol.MergeProfile{MissingMask: protocol.MissingEC | protocol.MissingEN}
	m := p.MissingMap()
	if !m[pcs.EvenCypher] || !m[pcs.EvenNoise] || m[pcs.OddCypher] || m[pcs.OddNoise] {
		t.Fatalf("unexpected map: %v", m)
	}
}
