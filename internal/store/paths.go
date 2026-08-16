package store

import (
	"os"
	"path/filepath"

	"github.com/eclipse-pcs/pcs"
	"github.com/eclipse-pcs/pcs/footer"
)

// MinParticleFileSize is the smallest valid on-disk particle (empty payload + footer).
const MinParticleFileSize = footer.Size

// ParticleRelPath returns the storage-relative path for a particle kind.
func ParticleRelPath(baseName string, kind pcs.ParticleKind) string {
	return filepath.Join(pcs.StorageForParticle(kind), pcs.ShardKey(baseName, kind))
}

// ParticlePath returns the absolute path for a particle kind.
func ParticlePath(baseDir, baseName string, kind pcs.ParticleKind) string {
	return filepath.Join(baseDir, ParticleRelPath(baseName, kind))
}

// EnsureStorageDirs creates storageA/B/C under baseDir.
func EnsureStorageDirs(baseDir string) error {
	for _, dir := range []string{pcs.StorageA, pcs.StorageB, pcs.StorageC} {
		if err := os.MkdirAll(filepath.Join(baseDir, dir), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// ReconstructedFileName returns the default decode output name for baseName.
func ReconstructedFileName(baseName string) string {
	ext := filepath.Ext(baseName)
	if ext == "" {
		return baseName + "_reconstructed"
	}
	name := baseName[:len(baseName)-len(ext)]
	return name + "_reconstructed" + ext
}
