package protocol

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/eclipse-pcs/pcs"

	"pcs-service/internal/store"
)

// Missing-core mask bits for merge profile (control channel line 2).
const (
	MissingEC uint8 = 1 << iota
	MissingOC
	MissingEN
	MissingON
)

// MergeProfile declares which core particle streams the client will not send.
type MergeProfile struct {
	MissingMask uint8
}

// MissingKinds returns particle kinds marked absent in the profile.
func (p MergeProfile) MissingKinds() []pcs.ParticleKind {
	var out []pcs.ParticleKind
	if p.MissingMask&MissingEC != 0 {
		out = append(out, pcs.EvenCypher)
	}
	if p.MissingMask&MissingOC != 0 {
		out = append(out, pcs.OddCypher)
	}
	if p.MissingMask&MissingEN != 0 {
		out = append(out, pcs.EvenNoise)
	}
	if p.MissingMask&MissingON != 0 {
		out = append(out, pcs.OddNoise)
	}
	return out
}

// MissingMap returns a bool map for omitted core streams.
func (p MergeProfile) MissingMap() map[pcs.ParticleKind]bool {
	m := make(map[pcs.ParticleKind]bool)
	for _, kind := range p.MissingKinds() {
		m[kind] = true
	}
	return m
}

// FormatProfileLine returns the merge profile control-channel line.
func (p MergeProfile) FormatProfileLine() string {
	return fmt.Sprintf("profile %x\n", p.MissingMask)
}

// ParseProfileLine parses "profile <hex-mask>".
func ParseProfileLine(line string) (MergeProfile, error) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) != 2 || fields[0] != "profile" {
		return MergeProfile{}, fmt.Errorf("invalid profile line %q", line)
	}
	mask, err := strconv.ParseUint(fields[1], 16, 8)
	if err != nil {
		return MergeProfile{}, fmt.Errorf("parse profile mask %q: %w", fields[1], err)
	}
	if mask&^uint64(MissingEC|MissingOC|MissingEN|MissingON) != 0 {
		return MergeProfile{}, fmt.Errorf("profile mask %#x has unknown bits", mask)
	}
	return MergeProfile{MissingMask: uint8(mask)}, nil
}

// ReadMergeProfile reads the required profile line after merge mode.
func ReadMergeProfile(r io.Reader) (MergeProfile, error) {
	br := bufio.NewReader(r)
	line, err := br.ReadString('\n')
	if err != nil {
		return MergeProfile{}, fmt.Errorf("read profile line: %w", err)
	}
	return ParseProfileLine(line)
}

// WriteMergeProfile writes the profile line for merge sessions.
func WriteMergeProfile(w io.Writer, p MergeProfile) error {
	_, err := io.WriteString(w, p.FormatProfileLine())
	return err
}

// MergeProfileFromInventory builds a mask from a scanned particle inventory.
func MergeProfileFromInventory(inv *store.Inventory) MergeProfile {
	var mask uint8
	for _, kind := range inv.MissingCoreParticles() {
		switch kind {
		case pcs.EvenCypher:
			mask |= MissingEC
		case pcs.OddCypher:
			mask |= MissingOC
		case pcs.EvenNoise:
			mask |= MissingEN
		case pcs.OddNoise:
			mask |= MissingON
		}
	}
	return MergeProfile{MissingMask: mask}
}

// MergeProfileFromSkipKinds builds a mask from explicit skip keys (e.g. --skip ec,oc).
func MergeProfileFromSkipKinds(skipped []string) (MergeProfile, error) {
	var mask uint8
	for _, key := range skipped {
		switch key {
		case "ec":
			mask |= MissingEC
		case "oc":
			mask |= MissingOC
		case "en":
			mask |= MissingEN
		case "on":
			mask |= MissingON
		default:
			return MergeProfile{}, fmt.Errorf("unknown skip kind %q", key)
		}
	}
	return MergeProfile{MissingMask: mask}, nil
}

// Validate checks the profile declares a recoverable merge.
func (p MergeProfile) Validate() error {
	if p.MissingMask&MissingEC != 0 && p.MissingMask&MissingOC != 0 {
		return fmt.Errorf("both cypher core particles missing")
	}
	if p.MissingMask&MissingEN != 0 && p.MissingMask&MissingON != 0 {
		return fmt.Errorf("both noise core particles missing")
	}
	return nil
}
