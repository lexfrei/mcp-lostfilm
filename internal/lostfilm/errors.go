package lostfilm

import "github.com/cockroachdb/errors"

// ErrNotAuthenticated indicates a session is missing or expired: a protected
// endpoint (v_search.php) answered with "log in first" instead of a result.
var ErrNotAuthenticated = errors.New("not authenticated: session missing or expired")

// ErrCaptcha indicates lostfilm demanded a captcha during login, which happens
// after several failed attempts. The caller must obtain an lf_session cookie
// manually (e.g. from a browser) and provide it via LOSTFILM_COOKIE.
var ErrCaptcha = errors.New("login requires a captcha: provide a session cookie via LOSTFILM_COOKIE")

// ErrLoginFailed indicates the e-mail/password were rejected by lostfilm.
var ErrLoginFailed = errors.New("login failed: invalid e-mail or password")

// ErrNoCredentials indicates neither a session cookie nor an e-mail/password
// were configured, so the client cannot authenticate for torrent resolution.
var ErrNoCredentials = errors.New("no credentials: set LOSTFILM_EMAIL/LOSTFILM_PASSWORD or LOSTFILM_COOKIE")

// ErrNotFound indicates the requested series or episode does not exist or is
// not visible.
var ErrNotFound = errors.New("not found")

// ErrParse indicates the page structure did not match the expected layout,
// usually because lostfilm changed its markup.
var ErrParse = errors.New("failed to parse lostfilm response")

// ErrInvalidBaseURL indicates the configured base URL lacks a scheme or host.
var ErrInvalidBaseURL = errors.New("invalid base URL: scheme and host are required")

// ErrInsecureBaseURL indicates the configured base URL is not HTTPS; lostfilm
// is HTTPS-only and an http base would silently drop the secure session cookie.
var ErrInsecureBaseURL = errors.New("base URL must use https")

// ErrMirrorUnavailable indicates a transport error or 5xx response from the
// current mirror, signalling the client to fail over to the next base URL.
var ErrMirrorUnavailable = errors.New("lostfilm mirror unavailable")

// ErrDownloadFailed indicates a torrent download returned something other than
// a bencoded .torrent file.
var ErrDownloadFailed = errors.New("download did not return a .torrent file")

// ErrTorrentTooLarge indicates the .torrent body exceeded the size cap; the
// download is rejected rather than silently truncated to a wrong info-hash.
var ErrTorrentTooLarge = errors.New("torrent file exceeds the maximum allowed size")
