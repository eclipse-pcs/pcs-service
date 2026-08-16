package client

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"github.com/eclipse-pcs/pcs"

	"github.com/eclipse-pcs/pcs-service/internal/mergeplan"
	"github.com/eclipse-pcs/pcs-service/internal/protocol"
	"github.com/eclipse-pcs/pcs-service/internal/store"
)

var coreParticleKinds = pcs.CoreParticleKinds

var parityParticleKinds = []pcs.ParticleKind{
	pcs.CypherParity, pcs.NoiseParity,
}

// PrepareMergeFromFiles scans on-disk particles and returns the merge profile mask.
func PrepareMergeFromFiles(dir, base string) (protocol.MergeProfile, error) {
	inv, err := store.ScanInventory(dir, base)
	if err != nil {
		return protocol.MergeProfile{}, err
	}
	return protocol.MergeProfileFromInventory(inv), nil
}

// PrepareMergeSendPlan scans particles and returns a validated send plan for the profile.
func PrepareMergeSendPlan(dir, base string, profile protocol.MergeProfile) (mergeplan.MergeSendPlan, error) {
	inv, err := store.ScanInventory(dir, base)
	if err != nil {
		return mergeplan.MergeSendPlan{}, err
	}
	return mergeplan.MergeSendPlanFrom(inv.PCSInventory(), profile.MissingMap())
}

// MergeSession uploads in-memory particles and returns the reconstructed secret.
func MergeSession(cfg *Config, core map[pcs.ParticleKind][]byte) ([]byte, *protocol.Trailer, error) {
	profile := mergeProfileFromCore(core)
	plan, err := mergeplan.MergeSendPlanFromCore(core, profile.MissingMap())
	if err != nil {
		return nil, nil, err
	}
	sess, err := cfg.OpenMergeSession(profile)
	if err != nil {
		return nil, nil, err
	}
	defer sess.Close()

	var secret bytes.Buffer
	trailer, err := mergeSessionToWriter(sess, func() error {
		return UploadParticlesMemory(sess, core, plan)
	}, &secret)
	if err != nil {
		return nil, nil, err
	}
	return secret.Bytes(), trailer, nil
}

// MergeFromFiles runs a file-based merge and streams the reconstructed document to out.
func MergeFromFiles(cfg *Config, dir, base string, profile protocol.MergeProfile, out io.Writer) (*protocol.Trailer, error) {
	inv, err := store.ScanInventory(dir, base)
	if err != nil {
		return nil, err
	}
	plan, err := mergeplan.MergeSendPlanFrom(inv.PCSInventory(), profile.MissingMap())
	if err != nil {
		return nil, err
	}
	sess, err := cfg.OpenMergeSession(profile)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	return mergeSessionToWriter(sess, func() error {
		return uploadParticlesFromFiles(sess, dir, base, plan)
	}, out)
}

func mergeSessionToWriter(sess *Session, upload func() error, out io.Writer) (*protocol.Trailer, error) {
	var wg sync.WaitGroup
	var uploadErr, readErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		uploadErr = upload()
	}()
	go func() {
		defer wg.Done()
		_, readErr = CopyBuffer(out, sess.Data.Doc)
	}()
	wg.Wait()
	if uploadErr != nil {
		return nil, uploadErr
	}
	if readErr != nil {
		return nil, readErr
	}
	return ReadTrailer(sess)
}

func mergeProfileFromCore(core map[pcs.ParticleKind][]byte) protocol.MergeProfile {
	var mask uint8
	for _, item := range []struct {
		kind pcs.ParticleKind
		bit  uint8
	}{
		{pcs.EvenCypher, protocol.MissingEC},
		{pcs.OddCypher, protocol.MissingOC},
		{pcs.EvenNoise, protocol.MissingEN},
		{pcs.OddNoise, protocol.MissingON},
	} {
		if _, ok := core[item.kind]; !ok {
			mask |= item.bit
		}
	}
	return protocol.MergeProfile{MissingMask: mask}
}

// UploadParticlesMemory writes complete particle files according to plan and closes each stream.
func UploadParticlesMemory(sess *Session, core map[pcs.ParticleKind][]byte, plan mergeplan.MergeSendPlan) error {
	writers := particleWriters(sess.Data)
	upload := func(kind pcs.ParticleKind) error {
		if !plan.Send[kind] {
			return nil
		}
		data := core[kind]
		if data == nil {
			data = []byte{}
		}
		if _, err := writers[kind].Write(data); err != nil {
			return fmt.Errorf("write %s: %w", kind, err)
		}
		if err := writers[kind].Close(); err != nil {
			return fmt.Errorf("close %s: %w", kind, err)
		}
		return nil
	}
	return uploadWithPlan(sess, plan, upload)
}

func uploadWithPlan(sess *Session, plan mergeplan.MergeSendPlan, upload func(kind pcs.ParticleKind) error) error {
	if err := closeOmittedParticleStreams(sess, plan); err != nil {
		return err
	}
	return uploadParticlePhases(plan, upload)
}

func closeOmittedParticleStreams(sess *Session, plan mergeplan.MergeSendPlan) error {
	writers := particleWriters(sess.Data)
	for _, kind := range append(append([]pcs.ParticleKind{}, coreParticleKinds...), parityParticleKinds...) {
		if plan.Send[kind] {
			continue
		}
		if err := writers[kind].Close(); err != nil {
			return fmt.Errorf("close %s: %w", kind, err)
		}
	}
	return nil
}

func uploadParticlePhases(plan mergeplan.MergeSendPlan, upload func(kind pcs.ParticleKind) error) error {
	runKinds := func(kinds []pcs.ParticleKind) error {
		var wg sync.WaitGroup
		errCh := make(chan error, len(kinds))
		for _, kind := range kinds {
			if !plan.Send[kind] {
				continue
			}
			wg.Add(1)
			go func(kind pcs.ParticleKind) {
				defer wg.Done()
				if err := upload(kind); err != nil {
					errCh <- err
				}
			}(kind)
		}
		wg.Wait()
		select {
		case err := <-errCh:
			return err
		default:
			return nil
		}
	}
	if err := runKinds(coreParticleKinds); err != nil {
		return err
	}
	return runKinds(parityParticleKinds)
}

func uploadParticlesFromFiles(sess *Session, dir, base string, plan mergeplan.MergeSendPlan) error {
	writers := particleWriters(sess.Data)
	upload := func(kind pcs.ParticleKind) error {
		if !plan.Send[kind] {
			return nil
		}
		path := store.ParticlePath(dir, base, kind)
		r, err := store.OpenParticleFile(path)
		if err != nil {
			return err
		}
		if _, err := CopyBuffer(writers[kind], r); err != nil {
			r.Close()
			return fmt.Errorf("write %s: %w", kind, err)
		}
		if err := r.Close(); err != nil {
			return err
		}
		if err := writers[kind].Close(); err != nil {
			return fmt.Errorf("close %s: %w", kind, err)
		}
		return nil
	}
	return uploadWithPlan(sess, plan, upload)
}

// UploadParticlesStreaming sends on-disk particles using inventory-derived profile.
func UploadParticlesStreaming(sess *Session, dir, base string) error {
	inv, err := store.ScanInventory(dir, base)
	if err != nil {
		return err
	}
	profile := protocol.MergeProfileFromInventory(inv)
	plan, err := mergeplan.MergeSendPlanFrom(inv.PCSInventory(), profile.MissingMap())
	if err != nil {
		return err
	}
	return uploadParticlesFromFiles(sess, dir, base, plan)
}

// UploadMergeFromFiles streams on-disk particles using inventory-derived profile.
func UploadMergeFromFiles(sess *Session, dir, base string, profile protocol.MergeProfile) error {
	inv, err := store.ScanInventory(dir, base)
	if err != nil {
		return err
	}
	plan, err := mergeplan.MergeSendPlanFrom(inv.PCSInventory(), profile.MissingMap())
	if err != nil {
		return err
	}
	return uploadParticlesFromFiles(sess, dir, base, plan)
}

// SplitCSV splits a comma-separated skip list (e.g. "ec,oc").
func SplitCSV(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := s[start:i]
			if part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}

// ConfigFromAddr builds a client config from a host:port address and token.
func ConfigFromAddr(addr, token string) (*Config, error) {
	host, portStr, err := netSplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := parsePort(portStr)
	if err != nil {
		return nil, err
	}
	return &Config{Host: host, Port: port, Token: token}, nil
}
