package particleio_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/eclipse-pcs/pcs"
	"github.com/eclipse-pcs/pcs/footer"
	"github.com/eclipse-pcs/pcs/stream"

	"pcs-service/internal/particleio"
)

type streamBuffer struct {
	r           io.Reader
	buf         []byte
	readScratch []byte
	eof         bool
}

func newStreamBuffer(r io.Reader) *streamBuffer {
	return &streamBuffer{r: r}
}

func (s *streamBuffer) len() int { return len(s.buf) }

func (s *streamBuffer) fill() error {
	if s.eof {
		return nil
	}
	if cap(s.readScratch) < 4096 {
		s.readScratch = make([]byte, 4096)
	}
	n, err := s.r.Read(s.readScratch)
	if n > 0 {
		s.buf = append(s.buf, s.readScratch[:n]...)
	}
	if err == io.EOF {
		s.eof = true
		return nil
	}
	return err
}

func (s *streamBuffer) take(n int) []byte {
	if n <= 0 {
		return nil
	}
	if n > len(s.buf) {
		n = len(s.buf)
	}
	out := s.buf[:n]
	s.buf = s.buf[n:]
	return out
}

func (s *streamBuffer) drainAll() error {
	for !s.eof || len(s.buf) > 0 {
		if len(s.buf) == 0 {
			if err := s.fill(); err != nil {
				return err
			}
			if len(s.buf) == 0 {
				return nil
			}
		}
		s.take(len(s.buf))
	}
	return nil
}

func TestStreamBufferThenFooterEachKind(t *testing.T) {
	secret := []byte("decoder smoke test")
	particles, _, err := stream.EncodeCollect(secret, bytes.Repeat([]byte{0x55}, len(secret)), 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range pcs.AllParticleKinds {
		t.Run(kind.String(), func(t *testing.T) {
			r := particleio.NewTrailingFooterReader(bytes.NewReader(particles[kind]))
			sb := newStreamBuffer(r)
			if err := sb.drainAll(); err != nil {
				t.Fatalf("drain: %v", err)
			}
			footerBuf := make([]byte, footer.Size)
			if _, err := io.ReadFull(r, footerBuf); err != nil {
				t.Fatalf("footer after drain: %v", err)
			}
		})
	}
}

func TestStreamBufferInterleavedLikeDecoder(t *testing.T) {
	secret := []byte("decoder smoke test")
	particles, _, err := stream.EncodeCollect(secret, bytes.Repeat([]byte{0x55}, len(secret)), 7)
	if err != nil {
		t.Fatal(err)
	}
	type slot struct {
		r io.Reader
		sb *streamBuffer
	}
	slots := make(map[pcs.ParticleKind]*slot, len(pcs.AllParticleKinds))
	for _, kind := range pcs.AllParticleKinds {
		r := particleio.NewTrailingFooterReader(bytes.NewReader(particles[kind]))
		slots[kind] = &slot{r: r, sb: newStreamBuffer(r)}
	}
	evenEC, oddOC := slots[pcs.EvenCypher].sb, slots[pcs.OddCypher].sb
	evenEN, oddON := slots[pcs.EvenNoise].sb, slots[pcs.OddNoise].sb
	for {
		if evenEC.eof && oddOC.eof {
			break
		}
		for _, sb := range []*streamBuffer{evenEC, oddOC, evenEN, oddON} {
			if sb.eof && sb.len() == 0 {
				continue
			}
			if sb.len() == 0 {
				if err := sb.fill(); err != nil {
					t.Fatal(err)
				}
			}
			if sb.len() > 0 {
				sb.take(1)
			}
		}
	}
	for _, kind := range []pcs.ParticleKind{pcs.CypherParity, pcs.NoiseParity} {
		if err := slots[kind].sb.drainAll(); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range pcs.AllParticleKinds {
		footerBuf := make([]byte, footer.Size)
		if _, err := io.ReadFull(slots[kind].r, footerBuf); err != nil {
			t.Fatalf("%s footer: %v", kind, err)
		}
	}
}
