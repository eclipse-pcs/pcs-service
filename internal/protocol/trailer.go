package protocol

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/eclipse-pcs/pcs"
	"github.com/eclipse-pcs/pcs/footer"
	"github.com/eclipse-pcs/pcs/stream"
)

// TrailerKind keys used in JSON maps.
const (
	KindEC = "ec"
	KindOC = "oc"
	KindEN = "en"
	KindON = "on"
	KindCP = "cp"
	KindNP = "np"
)

// Trailer is sent on the control channel after stream data completes.
type Trailer struct {
	SecretSHA256     string            `json:"secret_sha256"`
	FingerprintShard map[string]string `json:"fingerprint_shard"`
	CrossCRC         map[string]uint32 `json:"cross_crc"`
	Length           uint64            `json:"length"`
	WriteID          string            `json:"write_id"`
	WriteIDValid     *bool             `json:"write_id_valid,omitempty"`
	Footer           map[string]string `json:"footer,omitempty"`
	BytesProcessed   int64             `json:"bytes_processed"`
	Mode             Mode              `json:"mode"`
	HashValid        *bool             `json:"hash_valid,omitempty"`
	Recoveries       []string          `json:"recoveries,omitempty"`
}

func kindKey(kind pcs.ParticleKind) string {
	switch kind {
	case pcs.EvenCypher:
		return KindEC
	case pcs.OddCypher:
		return KindOC
	case pcs.EvenNoise:
		return KindEN
	case pcs.OddNoise:
		return KindON
	case pcs.CypherParity:
		return KindCP
	case pcs.NoiseParity:
		return KindNP
	default:
		return kind.String()
	}
}

// KindFromKey maps trailer JSON keys to particle kinds.
func KindFromKey(key string) (pcs.ParticleKind, bool) {
	switch key {
	case KindEC:
		return pcs.EvenCypher, true
	case KindOC:
		return pcs.OddCypher, true
	case KindEN:
		return pcs.EvenNoise, true
	case KindON:
		return pcs.OddNoise, true
	case KindCP:
		return pcs.CypherParity, true
	case KindNP:
		return pcs.NoiseParity, true
	default:
		return 0, false
	}
}

// BuildTrailerFromEncodeMeta builds a split-mode trailer from streaming encode metadata.
func BuildTrailerFromEncodeMeta(mode Mode, meta *stream.EncodeMeta) *Trailer {
	t := &Trailer{
		SecretSHA256:     hex.EncodeToString(meta.SHA256[:]),
		FingerprintShard: make(map[string]string, 6),
		CrossCRC:         make(map[string]uint32, 6),
		Footer:           make(map[string]string, 6),
		BytesProcessed:   meta.BytesProcessed,
		Mode:             mode,
	}
	var writeID uint64
	for kind, f := range meta.Footers {
		key := kindKey(kind)
		t.FingerprintShard[key] = hex.EncodeToString(f.FingerprintShard[:])
		t.CrossCRC[key] = f.CrossCRC
		raw := f.Marshal()
		t.Footer[key] = base64.StdEncoding.EncodeToString(raw[:])
		if writeID == 0 {
			writeID = f.WriteID
			t.Length = f.Length
		}
	}
	t.WriteID = fmt.Sprintf("%016x", writeID)
	return t
}

// BuildTrailerFromDecodeMeta builds a merge-mode trailer from streaming decode metadata.
func BuildTrailerFromDecodeMeta(mode Mode, meta *stream.DecodeMeta, recoveries []string) *Trailer {
	t := &Trailer{
		SecretSHA256:     hex.EncodeToString(meta.SHA256[:]),
		FingerprintShard: make(map[string]string, 6),
		CrossCRC:         make(map[string]uint32, 6),
		BytesProcessed:   meta.BytesRead,
		Mode:             mode,
		Recoveries:       recoveries,
	}
	valid := true
	t.HashValid = &valid
	t.WriteIDValid = &valid
	var writeID uint64
	for kind, f := range meta.Footers {
		key := kindKey(kind)
		t.FingerprintShard[key] = hex.EncodeToString(f.FingerprintShard[:])
		t.CrossCRC[key] = f.CrossCRC
		if writeID == 0 {
			writeID = f.WriteID
			t.Length = f.Length
		}
	}
	t.WriteID = fmt.Sprintf("%016x", writeID)
	return t
}

func (t *Trailer) Write(w io.Writer) error {
	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal trailer: %w", err)
	}
	if _, err := fmt.Fprintf(w, "%d\n", len(data)); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// ReadTrailer reads the session trailer from the control channel.
func ReadTrailer(r io.Reader) (*Trailer, error) {
	br := bufio.NewReader(r)
	return readSessionLine(br)
}

func readTrailerBody(r io.Reader, length int) (*Trailer, error) {
	if length < 0 || length > 1<<20 {
		return nil, fmt.Errorf("invalid trailer length %d", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("read trailer body: %w", err)
	}
	var t Trailer
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("unmarshal trailer: %w", err)
	}
	return &t, nil
}

// VerifyWriteIDsFromTrailer checks footer WriteID agreement when footer map is present.
func (t *Trailer) VerifyWriteIDsFromTrailer() error {
	if len(t.Footer) == 0 {
		return nil
	}
	footers := make(map[pcs.ParticleKind]*footer.Footer)
	for key, b64 := range t.Footer {
		kind, ok := KindFromKey(key)
		if !ok {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return fmt.Errorf("decode footer %s: %w", key, err)
		}
		f, err := footer.Parse(raw)
		if err != nil {
			return fmt.Errorf("parse footer %s: %w", key, err)
		}
		footers[kind] = f
	}
	return footer.VerifyWriteIDs(footers)
}
