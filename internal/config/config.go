package config

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

const (
	defaultListen       = "0.0.0.0:4567"
	defaultToken        = "SECRET_TOKEN"
	defaultMaxObject    = int64(0) // 0 = unlimited (streaming path is O(chunk))
	defaultChunkSize    = 32 << 10 // 32 KiB
	defaultSessionTO    = 5 * time.Minute
)

// Config holds pcs-service runtime settings.
type Config struct {
	Listen                string
	Token                 string
	SessionTimeout        time.Duration
	MaxObjectSize         int64
	ChunkSize             int
	MaxConcurrentSessions int
}

func Parse(args []string) (*Config, error) {
	fs := flag.NewFlagSet("pcs-service", flag.ContinueOnError)
	listen := fs.String("listen", envOr("PCS_LISTEN", defaultListen), "control channel bind address")
	token := fs.String("token", envOr("PCS_TOKEN", defaultToken), "shared session token")
	sessionTO := fs.Duration("session-timeout", defaultSessionTO, "idle timeout per session")
	maxSize := fs.Int64("max-object-size", envOrInt64("PCS_MAX_OBJECT_SIZE", defaultMaxObject), "maximum document size in bytes (0 = unlimited)")
	chunkSize := fs.Int("chunk-size", envOrInt("PCS_CHUNK_SIZE", defaultChunkSize), "read chunk size for streaming encode/decode")
	maxSessions := fs.Int("max-sessions", envOrInt("PCS_MAX_SESSIONS", 0), "max concurrent sessions (0 = unlimited)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *token == "" {
		return nil, fmt.Errorf("token must not be empty")
	}
	if *maxSize < 0 {
		return nil, fmt.Errorf("max-object-size must be >= 0")
	}
	if *chunkSize <= 0 {
		return nil, fmt.Errorf("chunk-size must be > 0")
	}
	if *maxSessions < 0 {
		return nil, fmt.Errorf("max-sessions must be >= 0")
	}
	return &Config{
		Listen:                *listen,
		Token:                 *token,
		SessionTimeout:        *sessionTO,
		MaxObjectSize:         *maxSize,
		ChunkSize:             *chunkSize,
		MaxConcurrentSessions: *maxSessions,
	}, nil
}

// SecurityWarnings returns startup warnings for unsafe production settings.
func (c *Config) SecurityWarnings() []string {
	if c.Token == defaultToken && !isLoopbackListen(c.Listen) {
		return []string{
			"default token SECRET_TOKEN on non-loopback listen address; set PCS_TOKEN before production use",
		}
	}
	return nil
}

func isLoopbackListen(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return false
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func envOrInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
