package protocol

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

const errorPrefix = "error "

// SessionError is returned when the server reports a session failure on the control channel.
type SessionError struct {
	Message string
}

func (e *SessionError) Error() string {
	return e.Message
}

// WriteErrorLine sends a session error frame on the control channel before closing data ports.
func WriteErrorLine(w io.Writer, err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ReplaceAll(err.Error(), "\n", " ")
	_, writeErr := fmt.Fprintf(w, "%s%s\n", errorPrefix, msg)
	return writeErr
}

func parseErrorLine(line string) error {
	line = strings.TrimSuffix(line, "\n")
	if !strings.HasPrefix(line, errorPrefix) {
		return nil
	}
	msg := strings.TrimPrefix(line, errorPrefix)
	if msg == "" {
		return &SessionError{Message: "server session failed"}
	}
	return &SessionError{Message: msg}
}

// readSessionLine reads the first post-data line and returns a SessionError if it is an error frame.
func readSessionLine(br *bufio.Reader) (*Trailer, error) {
	line, err := br.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read session line: %w", err)
	}
	if sessionErr := parseErrorLine(line); sessionErr != nil {
		return nil, sessionErr
	}
	length, err := parseTrailerLength(line)
	if err != nil {
		return nil, err
	}
	return readTrailerBody(br, length)
}

func parseTrailerLength(line string) (int, error) {
	line = strings.TrimSuffix(line, "\n")
	length, err := parseIntDecimal(line)
	if err != nil {
		return 0, fmt.Errorf("parse trailer length: %w", err)
	}
	return length, nil
}

func parseIntDecimal(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty length")
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid integer %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
