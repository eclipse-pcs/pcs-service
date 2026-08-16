package client

import (
	"io"
	"sync"

	"github.com/eclipse-pcs/pcs"

	"pcs-service/internal/protocol"
)

func particleReaders(sess *protocol.SessionPortSet) map[pcs.ParticleKind]io.ReadCloser {
	return map[pcs.ParticleKind]io.ReadCloser{
		pcs.EvenCypher:   sess.EC,
		pcs.OddCypher:    sess.OC,
		pcs.EvenNoise:    sess.EN,
		pcs.OddNoise:     sess.ON,
		pcs.CypherParity: sess.CP,
		pcs.NoiseParity:  sess.NP,
	}
}

func particleWriters(sess *protocol.SessionPortSet) map[pcs.ParticleKind]io.WriteCloser {
	return map[pcs.ParticleKind]io.WriteCloser{
		pcs.EvenCypher:   sess.EC,
		pcs.OddCypher:    sess.OC,
		pcs.EvenNoise:    sess.EN,
		pcs.OddNoise:     sess.ON,
		pcs.CypherParity: sess.CP,
		pcs.NoiseParity:  sess.NP,
	}
}

func copyParticleStreams(sess *protocol.SessionPortSet, dst map[pcs.ParticleKind]io.Writer) error {
	readers := particleReaders(sess)
	var wg sync.WaitGroup
	errCh := make(chan error, len(readers))
	for kind, r := range readers {
		w, ok := dst[kind]
		if !ok {
			continue
		}
		wg.Add(1)
		go func(kind pcs.ParticleKind, r io.ReadCloser, w io.Writer) {
			defer wg.Done()
			_, err := CopyBuffer(w, r)
			_ = r.Close()
			if err != nil {
				errCh <- err
			}
		}(kind, r, w)
	}
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// SplitResult holds in-memory particle streams from a split session.
type SplitResult struct {
	Particles map[pcs.ParticleKind][]byte
	Trailer   *protocol.Trailer
}

// SplitSession runs a split and collects particles in memory.
func SplitSession(cfg *Config, secret []byte) (*SplitResult, error) {
	sess, err := cfg.OpenSession(protocol.ModeSplit)
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	type chunk struct {
		kind pcs.ParticleKind
		data []byte
		err  error
	}
	ch := make(chan chunk, 6)
	for kind, r := range particleReaders(sess.Data) {
		go func(kind pcs.ParticleKind, r io.Reader) {
			data, err := io.ReadAll(r)
			ch <- chunk{kind, data, err}
		}(kind, r)
	}
	if _, err := sess.Data.Doc.Write(secret); err != nil {
		return nil, err
	}
	if err := sess.Data.Doc.Close(); err != nil {
		return nil, err
	}
	core := make(map[pcs.ParticleKind][]byte, 6)
	for i := 0; i < 6; i++ {
		c := <-ch
		if c.err != nil {
			return nil, c.err
		}
		core[c.kind] = c.data
	}
	trailer, err := ReadTrailer(sess)
	if err != nil {
		return nil, err
	}
	return &SplitResult{Particles: core, Trailer: trailer}, nil
}
