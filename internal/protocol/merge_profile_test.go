package protocol_test

import (
	"bytes"
	"testing"

	"github.com/eclipse-pcs/pcs-service/internal/protocol"
)

func TestReadClientHandshake(t *testing.T) {
	var buf bytes.Buffer
	_, _ = buf.WriteString("merge\nprofile 9\n")
	mode, profile, err := protocol.ReadClientHandshake(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if mode != protocol.ModeMerge {
		t.Fatalf("mode %q", mode)
	}
	if profile.MissingMask != protocol.MissingEC|protocol.MissingON {
		t.Fatalf("mask %#x", profile.MissingMask)
	}
}

func TestMergeProfileFromSkipKinds(t *testing.T) {
	got, err := protocol.MergeProfileFromSkipKinds([]string{"ec", "on"})
	if err != nil {
		t.Fatal(err)
	}
	want := protocol.MissingEC | protocol.MissingON
	if got.MissingMask != want {
		t.Fatalf("got %#x want %#x", got.MissingMask, want)
	}
}

func TestMergeProfileValidate(t *testing.T) {
	if err := (protocol.MergeProfile{MissingMask: 0}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (protocol.MergeProfile{MissingMask: protocol.MissingEC | protocol.MissingOC}).Validate(); err == nil {
		t.Fatal("expected error for both cypher cores missing")
	}
}
