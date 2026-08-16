package particleio_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/eclipse-pcs/pcs/footer"
	"github.com/eclipse-pcs/pcs/stream"

	"pcs-service/internal/particleio"
)

func TestTrailingFooterReaderRoundTrip(t *testing.T) {
	secret := []byte("trailing footer split test payload")
	particles, meta, err := stream.EncodeCollect(secret, bytes.Repeat([]byte{0x42}, len(secret)), 7)
	if err != nil {
		t.Fatal(err)
	}
	for kind, data := range particles {
		r := particleio.NewTrailingFooterReader(bytes.NewReader(data))
		payload, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("%s payload: %v", kind, err)
		}
		footerBytes := make([]byte, footer.Size)
		if _, err := io.ReadFull(r, footerBytes); err != nil {
			t.Fatalf("%s footer read: %v", kind, err)
		}
		if len(payload)+footer.Size != len(data) {
			t.Fatalf("%s payload len %d want %d", kind, len(payload), len(data)-footer.Size)
		}
		gotFooter, err := footer.Parse(footerBytes)
		if err != nil {
			t.Fatal(err)
		}
		wantFooter := meta.Footers[kind]
		if gotFooter.WriteID != wantFooter.WriteID {
			t.Fatalf("WriteID mismatch on %s", kind)
		}
	}
}

func TestTrailingFooterReaderTooShort(t *testing.T) {
	r := particleio.NewTrailingFooterReader(bytes.NewReader([]byte("short")))
	_, err := io.ReadAll(r)
	if err == nil {
		t.Fatal("expected error for short stream")
	}
}
