package protocol

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/eclipse-pcs/pcs"
)

// SessionPortSet holds connections for one session.
type SessionPortSet struct {
	Doc net.Conn
	EC  io.ReadWriteCloser
	OC  io.ReadWriteCloser
	EN  io.ReadWriteCloser
	ON  io.ReadWriteCloser
	CP  io.ReadWriteCloser
	NP  io.ReadWriteCloser
}

type payloadConn struct {
	net.Conn
	r *bufio.Reader
}

func (p *payloadConn) Read(b []byte) (int, error) {
	return p.r.Read(b)
}

// AllocatePorts binds seven ephemeral listeners and returns ports plus accept funcs.
func AllocatePorts() (DataPorts, []func() (net.Conn, error), error) {
	listeners := make([]net.Listener, 7)
	ports := make([]int, 7)
	for i := range listeners {
		ln, err := net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			for j := 0; j < i; j++ {
				listeners[j].Close()
			}
			return DataPorts{}, nil, fmt.Errorf("listen ephemeral port: %w", err)
		}
		listeners[i] = ln
		ports[i] = ln.Addr().(*net.TCPAddr).Port
	}
	accepts := make([]func() (net.Conn, error), 7)
	for i, ln := range listeners {
		ln := ln
		accepts[i] = func() (net.Conn, error) {
			conn, err := ln.Accept()
			if err != nil {
				return nil, err
			}
			ln.Close()
			return conn, nil
		}
	}
	return DataPorts{
		Doc: ports[0], EC: ports[1], OC: ports[2], EN: ports[3], ON: ports[4], CP: ports[5], NP: ports[6],
	}, accepts, nil
}

// AcceptSession waits for seven connections and validates tokens on selected ports.
func AcceptSession(token string, accepts []func() (net.Conn, error), validateDoc bool) (*SessionPortSet, error) {
	if len(accepts) != 7 {
		return nil, fmt.Errorf("want 7 accept funcs, got %d", len(accepts))
	}
	conns := make([]net.Conn, 7)
	var wg sync.WaitGroup
	errs := make([]error, 7)
	for i := range conns {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			conn, err := accepts[idx]()
			if err != nil {
				errs[idx] = err
				return
			}
			conns[idx] = conn
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			closeAll(conns)
			return nil, fmt.Errorf("accept port %d: %w", i, err)
		}
	}
	validate := []bool{validateDoc, true, true, true, true, true, true}
	payloads := make([]io.ReadWriteCloser, 7)
	for i, conn := range conns {
		if validate[i] {
			pc, err := consumeTokenConn(conn, token)
			if err != nil {
				closeAll(conns)
				return nil, fmt.Errorf("port %d token auth: %w", i, err)
			}
			payloads[i] = pc
		} else {
			payloads[i] = conn
		}
	}
	return &SessionPortSet{
		Doc: conns[0], EC: payloads[1], OC: payloads[2], EN: payloads[3], ON: payloads[4], CP: payloads[5], NP: payloads[6],
	}, nil
}

func (p *payloadConn) hasData() bool {
	if p.r.Buffered() > 0 {
		return true
	}
	_, err := p.r.Peek(1)
	return err == nil
}

// ReaderHasData reports whether r has unread payload bytes after token consumption.
func ReaderHasData(r io.Reader) bool {
	if p, ok := r.(*payloadConn); ok {
		return p.hasData()
	}
	return true
}

func consumeTokenConn(conn net.Conn, token string) (*payloadConn, error) {
	br := bufio.NewReader(conn)
	for i := 0; i < len(token); i++ {
		b, err := br.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("read token: %w", err)
		}
		if b != token[i] {
			return nil, fmt.Errorf("invalid token prefix")
		}
	}
	return &payloadConn{Conn: conn, r: br}, nil
}

// DialSession connects to all data ports and sends the token prefix.
func DialSession(host string, inv *Invitation) (*SessionPortSet, error) {
	portList := []int{inv.Ports.Doc, inv.Ports.EC, inv.Ports.OC, inv.Ports.EN, inv.Ports.ON, inv.Ports.CP, inv.Ports.NP}
	conns := make([]net.Conn, 7)
	for i, port := range portList {
		addr := fmt.Sprintf("%s:%d", host, port)
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			closeAll(conns[:i])
			return nil, fmt.Errorf("dial %s: %w", addr, err)
		}
		if _, err := io.WriteString(conn, inv.Token); err != nil {
			closeAll(conns[:i+1])
			return nil, fmt.Errorf("write token to %s: %w", addr, err)
		}
		conns[i] = conn
	}
	return &SessionPortSet{
		Doc: conns[0], EC: conns[1], OC: conns[2], EN: conns[3], ON: conns[4], CP: conns[5], NP: conns[6],
	}, nil
}

// StripTokenReader strips a leading token from a document stream (split mode).
type StripTokenReader struct {
	token string
	r     *bufio.Reader
	done  bool
}

func NewStripTokenReader(conn net.Conn, token string) *StripTokenReader {
	return &StripTokenReader{token: token, r: bufio.NewReader(conn)}
}

func (s *StripTokenReader) Read(p []byte) (int, error) {
	if !s.done {
		if err := s.consumeToken(); err != nil {
			return 0, err
		}
	}
	return s.r.Read(p)
}

func (s *StripTokenReader) consumeToken() error {
	for i := 0; i < len(s.token); i++ {
		b, err := s.r.ReadByte()
		if err != nil {
			return err
		}
		if b != s.token[i] {
			return fmt.Errorf("invalid document port token")
		}
	}
	s.done = true
	return nil
}

func closeAll(conns []net.Conn) {
	for _, c := range conns {
		if c != nil {
			c.Close()
		}
	}
}

func (s *SessionPortSet) Close() {
	closeIO(s.Doc)
	closeIO(s.EC)
	closeIO(s.OC)
	closeIO(s.EN)
	closeIO(s.ON)
	closeIO(s.CP)
	closeIO(s.NP)
}

func closeIO(c io.Closer) {
	if c != nil {
		c.Close()
	}
}

func ParticleKinds() [6]pcs.ParticleKind {
	return [6]pcs.ParticleKind{pcs.EvenCypher, pcs.OddCypher, pcs.EvenNoise, pcs.OddNoise, pcs.CypherParity, pcs.NoiseParity}
}
