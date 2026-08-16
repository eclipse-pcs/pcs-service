package server

import (
	"testing"
	"time"

	"github.com/eclipse-pcs/pcs-service/internal/protocol"
)

// TestSessionStatsRecoveryUsed checks recoveryUsed reflects non-empty recoveries in session stats.
func TestSessionStatsRecoveryUsed(t *testing.T) {
	stats := sessionStats{recoveries: []string{"parity recovery used"}}
	if !stats.recoveryUsed() {
		t.Fatal("expected recovery used")
	}
	if (sessionStats{}).recoveryUsed() {
		t.Fatal("expected no recovery")
	}
}

// TestLogSessionDoesNotPanic smoke-tests session logging for merge/split success and error paths.
func TestLogSessionDoesNotPanic(t *testing.T) {
	valid := true
	logSession(sessionStats{
		mode:       protocol.ModeMerge,
		bytes:      42,
		recoveries: []string{"parity recovery used"},
		hashValid:  &valid,
	}, 10*time.Millisecond, nil)
	logSession(sessionStats{mode: protocol.ModeSplit}, time.Second, errExample{})
}

type errExample struct{}

func (errExample) Error() string { return "boom" }
