package protocol_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/eclipse-pcs/pcs-service/internal/protocol"
)

func TestWriteErrorLineAndReadTrailer(t *testing.T) {
	var buf bytes.Buffer
	want := "stream encode: object exceeds max size 67108864"
	if err := protocol.WriteErrorLine(&buf, errors.New(want)); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "error "+want+"\n" {
		t.Fatalf("frame %q", got)
	}
	_, err := protocol.ReadTrailer(&buf)
	if err == nil {
		t.Fatal("expected error")
	}
	var sessionErr *protocol.SessionError
	if !errors.As(err, &sessionErr) {
		t.Fatalf("got %T: %v", err, err)
	}
	if sessionErr.Message != want {
		t.Fatalf("message %q", sessionErr.Message)
	}
}

func TestReadTrailerStillParsesSuccess(t *testing.T) {
	valid := true
	tr := &protocol.Trailer{
		SecretSHA256:     "abc",
		FingerprintShard: map[string]string{"ec": "0102030405060708090a0b0c0d0e0f10"},
		CrossCRC:         map[string]uint32{"ec": 1},
		Length:           3,
		WriteID:          "0123456789abcdef",
		BytesProcessed:   3,
		Mode:             protocol.ModeSplit,
		HashValid:        &valid,
	}
	var buf bytes.Buffer
	if err := tr.Write(&buf); err != nil {
		t.Fatal(err)
	}
	got, err := protocol.ReadTrailer(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.BytesProcessed != 3 {
		t.Fatalf("bytes %d", got.BytesProcessed)
	}
}
