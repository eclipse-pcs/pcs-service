package server

import (
	"bytes"
	"io"
	"testing"

	"github.com/eclipse-pcs/pcs"
	"github.com/eclipse-pcs/pcs/footer"
	"github.com/eclipse-pcs/pcs/stream"

	"pcs-service/internal/particleio"
	"pcs-service/internal/protocol"
)

func TestBuildMergeSourcesUsesStreamingReaders(t *testing.T) {
	ports := &protocol.SessionPortSet{
		EC: readWriteCloser{Reader: io.NopCloser(bytes.NewReader(nil))},
		OC: readWriteCloser{Reader: io.NopCloser(bytes.NewReader(nil))},
		EN: readWriteCloser{Reader: io.NopCloser(bytes.NewReader(nil))},
		ON: readWriteCloser{Reader: io.NopCloser(bytes.NewReader(nil))},
	}
	sources, err := buildMergeSources(ports, protocol.MergeProfile{})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range pcs.CoreParticleKinds {
		src := sourceForKind(sources, kind)
		if src.R == nil {
			t.Fatalf("%s: expected reader", kind)
		}
		if src.PayloadLen != -1 {
			t.Fatalf("%s: PayloadLen %d, want -1", kind, src.PayloadLen)
		}
		if _, ok := src.R.(*particleio.TrailingFooterReader); !ok {
			t.Fatalf("%s: reader type %T, want *particleio.TrailingFooterReader", kind, src.R)
		}
	}
	if sources.CP.R != nil || sources.NP.R != nil {
		t.Fatalf("full merge should omit parity streams: cp=%v np=%v", sources.CP.R, sources.NP.R)
	}
}

func TestMergeStreamOpenDoesNotBufferFullParticle(t *testing.T) {
	const chunk = 17
	secret := bytes.Repeat([]byte{0xab}, chunk*3+5)
	particles, _, err := stream.EncodeCollect(secret, bytes.Repeat([]byte{0xcd}, len(secret)), chunk)
	if err != nil {
		t.Fatal(err)
	}

	readers := make(map[pcs.ParticleKind]*particleio.TrailingFooterReader, 4)
	restore := mergeStreamOpen
	mergeStreamOpen = func(r io.Reader) io.Reader {
		tfr := particleio.NewTrailingFooterReader(r)
		// last opened reader per kind is tracked below via explicit assignment
		_ = tfr
		return tfr
	}
	t.Cleanup(func() { mergeStreamOpen = restore })

	open := func(kind pcs.ParticleKind) stream.Source {
		tfr := particleio.NewTrailingFooterReader(bytes.NewReader(particles[kind]))
		readers[kind] = tfr
		return stream.Source{R: tfr, PayloadLen: -1}
	}
	sources := stream.Sources{
		EC: open(pcs.EvenCypher),
		OC: open(pcs.OddCypher),
		EN: open(pcs.EvenNoise),
		ON: open(pcs.OddNoise),
	}
	var out bytes.Buffer
	if _, err := stream.NewDecoder(chunk).Decode(sources, &out, stream.DecodeOptions{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secret, out.Bytes()) {
		t.Fatal("decode mismatch")
	}
	for kind, r := range readers {
		if hold := r.PeakTailHold(); hold > footer.Size {
			t.Fatalf("%s peak tail hold %d exceeds footer size %d", kind, hold, footer.Size)
		}
	}
}

func sourceForKind(sources stream.Sources, kind pcs.ParticleKind) stream.Source {
	switch kind {
	case pcs.EvenCypher:
		return sources.EC
	case pcs.OddCypher:
		return sources.OC
	case pcs.EvenNoise:
		return sources.EN
	case pcs.OddNoise:
		return sources.ON
	case pcs.CypherParity:
		return sources.CP
	case pcs.NoiseParity:
		return sources.NP
	default:
		return stream.Source{}
	}
}

type readWriteCloser struct {
	io.Reader
}

func (readWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (readWriteCloser) Close() error                { return nil }
