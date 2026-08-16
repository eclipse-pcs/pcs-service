package protocol

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Mode is a PCS session mode.
type Mode string

const (
	ModeSplit Mode = "split"
	ModeMerge Mode = "merge"
)

// DataPorts holds ephemeral TCP ports for one session.
type DataPorts struct {
	Doc int
	EC  int
	OC  int
	EN  int
	ON  int
	CP  int
	NP  int
}

// Invitation is sent to the client on the control channel after handshake.
type Invitation struct {
	Token string
	Mode  Mode
	Ports DataPorts
}

func ParseModeLine(line string) (Mode, error) {
	mode := Mode(strings.TrimSpace(line))
	switch mode {
	case ModeSplit, ModeMerge:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid mode %q: want split or merge", line)
	}
}

func (inv Invitation) Format() string {
	return fmt.Sprintf("%s %s %d %d %d %d %d %d %d\n",
		inv.Token, inv.Mode,
		inv.Ports.Doc, inv.Ports.EC, inv.Ports.OC, inv.Ports.EN, inv.Ports.ON, inv.Ports.CP, inv.Ports.NP)
}

// ReadClientHandshake reads the mode line and, for merge, the required profile line.
func ReadClientHandshake(r io.Reader) (Mode, MergeProfile, error) {
	br := bufio.NewReader(r)
	line, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", MergeProfile{}, fmt.Errorf("read mode line: %w", err)
	}
	if line == "" {
		return "", MergeProfile{}, fmt.Errorf("empty mode line")
	}
	mode, err := ParseModeLine(line)
	if err != nil {
		return "", MergeProfile{}, err
	}
	var profile MergeProfile
	if mode == ModeMerge {
		line, err = br.ReadString('\n')
		if err != nil {
			return "", MergeProfile{}, fmt.Errorf("read profile line: %w", err)
		}
		profile, err = ParseProfileLine(line)
		if err != nil {
			return "", MergeProfile{}, err
		}
	}
	return mode, profile, nil
}

func ReadClientMode(r io.Reader) (Mode, error) {
	br := bufio.NewReader(r)
	line, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read mode line: %w", err)
	}
	if line == "" {
		return "", fmt.Errorf("empty mode line")
	}
	return ParseModeLine(line)
}

func ReadInvitation(r io.Reader) (*Invitation, error) {
	br := bufio.NewReader(r)
	line, err := br.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read invitation: %w", err)
	}
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) != 9 {
		return nil, fmt.Errorf("invitation has %d fields, want 9", len(fields))
	}
	ports := make([]int, 7)
	for i := range ports {
		ports[i], err = strconv.Atoi(fields[2+i])
		if err != nil {
			return nil, fmt.Errorf("parse port %q: %w", fields[2+i], err)
		}
	}
	return &Invitation{
		Token: fields[0],
		Mode:  Mode(fields[1]),
		Ports: DataPorts{
			Doc: ports[0], EC: ports[1], OC: ports[2], EN: ports[3], ON: ports[4], CP: ports[5], NP: ports[6],
		},
	}, nil
}
