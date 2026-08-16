package store

import (
	"fmt"
	"io"
	"os"

	"github.com/eclipse-pcs/pcs/footer"
)

// OpenParticleFile opens a complete particle file (payload + footer) for reading.
func OpenParticleFile(path string) (io.ReadCloser, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() < footer.Size {
		return nil, fmt.Errorf("particle file too short: %s", path)
	}
	return os.Open(path)
}

// PayloadLen returns payload bytes from a particle file size on disk.
func PayloadLen(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return footer.PayloadLen(fi.Size())
}
