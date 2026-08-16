package particleio_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/eclipse-pcs/pcs"
	"github.com/eclipse-pcs/pcs/stream"

	"pcs-service/internal/particleio"
)

func TestTrailingFooterWithStreamDecoder(t *testing.T) {
	secret := []byte("decoder smoke test")
	particles, _, err := stream.EncodeCollect(secret, bytes.Repeat([]byte{0x55}, len(secret)), 7)
	if err != nil {
		t.Fatal(err)
	}
	sources := stream.Sources{
		EC: stream.Source{R: particleio.NewTrailingFooterReader(bytes.NewReader(particles[pcs.EvenCypher])), PayloadLen: -1},
		OC: stream.Source{R: particleio.NewTrailingFooterReader(bytes.NewReader(particles[pcs.OddCypher])), PayloadLen: -1},
		EN: stream.Source{R: particleio.NewTrailingFooterReader(bytes.NewReader(particles[pcs.EvenNoise])), PayloadLen: -1},
		ON: stream.Source{R: particleio.NewTrailingFooterReader(bytes.NewReader(particles[pcs.OddNoise])), PayloadLen: -1},
		CP: stream.Source{R: particleio.NewTrailingFooterReader(bytes.NewReader(particles[pcs.CypherParity])), PayloadLen: -1},
		NP: stream.Source{R: particleio.NewTrailingFooterReader(bytes.NewReader(particles[pcs.NoiseParity])), PayloadLen: -1},
	}
	var out bytes.Buffer
	_, err = stream.NewDecoder(7).Decode(sources, &out, stream.DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secret, out.Bytes()) {
		t.Fatalf("got %q", out.Bytes())
	}
}

func TestTrailingFooterReaderReadAllThenFooter(t *testing.T) {
	secret := []byte("x")
	particles, _, err := stream.EncodeCollect(secret, bytes.Repeat([]byte{1}, len(secret)), 7)
	if err != nil {
		t.Fatal(err)
	}
	data := particles[pcs.EvenNoise]
	r := particleio.NewTrailingFooterReader(bytes.NewReader(data))
	payload, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	footerBuf := make([]byte, 64)
	if _, err := io.ReadFull(r, footerBuf); err != nil {
		t.Fatalf("footer: %v len(payload)=%d len(data)=%d", err, len(payload), len(data))
	}
}
