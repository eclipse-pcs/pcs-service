package client

import (
	"fmt"
	"net"
	"strconv"
)

func netSplitHostPort(addr string) (host, port string, err error) {
	return net.SplitHostPort(addr)
}

func parsePort(s string) (int, error) {
	port, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("parse port %q: %w", s, err)
	}
	return port, nil
}
