package server

import (
	"fmt"
	"io"

	"github.com/eclipse-pcs/pcs"
	"github.com/eclipse-pcs/pcs/stream"

	"pcs-service/internal/mergeplan"
	"pcs-service/internal/particleio"
	"pcs-service/internal/protocol"
)

// mergeStreamOpen wraps an incoming particle TCP stream for streaming merge decode.
// Tests may replace this hook to assert the merge path stays streaming.
var mergeStreamOpen = func(r io.Reader) io.Reader {
	return particleio.NewTrailingFooterReader(r)
}

func mergeSendPlan(profile protocol.MergeProfile) (mergeplan.MergeSendPlan, error) {
	inv := pcs.NewParticleInventory(map[pcs.ParticleKind]bool{
		pcs.EvenCypher: true, pcs.OddCypher: true,
		pcs.EvenNoise: true, pcs.OddNoise: true,
		pcs.CypherParity: true, pcs.NoiseParity: true,
	})
	return mergeplan.MergeSendPlanFrom(inv, profile.MissingMap())
}

func buildMergeSources(ports *protocol.SessionPortSet, profile protocol.MergeProfile) (stream.Sources, error) {
	plan, err := mergeSendPlan(profile)
	if err != nil {
		return stream.Sources{}, err
	}
	return stream.Sources{
		EC: mergeSource(ports.EC, plan.Send[pcs.EvenCypher]),
		OC: mergeSource(ports.OC, plan.Send[pcs.OddCypher]),
		EN: mergeSource(ports.EN, plan.Send[pcs.EvenNoise]),
		ON: mergeSource(ports.ON, plan.Send[pcs.OddNoise]),
		CP: mergeSource(ports.CP, plan.Send[pcs.CypherParity]),
		NP: mergeSource(ports.NP, plan.Send[pcs.NoiseParity]),
	}, nil
}

func mergeSource(r io.Reader, send bool) stream.Source {
	if r == nil || !send {
		return stream.Source{}
	}
	return stream.Source{R: mergeStreamOpen(r), PayloadLen: -1}
}

func limitedDocWriter(w io.Writer, maxSize int64) io.Writer {
	if maxSize <= 0 {
		return w
	}
	return &maxSizeWriter{w: w, max: maxSize}
}

type maxSizeWriter struct {
	w     io.Writer
	max   int64
	wrote int64
}

func (m *maxSizeWriter) Write(p []byte) (int, error) {
	if m.wrote >= m.max {
		return 0, fmt.Errorf("object exceeds max size %d", m.max)
	}
	remain := m.max - m.wrote
	if int64(len(p)) > remain {
		p = p[:remain]
	}
	n, err := m.w.Write(p)
	m.wrote += int64(n)
	if m.wrote > m.max {
		return n, fmt.Errorf("object exceeds max size %d", m.max)
	}
	return n, err
}
