package particleio

import (
	"fmt"
	"io"

	"github.com/eclipse-pcs/pcs/footer"
)

// TrailingFooterReader streams payload bytes; the trailing 64-byte footer is read only after payload EOF.
type TrailingFooterReader struct {
	r           io.Reader
	tail        []byte
	payloadDone bool
	footPhase   bool
	footOff     int
	peakTail    int
}

// NewTrailingFooterReader wraps r so payload reads exclude the fixed footer suffix.
func NewTrailingFooterReader(r io.Reader) *TrailingFooterReader {
	return &TrailingFooterReader{r: r}
}

// Read returns payload bytes until EOF. Footer bytes are served only after payloadDone on later reads.
func (t *TrailingFooterReader) Read(p []byte) (int, error) {
	if t.footPhase {
		if t.footOff >= footer.Size {
			return 0, io.EOF
		}
		n := copy(p, t.tail[t.footOff:footer.Size])
		t.footOff += n
		if t.footOff >= footer.Size {
			return n, io.EOF
		}
		return n, nil
	}
	if t.payloadDone {
		t.footPhase = true
		return t.Read(p)
	}

	n, err := t.r.Read(p)
	if n > 0 {
		combined := append(append([]byte(nil), t.tail...), p[:n]...)
		if len(combined) <= footer.Size {
			t.tail = combined
			t.noteTail()
			if err == io.EOF {
				if len(t.tail) < footer.Size {
					return 0, fmt.Errorf("stream shorter than footer (%d bytes)", len(t.tail))
				}
				t.payloadDone = true
				return 0, io.EOF
			}
			return 0, nil
		}
		emitLen := len(combined) - footer.Size
		copy(p, combined[:emitLen])
		t.tail = append([]byte(nil), combined[emitLen:]...)
		t.noteTail()
		if err == io.EOF {
			t.payloadDone = true
			return emitLen, io.EOF
		}
		return emitLen, nil
	}
	if err == io.EOF {
		if len(t.tail) < footer.Size {
			return 0, fmt.Errorf("stream shorter than footer (%d bytes)", len(t.tail))
		}
		t.payloadDone = true
		return 0, io.EOF
	}
	return n, err
}

func (t *TrailingFooterReader) noteTail() {
	if n := len(t.tail); n > t.peakTail {
		t.peakTail = n
	}
}

// PeakTailHold reports the maximum payload look-behind size observed (≤ footer.Size).
func (t *TrailingFooterReader) PeakTailHold() int {
	return t.peakTail
}

// ParsedFooter returns the footer after payload and footer bytes have been read from this reader.
func (t *TrailingFooterReader) ParsedFooter() (*footer.Footer, error) {
	if !t.footPhase || t.footOff < footer.Size {
		return nil, fmt.Errorf("footer not available yet")
	}
	return footer.Parse(t.tail)
}
