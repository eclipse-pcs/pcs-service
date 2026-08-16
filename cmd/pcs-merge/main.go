package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"pcs-service/internal/client"
	"pcs-service/internal/protocol"
	"pcs-service/internal/store"
)

func main() {
	host := flag.String("host", "127.0.0.1", "server host")
	port := flag.Int("port", 4567, "control port")
	token := flag.String("token", "SECRET_TOKEN", "session token (must match server)")
	file := flag.String("f", "", "original base file name used for particles")
	out := flag.String("o", "", "output reconstructed file (default <base>_reconstructed)")
	inDir := flag.String("dir", ".", "directory containing storageA/B/C")
	skip := flag.String("skip", "", "comma-separated core kinds to omit (e.g. ec)")
	flag.Parse()
	if *file == "" {
		fmt.Fprintln(os.Stderr, "usage: pcs-merge -f Hello.txt")
		os.Exit(1)
	}
	base := filepath.Base(*file)
	skipped := client.SplitCSV(*skip)

	cfg := &client.Config{Host: *host, Port: *port, Token: *token}
	profile, err := client.PrepareMergeFromFiles(*inDir, base)
	if err != nil {
		fatal(err)
	}
	if len(skipped) > 0 {
		profile, err = protocol.MergeProfileFromSkipKinds(skipped)
		if err != nil {
			fatal(err)
		}
	}
	if _, err := client.PrepareMergeSendPlan(*inDir, base, profile); err != nil {
		fatal(err)
	}
	outPath := *out
	if outPath == "" {
		outPath = store.ReconstructedFileName(base)
	}
	outFile, err := os.Create(outPath)
	if err != nil {
		fatal(err)
	}
	defer outFile.Close()

	trailer, err := client.MergeFromFiles(cfg, *inDir, base, profile, outFile)
	if err != nil {
		fatal(err)
	}
	if trailer.HashValid != nil && !*trailer.HashValid {
		fmt.Fprintf(os.Stderr, "warning: hash validation failed\n")
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
