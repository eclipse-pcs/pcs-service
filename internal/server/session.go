package server

import (
	"io"
	"log"
	"time"

	"pcs-service/internal/protocol"
)

type sessionStats struct {
	mode       protocol.Mode
	bytes      int64
	recoveries []string
	hashValid  *bool
}

func (s sessionStats) recoveryUsed() bool {
	return len(s.recoveries) > 0
}

func logSession(stats sessionStats, duration time.Duration, err error) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	hashValid := "n/a"
	if stats.hashValid != nil {
		if *stats.hashValid {
			hashValid = "true"
		} else {
			hashValid = "false"
		}
	}
	log.Printf(
		"pcs-service session mode=%s bytes=%d duration_ms=%d recovery=%t hash_valid=%s error=%q",
		stats.mode,
		stats.bytes,
		duration.Milliseconds(),
		stats.recoveryUsed(),
		hashValid,
		errMsg,
	)
}

func closeWriteParticlePorts(ports *protocol.SessionPortSet) {
	closeWriteConn(ports.EC)
	closeWriteConn(ports.OC)
	closeWriteConn(ports.EN)
	closeWriteConn(ports.ON)
	closeWriteConn(ports.CP)
	closeWriteConn(ports.NP)
}

func closeWriteConn(c io.Closer) {
	if c == nil {
		return
	}
	type halfCloser interface {
		CloseWrite() error
	}
	if hw, ok := c.(halfCloser); ok {
		_ = hw.CloseWrite()
	}
}
