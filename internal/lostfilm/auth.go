package lostfilm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ajaxikPath is the JSON AJAX endpoint lostfilm uses for both login and search.
const ajaxikPath = "/ajaxik.php"

// ajaxik.php form field names shared by the login and search requests.
const (
	fieldAct  = "act"
	fieldType = "type"
)

// File permissions for the persisted session file and its parent directory.
const (
	cookieDirPerm  = 0o700
	cookieFilePerm = 0o600
)

// loginResponse is the JSON returned by the ajaxik.php login call. A successful
// login is {"name":"<user>","success":true,"result":"ok"}; a failure carries a
// non-zero "error" code instead.
type loginResponse struct {
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Error   int    `json:"error"`
	Result  string `json:"result"`
}

// loginErrorBadCredentials is the ajaxik.php error code for a rejected
// e-mail/password (as opposed to a captcha challenge).
const loginErrorBadCredentials = 3

// persistedCookie is the on-disk representation of a single session cookie.
type persistedCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// seedCookies populates the jar from the raw cookie override and, failing that,
// the persisted session file. It never logs in; that is deferred to ensureAuth.
// The override and persisted cookies are applied to every mirror so they remain
// valid after a failover.
func (s *Scraper) seedCookies() {
	if s.cookie != "" {
		cookies := parseCookieHeader(s.cookie)
		for _, base := range s.bases {
			s.jar.SetCookies(base, cookies)
		}

		return
	}

	s.loadCookies()
}

// ensureAuth guarantees a usable session before the first protected request.
func (s *Scraper) ensureAuth(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.authed || s.hasSessionCookie() {
		s.authed = true

		return nil
	}

	return s.login(ctx)
}

// reauth forces a fresh login after a session expired mid-flight. A client
// configured with only a raw cookie (no password) cannot recover and surfaces
// ErrNotAuthenticated.
func (s *Scraper) reauth(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.authed = false

	if s.email == "" || s.password == "" {
		return ErrNotAuthenticated
	}

	return s.login(ctx)
}

// login submits the credentials form and verifies a session cookie was issued.
// It must be called with s.mu held.
func (s *Scraper) login(ctx context.Context) error {
	if s.email == "" || s.password == "" {
		return ErrNoCredentials
	}

	// A stale session cookie makes ajaxik.php reject the login, so drop it first.
	if s.hasSessionCookie() {
		_ = s.logout(ctx)
	}

	form := url.Values{
		fieldAct:  {"users"},
		fieldType: {"login"},
		"mail":    {s.email},
		"pass":    {s.password},
		"rem":     {"1"},
	}

	body, err := s.postForm(ctx, ajaxikPath, form)
	if err != nil {
		return err
	}

	var resp loginResponse

	jsonErr := json.Unmarshal([]byte(body), &resp)
	if jsonErr == nil && resp.Success && s.hasSessionCookie() {
		s.authed = true
		s.saveCookies()

		return nil
	}

	return classifyLoginFailure(body, resp)
}

// logout invalidates the current session so a subsequent login starts clean.
func (s *Scraper) logout(ctx context.Context) error {
	_, err := s.postForm(ctx, ajaxikPath, url.Values{
		fieldAct:  {"users"},
		fieldType: {"logout"},
	})

	return err
}

// classifyLoginFailure distinguishes a captcha challenge from rejected
// credentials, given the parsed and raw login response.
func classifyLoginFailure(body string, resp loginResponse) error {
	if strings.Contains(strings.ToLower(body), "capt") {
		return ErrCaptcha
	}

	if resp.Error == loginErrorBadCredentials {
		return ErrLoginFailed
	}

	// Any other non-success code after several attempts is, in practice, a
	// captcha gate; steer the user toward the cookie override either way.
	if resp.Error != 0 {
		return ErrCaptcha
	}

	return ErrLoginFailed
}

// sessionCookie builds a lostfilm cookie with conservative attributes so the
// jar stores and replays it like a browser would.
func sessionCookie(name, value string) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

// parseCookieHeader turns a "name=value; name2=value2" header into cookies.
func parseCookieHeader(header string) []*http.Cookie {
	var cookies []*http.Cookie

	for part := range strings.SplitSeq(header, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		name, value, found := strings.Cut(part, "=")
		if !found {
			continue
		}

		cookies = append(cookies, sessionCookie(strings.TrimSpace(name), strings.TrimSpace(value)))
	}

	return cookies
}

// loadCookies restores persisted cookies into the jar, if a session file exists.
func (s *Scraper) loadCookies() {
	if s.cookiePath == "" {
		return
	}

	data, err := os.ReadFile(s.cookiePath)
	if err != nil {
		return
	}

	var stored []persistedCookie

	jsonErr := json.Unmarshal(data, &stored)
	if jsonErr != nil {
		return
	}

	cookies := make([]*http.Cookie, 0, len(stored))
	for _, item := range stored {
		cookies = append(cookies, sessionCookie(item.Name, item.Value))
	}

	for _, base := range s.bases {
		s.jar.SetCookies(base, cookies)
	}
}

// saveCookies persists the active mirror's jar cookies.
func (s *Scraper) saveCookies() {
	if s.cookiePath == "" {
		return
	}

	jarCookies := s.jar.Cookies(s.currentBase())
	stored := make([]persistedCookie, 0, len(jarCookies))

	for _, cookie := range jarCookies {
		stored = append(stored, persistedCookie{Name: cookie.Name, Value: cookie.Value})
	}

	data, err := json.Marshal(stored)
	if err != nil {
		return
	}

	mkErr := os.MkdirAll(filepath.Dir(s.cookiePath), cookieDirPerm)
	if mkErr != nil {
		return
	}

	_ = os.WriteFile(s.cookiePath, data, cookieFilePerm)
}
