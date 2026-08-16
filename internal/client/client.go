package client

import (
	"fmt"
	"io"
	"net"

	"github.com/eclipse-pcs/pcs"

	"github.com/eclipse-pcs/pcs-service/internal/protocol"
)

const CopyBufferSize = 32 << 10

// Config holds client connection settings shared by pcs-split and pcs-merge.
type Config struct {
	Host  string
	Port  int
	Token string
}

// Address returns the control channel dial address.
func (c *Config) Address() string {
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%d", host, c.Port)
}

// Session is an active PCS control + data port session.
type Session struct {
	Control net.Conn
	Data    *protocol.SessionPortSet
}

// OpenSession dials the control channel, completes handshake, and connects data ports.
func (c *Config) OpenSession(mode protocol.Mode) (*Session, error) {
	return c.openSession(mode, protocol.MergeProfile{})
}

// OpenMergeSession opens a merge session with the required missing-core profile line.
func (c *Config) OpenMergeSession(profile protocol.MergeProfile) (*Session, error) {
	return c.openSession(protocol.ModeMerge, profile)
}

func (c *Config) openSession(mode protocol.Mode, profile protocol.MergeProfile) (*Session, error) {
	com, err := net.Dial("tcp", c.Address())
	if err != nil {
		return nil, fmt.Errorf("dial control: %w", err)
	}
	if _, err := io.WriteString(com, string(mode)+"\n"); err != nil {
		com.Close()
		return nil, fmt.Errorf("write mode: %w", err)
	}
	if mode == protocol.ModeMerge {
		if err := protocol.WriteMergeProfile(com, profile); err != nil {
			com.Close()
			return nil, fmt.Errorf("write merge profile: %w", err)
		}
	}
	inv, err := protocol.ReadInvitation(com)
	if err != nil {
		com.Close()
		return nil, fmt.Errorf("read invitation: %w", err)
	}
	if c.Token != "" && inv.Token != c.Token {
		com.Close()
		return nil, fmt.Errorf("unexpected token %q", inv.Token)
	}
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	ports, err := protocol.DialSession(host, inv)
	if err != nil {
		com.Close()
		return nil, fmt.Errorf("dial session: %w", err)
	}
	return &Session{Control: com, Data: ports}, nil
}

// Close shuts down control and data connections.
func (s *Session) Close() {
	if s.Data != nil {
		s.Data.Close()
	}
	if s.Control != nil {
		s.Control.Close()
	}
}

// ReadTrailer reads the session trailer from the control channel.
func ReadTrailer(s *Session) (*protocol.Trailer, error) {
	return protocol.ReadTrailer(s.Control)
}

// CopyBuffer copies from src to dst using a fixed-size buffer.
func CopyBuffer(dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, CopyBufferSize)
	return io.CopyBuffer(dst, src, buf)
}

// UploadDocument streams a document to the split session document port and half-closes it.
func (s *Session) UploadDocument(doc io.Reader) error {
	if _, err := CopyBuffer(s.Data.Doc, doc); err != nil {
		return err
	}
	return s.Data.Doc.Close()
}

// ReceiveParticles copies all particle streams into dst (keys are particle kinds).
func (s *Session) ReceiveParticles(dst map[pcs.ParticleKind]io.Writer) error {
	return copyParticleStreams(s.Data, dst)
}

// DrainParticles discards all particle streams in parallel.
func (s *Session) DrainParticles() error {
	return s.ReceiveParticles(discardWriters())
}

func discardWriters() map[pcs.ParticleKind]io.Writer {
	out := make(map[pcs.ParticleKind]io.Writer, 6)
	for _, kind := range protocol.ParticleKinds() {
		out[kind] = io.Discard
	}
	return out
}
