package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/eclipse-pcs/pcs"
)

// Inventory tracks particle presence for one logical object on disk.
type Inventory struct {
	MissingStorages []string
	Present         map[pcs.ParticleKind]bool
	Missing         []pcs.ParticleKind
}

func (inv *Inventory) MissingCoreParticles() []pcs.ParticleKind {
	var missing []pcs.ParticleKind
	for _, kind := range pcs.CoreParticleKinds {
		if !inv.Present[kind] {
			missing = append(missing, kind)
		}
	}
	return missing
}

func (inv *Inventory) NeedsParityRecovery() bool {
	return len(inv.MissingCoreParticles()) > 0
}

// PCSInventory converts to the shared module inventory type.
func (inv *Inventory) PCSInventory() *pcs.ParticleInventory {
	return pcs.NewParticleInventory(inv.Present)
}

// ScanInventory scans storage folders for the six unified particle files.
func ScanInventory(baseDir, baseName string) (*Inventory, error) {
	inv := &Inventory{Present: make(map[pcs.ParticleKind]bool)}

	for _, storage := range []string{pcs.StorageA, pcs.StorageB, pcs.StorageC} {
		path := filepath.Join(baseDir, storage)
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				inv.MissingStorages = append(inv.MissingStorages, storage)
				continue
			}
			return nil, fmt.Errorf("stat storage directory %s: %w", path, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("storage path is not a directory: %s", path)
		}
	}

	if len(inv.MissingStorages) >= 2 {
		return nil, fmt.Errorf("too many missing storage folders: %v", inv.MissingStorages)
	}

	for _, kind := range pcs.AllParticleKinds {
		path := ParticlePath(baseDir, baseName, kind)
		_, err := os.Stat(path)
		if err == nil {
			inv.Present[kind] = true
			continue
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat particle file %s: %w", path, err)
		}
		if kind == pcs.EvenCypher || kind == pcs.OddCypher || kind == pcs.EvenNoise || kind == pcs.OddNoise {
			inv.Missing = append(inv.Missing, kind)
		}
	}

	if inv.NeedsParityRecovery() {
		if !inv.Present[pcs.CypherParity] || !inv.Present[pcs.NoiseParity] {
			return nil, fmt.Errorf("parity particles required for recovery but not found")
		}
	}

	return inv, nil
}
