package server

import (
	"fmt"
	"io"
	"net"

	"github.com/eclipse-pcs/pcs/stream"

	"pcs-service/internal/config"
	"pcs-service/internal/protocol"
)

func handleSplit(comConn io.ReadWriter, ports *protocol.SessionPortSet, cfg *config.Config) (sessionStats, error) {
	defer ports.Close()
	stats := sessionStats{mode: protocol.ModeSplit}
	doc := protocol.NewStripTokenReader(ports.Doc, cfg.Token)
	n, err := splitStreaming(comConn, doc, ports, cfg)
	stats.bytes = n
	return stats, err
}

func splitStreaming(comConn io.Writer, doc io.Reader, ports *protocol.SessionPortSet, cfg *config.Config) (int64, error) {
	doc = limitedDocReader(doc, cfg.MaxObjectSize)
	enc := stream.NewEncoder(cfg.ChunkSize)
	meta, err := enc.Encode(doc, stream.Writers{
		EC: ports.EC, OC: ports.OC, EN: ports.EN, ON: ports.ON, CP: ports.CP, NP: ports.NP,
	}, stream.EncodeOptions{})
	if err != nil {
		return 0, fmt.Errorf("stream encode: %w", err)
	}
	closeWriteParticlePorts(ports)
	trailer := protocol.BuildTrailerFromEncodeMeta(protocol.ModeSplit, meta)
	if err := trailer.Write(comConn); err != nil {
		return 0, err
	}
	return meta.BytesProcessed, nil
}

func handleMerge(comConn io.ReadWriter, ports *protocol.SessionPortSet, cfg *config.Config, profile protocol.MergeProfile) (sessionStats, error) {
	defer ports.Close()
	return mergeStreaming(comConn, ports, cfg, profile)
}

func mergeStreaming(comConn io.Writer, ports *protocol.SessionPortSet, cfg *config.Config, profile protocol.MergeProfile) (sessionStats, error) {
	stats := sessionStats{mode: protocol.ModeMerge}
	if profile.MissingMask != 0 {
		stats.recoveries = []string{"parity recovery used"}
	}
	sources, err := buildMergeSources(ports, profile)
	if err != nil {
		return stats, err
	}
	dec := stream.NewDecoder(cfg.ChunkSize)
	docOut := limitedDocWriter(ports.Doc, cfg.MaxObjectSize)
	meta, err := dec.Decode(sources, docOut, stream.DecodeOptions{})
	if err != nil {
		return stats, fmt.Errorf("stream decode: %w", err)
	}
	stats.bytes = meta.BytesRead
	valid := true
	stats.hashValid = &valid
	if tc, ok := ports.Doc.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
	trailer := protocol.BuildTrailerFromDecodeMeta(protocol.ModeMerge, meta, stats.recoveries)
	if err := trailer.Write(comConn); err != nil {
		return stats, err
	}
	return stats, nil
}

// limitedDocReader wraps r with a size cap when maxSize > 0. maxSize 0 means unlimited.
func limitedDocReader(r io.Reader, maxSize int64) io.Reader {
	if maxSize <= 0 {
		return r
	}
	return &maxSizeReader{r: r, max: maxSize}
}

type maxSizeReader struct {
	r      io.Reader
	max    int64
	read   int64
	exceed bool
}

func (m *maxSizeReader) Read(p []byte) (int, error) {
	if m.exceed {
		return 0, fmt.Errorf("object exceeds max size %d", m.max)
	}
	if m.read >= m.max {
		m.exceed = true
		return 0, fmt.Errorf("object exceeds max size %d", m.max)
	}
	remain := m.max - m.read
	if int64(len(p)) > remain+1 {
		p = p[:remain+1]
	}
	n, err := m.r.Read(p)
	m.read += int64(n)
	if m.read > m.max {
		return n, fmt.Errorf("object exceeds max size %d", m.max)
	}
	return n, err
}
