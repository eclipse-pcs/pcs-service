package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/eclipse-pcs/pcs-service/internal/config"
	"github.com/eclipse-pcs/pcs-service/internal/protocol"
)

// Server is the pcs-service TCP server.
type Server struct {
	cfg *config.Config

	mu           sync.Mutex
	ln           net.Listener
	shuttingDown bool
	wg           sync.WaitGroup
	sessionSem   chan struct{}
}

func New(cfg *config.Config) *Server {
	s := &Server{cfg: cfg}
	if cfg.MaxConcurrentSessions > 0 {
		s.sessionSem = make(chan struct{}, cfg.MaxConcurrentSessions)
	}
	return s
}

func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.cfg.Listen, err)
	}
	return s.Serve(ln)
}

func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.isShuttingDown() {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		if !s.tryAcquireSession() {
			_ = conn.Close()
			log.Printf("pcs-service session rejected reason=at_capacity max_sessions=%d", s.cfg.MaxConcurrentSessions)
			continue
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			defer s.releaseSession()
			s.handleCom(c)
		}(conn)
	}
}

// Shutdown stops accepting new sessions and waits for in-flight sessions to finish.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.shuttingDown = true
	ln := s.ln
	s.mu.Unlock()

	if ln != nil {
		_ = ln.Close()
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) isShuttingDown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shuttingDown
}

func (s *Server) tryAcquireSession() bool {
	if s.sessionSem == nil {
		return true
	}
	select {
	case s.sessionSem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Server) releaseSession() {
	if s.sessionSem == nil {
		return
	}
	<-s.sessionSem
}

func (s *Server) handleCom(conn net.Conn) {
	defer conn.Close()
	start := time.Now()
	if s.cfg.SessionTimeout > 0 {
		deadline := time.Now().Add(s.cfg.SessionTimeout)
		_ = conn.SetDeadline(deadline)
	}
	mode, mergeProfile, err := protocol.ReadClientHandshake(conn)
	if err != nil {
		return
	}
	if mode == protocol.ModeMerge {
		if err := mergeProfile.Validate(); err != nil {
			_ = protocol.WriteErrorLine(conn, err)
			return
		}
	}
	ports, accepts, err := protocol.AllocatePorts()
	if err != nil {
		return
	}
	inv := protocol.Invitation{Token: s.cfg.Token, Mode: mode, Ports: ports}
	if _, err := io.WriteString(conn, inv.Format()); err != nil {
		return
	}
	validateDoc := mode == protocol.ModeMerge
	sess, err := protocol.AcceptSession(s.cfg.Token, accepts, validateDoc)
	if err != nil {
		return
	}
	_ = conn.SetDeadline(time.Time{})

	var stats sessionStats
	switch mode {
	case protocol.ModeSplit:
		stats, err = handleSplit(conn, sess, s.cfg)
	case protocol.ModeMerge:
		stats, err = handleMerge(conn, sess, s.cfg, mergeProfile)
	default:
		err = fmt.Errorf("unknown mode %q", mode)
	}
	if err != nil {
		_ = protocol.WriteErrorLine(conn, err)
	}
	logSession(stats, time.Since(start), err)
}

// ErrServerClosed is returned when Serve exits after Shutdown.
var ErrServerClosed = errors.New("server closed")
