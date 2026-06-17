package lostfilm

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
)

// defaultTimeout bounds a single HTTP request/response cycle.
const defaultTimeout = 30 * time.Second

// sessionCookieName is the cookie lostfilm sets on a successful login.
const sessionCookieName = "lf_session"

// defaultMirrors lists the lostfilm base URLs tried, in order, when no explicit
// BaseURL is configured. lostfilm is blocked in some regions and rotates
// domains, so the client fails over to the next on a network or 5xx error.
func defaultMirrors() []string {
	return []string{
		"https://www.lostfilm.tv/",
		"https://www.lostfilmtv5.site/",
		"https://www.lostfilm.today/",
		"https://www.lostfilm.download/",
		"https://www.lostfilm.run/",
		"https://www.lostfilm.life/",
	}
}

// Client is the behaviour the MCP tools depend on. It is satisfied by *Scraper
// and mocked in tests.
type Client interface {
	// Feed returns the latest releases from the public RSS feed (no auth).
	Feed(ctx context.Context) ([]FeedItem, error)
	// Search returns series and movies matching query (no auth).
	Search(ctx context.Context, query string) ([]Series, error)
	// SeriesInfo returns a series' metadata and episode list. link is the
	// site-relative path from a search result (e.g. "/series/Peaky_Blinders").
	SeriesInfo(ctx context.Context, link string) (*SeriesInfo, error)
	// Torrents resolves the available quality variants for an episode. Pass
	// WholeSeasonEpisode (999) as episode for a whole-season pack. Requires auth.
	Torrents(ctx context.Context, seriesID, season, episode int) ([]TorrentVariant, error)
	// Download fetches the raw .torrent file from a variant's DownloadURL.
	Download(ctx context.Context, downloadURL string) (*TorrentFile, error)
}

// Options configures a Scraper. Credentials are optional: discovery works
// without them and only torrent resolution requires a session.
type Options struct {
	// BaseURL pins a single lostfilm base (e.g. a reachable mirror). When empty,
	// the client round-robins over defaultMirrors on failure.
	BaseURL string
	// Email and Password authenticate via the ajaxik.php login form.
	Email    string
	Password string
	// Cookie is a raw Cookie header (e.g. "lf_session=...; cf_clearance=...")
	// used instead of, or before, a password login. Lets the user bypass captcha
	// and Cloudflare by pasting a browser session.
	Cookie string
	// CookiePath persists the session between runs (empty disables persistence).
	CookiePath string
	// UserAgent overrides defaultUserAgent.
	UserAgent string
	// Transport overrides the HTTP round-tripper while leaving cookie-jar
	// ownership with the Scraper. Use it for a proxy or custom TLS.
	Transport http.RoundTripper
}

// Scraper is the concrete lostfilm.Client backed by net/http.
type Scraper struct {
	bases      []*url.URL
	active     atomic.Int32
	http       Doer
	jar        *cookiejar.Jar
	email      string
	password   string
	cookie     string
	cookiePath string
	userAgent  string

	// maxTorrentBytes caps a download body; overridable in tests.
	maxTorrentBytes int64

	mu     sync.Mutex
	authed bool
}

// New builds a Scraper from opts. It seeds the cookie jar from the raw cookie
// override and the on-disk session file, but defers the actual login until the
// first protected request.
func New(opts *Options) (*Scraper, error) {
	if opts == nil {
		opts = &Options{}
	}

	bases, err := parseBases(opts.BaseURL)
	if err != nil {
		return nil, err
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, errors.Wrap(err, "create cookie jar")
	}

	userAgent := opts.UserAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}

	scraper := &Scraper{
		bases: bases,
		http: &http.Client{
			Jar:       jar,
			Timeout:   defaultTimeout,
			Transport: opts.Transport,
		},
		jar:             jar,
		email:           opts.Email,
		password:        opts.Password,
		cookie:          opts.Cookie,
		cookiePath:      opts.CookiePath,
		userAgent:       userAgent,
		maxTorrentBytes: maxTorrentSize,
	}

	scraper.seedCookies()

	return scraper, nil
}

// parseBases resolves the configured base into one or more validated URLs.
func parseBases(rawBase string) ([]*url.URL, error) {
	raw := []string{rawBase}
	if rawBase == "" {
		raw = defaultMirrors()
	}

	bases := make([]*url.URL, 0, len(raw))

	for _, candidate := range raw {
		parsed, err := url.Parse(candidate)
		if err != nil {
			return nil, errors.Wrapf(err, "parse base URL %q", candidate)
		}

		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, ErrInvalidBaseURL
		}

		// lostfilm and its mirrors are HTTPS-only. Rejecting http bases avoids
		// silently dropping the Secure session cookie (which the jar will not
		// send over http), which would surface as a confusing ErrNotAuthenticated.
		if parsed.Scheme != "https" {
			return nil, errors.Wrapf(ErrInsecureBaseURL, "%q", candidate)
		}

		bases = append(bases, parsed)
	}

	return bases, nil
}

// currentBase returns the active mirror. It is lock-free so it can be called
// from request paths that already hold the auth mutex.
func (s *Scraper) currentBase() *url.URL {
	return s.bases[s.active.Load()]
}

// advanceMirror moves off the mirror at index failed, but only if it is still
// the active one. The compare-and-swap makes concurrent failovers converge:
// when several goroutines fail on the same mirror at once, exactly one advances
// and the rest become no-ops instead of racing the index forward and back.
func (s *Scraper) advanceMirror(failed int32) {
	//nolint:gosec // G115: bases holds a handful of mirrors; the length fits int32.
	count := int32(len(s.bases))
	s.active.CompareAndSwap(failed, (failed+1)%count)

	s.mu.Lock()
	s.authed = false
	s.mu.Unlock()
}

// runOnMirror runs a public operation, failing over to the next mirror on a
// network or 5xx error. It performs no authentication.
func runOnMirror[T any](scraper *Scraper, operation func() (T, error)) (T, error) {
	var (
		zero    T
		lastErr error
	)

	for range scraper.bases {
		active := scraper.active.Load()

		result, err := operation()
		if isMirrorError(err) {
			lastErr = err

			scraper.advanceMirror(active)

			continue
		}

		return result, err
	}

	return zero, lastErr
}

// runAuthed ensures a session, runs operation, retries once on session expiry,
// and fails over to the next mirror on network or 5xx errors.
func runAuthed[T any](ctx context.Context, scraper *Scraper, operation func() (T, error)) (T, error) {
	var (
		zero    T
		lastErr error
	)

	for range scraper.bases {
		active := scraper.active.Load()

		result, err := attemptAuthed(ctx, scraper, operation)
		if isMirrorError(err) {
			lastErr = err

			scraper.advanceMirror(active)

			continue
		}

		return result, err
	}

	return zero, lastErr
}

// attemptAuthed runs one mirror's worth of ensure-auth + operation, retrying
// once if the session expired mid-flight.
func attemptAuthed[T any](ctx context.Context, scraper *Scraper, operation func() (T, error)) (T, error) {
	var zero T

	err := scraper.ensureAuth(ctx)
	if err != nil {
		return zero, err
	}

	result, err := operation()
	if errors.Is(err, ErrNotAuthenticated) {
		reErr := scraper.reauth(ctx)
		if reErr != nil {
			return zero, reErr
		}

		return operation()
	}

	return result, err
}

// hasSessionCookie reports whether the jar holds a session for the active mirror.
func (s *Scraper) hasSessionCookie() bool {
	if s.jar == nil {
		return false
	}

	for _, cookie := range s.jar.Cookies(s.currentBase()) {
		if cookie.Name == sessionCookieName && cookie.Value != "" {
			return true
		}
	}

	return false
}

// isMirrorError reports whether err indicates the current mirror is unreachable
// and the client should fail over to the next one.
func isMirrorError(err error) bool {
	return errors.Is(err, ErrMirrorUnavailable)
}
