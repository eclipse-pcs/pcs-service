package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/eclipse-pcs/pcs"

	"github.com/eclipse-pcs/pcs-service/internal/client"
	"github.com/eclipse-pcs/pcs-service/internal/protocol"
	"github.com/eclipse-pcs/pcs-service/internal/store"
)

func main() {
	host := flag.String("host", "127.0.0.1", "server host")
	port := flag.Int("port", 4567, "control port")
	token := flag.String("token", "SECRET_TOKEN", "session token")
	file := flag.String("f", "", "input file (- for stdin)")
	name := flag.String("name", "", "output basename when reading stdin")
	outDir := flag.String("o", ".", "output directory for particle files")
	flag.Parse()
	if *file == "" {
		fmt.Fprintln(os.Stderr, "usage: pcs-split -f file [-name basename when -f -]")
		os.Exit(1)
	}

	base, input, err := openInput(*file, *name)
	if err != nil {
		fatal(err)
	}
	if c, ok := input.(io.Closer); ok {
		defer c.Close()
	}

	cfg := &client.Config{Host: *host, Port: *port, Token: *token}
	sess, err := cfg.OpenSession(protocol.ModeSplit)
	if err != nil {
		fatal(err)
	}
	defer sess.Close()

	if err := store.EnsureStorageDirs(*outDir); err != nil {
		fatal(err)
	}
	writers, err := openParticleWriters(*outDir, base)
	if err != nil {
		fatal(err)
	}

	var wg sync.WaitGroup
	var copyErr error
	var mu sync.Mutex
	copyTo := func(kind pcs.ParticleKind, dst io.Writer, src io.ReadCloser) {
		defer wg.Done()
		_, err := client.CopyBuffer(dst, src)
		_ = src.Close()
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			copyErr = err
		}
		if c, ok := dst.(io.Closer); ok {
			_ = c.Close()
		}
		_ = kind
	}

	wg.Add(6)
	go copyTo(pcs.EvenCypher, writers[pcs.EvenCypher], sess.Data.EC)
	go copyTo(pcs.OddCypher, writers[pcs.OddCypher], sess.Data.OC)
	go copyTo(pcs.EvenNoise, writers[pcs.EvenNoise], sess.Data.EN)
	go copyTo(pcs.OddNoise, writers[pcs.OddNoise], sess.Data.ON)
	go copyTo(pcs.CypherParity, writers[pcs.CypherParity], sess.Data.CP)
	go copyTo(pcs.NoiseParity, writers[pcs.NoiseParity], sess.Data.NP)

	if err := sess.UploadDocument(input); err != nil {
		fatal(err)
	}
	wg.Wait()
	if _, err := client.ReadTrailer(sess); err != nil {
		fatal(err)
	}
	if copyErr != nil {
		fatal(copyErr)
	}
}

func openInput(file, name string) (base string, r io.Reader, err error) {
	if file == "-" {
		if name == "" {
			return "", nil, fmt.Errorf("-name is required when -f -")
		}
		return filepath.Base(name), os.Stdin, nil
	}
	f, err := os.Open(file)
	if err != nil {
		return "", nil, err
	}
	return filepath.Base(file), f, nil
}

func openParticleWriters(outDir, base string) (map[pcs.ParticleKind]*os.File, error) {
	out := make(map[pcs.ParticleKind]*os.File, len(pcs.AllParticleKinds))
	for _, kind := range pcs.AllParticleKinds {
		path := store.ParticlePath(outDir, base, kind)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			closeFiles(out)
			return nil, err
		}
		f, err := os.Create(path)
		if err != nil {
			closeFiles(out)
			return nil, err
		}
		out[kind] = f
	}
	return out, nil
}

func closeFiles(files map[pcs.ParticleKind]*os.File) {
	for _, f := range files {
		if f != nil {
			_ = f.Close()
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
