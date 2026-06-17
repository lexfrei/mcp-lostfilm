// Package config loads the mcp-lostfilm configuration from environment
// variables.
package config

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/cockroachdb/errors"
)

const maxPort = 65535

// ErrInvalidHTTPPort is returned when MCP_HTTP_PORT is not a valid port number.
var ErrInvalidHTTPPort = errors.New("MCP_HTTP_PORT must be a valid port number (1-65535)")

// ErrInvalidProxy is returned when LOSTFILM_PROXY is not a valid URL.
var ErrInvalidProxy = errors.New("LOSTFILM_PROXY must be a valid proxy URL")

// Config holds the application configuration loaded from environment variables.
type Config struct {
	// Email and Password authenticate against the lostfilm login form. The
	// username must be the account e-mail address.
	Email    string
	Password string
	// Cookie is a raw Cookie header used instead of a password login. It should
	// carry at least lf_session, and optionally cf_clearance when a mirror is
	// behind a Cloudflare challenge.
	Cookie string
	// CookieFile persists the session between runs.
	CookieFile string
	// BaseURL pins a single lostfilm mirror; empty enables mirror round-robin.
	BaseURL string
	// UserAgent overrides the default browser User-Agent. It must match the one
	// used to obtain cf_clearance when that cookie is supplied.
	UserAgent string
	// Proxy is an optional HTTP/SOCKS5 proxy URL.
	Proxy string
	// DownloadDir is where .torrent files are written when requested.
	DownloadDir string
	// HTTPPort and HTTPHost configure the optional HTTP transport.
	HTTPPort string
	HTTPHost string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	httpPort := os.Getenv("MCP_HTTP_PORT")
	if httpPort != "" {
		port, err := strconv.Atoi(httpPort)
		if err != nil || port < 1 || port > maxPort {
			return nil, ErrInvalidHTTPPort
		}
	}

	proxy := os.Getenv("LOSTFILM_PROXY")
	if proxy != "" {
		_, err := parseProxy(proxy)
		if err != nil {
			return nil, err
		}
	}

	httpHost := os.Getenv("MCP_HTTP_HOST")
	if httpHost == "" {
		httpHost = "127.0.0.1"
	}

	return &Config{
		Email:       os.Getenv("LOSTFILM_EMAIL"),
		Password:    os.Getenv("LOSTFILM_PASSWORD"),
		Cookie:      os.Getenv("LOSTFILM_COOKIE"),
		CookieFile:  resolveCookieFile(os.Getenv("LOSTFILM_COOKIE_FILE")),
		BaseURL:     os.Getenv("LOSTFILM_BASE_URL"),
		UserAgent:   os.Getenv("LOSTFILM_USER_AGENT"),
		Proxy:       proxy,
		DownloadDir: os.Getenv("LOSTFILM_DOWNLOAD_DIR"),
		HTTPPort:    httpPort,
		HTTPHost:    httpHost,
	}, nil
}

// resolveCookieFile returns the configured cookie file, defaulting to
// ~/.mcp-lostfilm/cookies.json when unset and a home directory is available.
func resolveCookieFile(configured string) string {
	if configured != "" {
		return configured
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".mcp-lostfilm", "cookies.json")
}

// HasCredentials reports whether any authentication method is configured.
// Discovery tools (feed, search) work without credentials; only torrent
// resolution and download require a session, so the server starts either way.
func (c *Config) HasCredentials() bool {
	return c.Cookie != "" || (c.Email != "" && c.Password != "")
}

// ProxyTransport builds an HTTP round-tripper honouring the configured proxy,
// or returns nil when no proxy is set.
func (c *Config) ProxyTransport() (http.RoundTripper, error) {
	if c.Proxy == "" {
		return nil, nil //nolint:nilnil // no proxy configured means no custom transport.
	}

	proxyURL, err := parseProxy(c.Proxy)
	if err != nil {
		return nil, err
	}

	// Clone the default transport so HTTP/2, connection pooling, and the
	// dial/TLS-handshake timeouts are preserved; only the proxy is overridden.
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{Proxy: http.ProxyURL(proxyURL)}, nil
	}

	cloned := transport.Clone()
	cloned.Proxy = http.ProxyURL(proxyURL)

	return cloned, nil
}

// parseProxy validates and parses a proxy URL, requiring a scheme and host.
func parseProxy(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.Wrap(ErrInvalidProxy, raw)
	}

	return parsed, nil
}

// HTTPEnabled reports whether the HTTP transport should be started.
func (c *Config) HTTPEnabled() bool {
	return c.HTTPPort != ""
}

// HTTPAddr returns the host:port address for the HTTP server.
func (c *Config) HTTPAddr() string {
	return net.JoinHostPort(c.HTTPHost, c.HTTPPort)
}
