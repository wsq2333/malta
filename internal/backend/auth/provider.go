// Package auth obtains credentials from mounted Secret files. Tokens stay in memory.
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Type             string `json:"type"`
	TokenFile        string `json:"token_file,omitempty"`
	TokenURL         string `json:"token_url,omitempty"`
	ClientID         string `json:"client_id,omitempty"`
	ClientSecretFile string `json:"client_secret_file,omitempty"`
	UsernameFile     string `json:"username_file,omitempty"`
	PasswordFile     string `json:"password_file,omitempty"`
	Scope            string `json:"scope,omitempty"`
}
type Provider struct {
	config         Config
	http           *http.Client
	mu             sync.Mutex
	token, refresh string
	expires, renew time.Time
	credentials    [32]byte
	retryAt        time.Time
}

func New(c Config) (*Provider, error) {
	if c.Type == "" {
		c.Type = "bearer_token_file"
	}
	switch c.Type {
	case "bearer_token_file":
		if c.TokenFile == "" || c.TokenURL != "" || c.ClientSecretFile != "" || c.UsernameFile != "" || c.PasswordFile != "" {
			return nil, errors.New("invalid bearer file configuration")
		}
	case "oauth2_client_credentials", "oauth2_password":
		u, e := url.Parse(c.TokenURL)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && net.ParseIP(u.Hostname()).IsLoopback())) {
			return nil, errors.New("OAuth token URL requires HTTPS (HTTP allowed only on loopback)")
		}
		if c.ClientID == "" || c.TokenFile != "" {
			return nil, errors.New("OAuth requires client_id and no static token_file")
		}
		if c.Type == "oauth2_client_credentials" && (c.ClientSecretFile == "" || c.UsernameFile != "" || c.PasswordFile != "") {
			return nil, errors.New("client credentials requires only client_secret_file")
		}
		if c.Type == "oauth2_password" && (c.UsernameFile == "" || c.PasswordFile == "") {
			return nil, errors.New("password authentication requires username_file and password_file")
		}
	default:
		return nil, errors.New("unsupported authentication type")
	}
	return &Provider{config: c, http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func read(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", errors.New("credential file unavailable")
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 65537))
	if e != nil || len(b) > 65536 {
		return "", errors.New("invalid credential file")
	}
	s := strings.TrimSpace(string(b))
	if s == "" || strings.ContainsAny(s, "\r\n") {
		return "", errors.New("credential file empty or multiline")
	}
	return s, nil
}
func (p *Provider) form() (url.Values, [32]byte, error) {
	v := url.Values{"client_id": {p.config.ClientID}}
	for k, path := range map[string]string{"client_secret": p.config.ClientSecretFile, "username": p.config.UsernameFile, "password": p.config.PasswordFile} {
		if path != "" {
			s, e := read(path)
			if e != nil {
				return nil, [32]byte{}, e
			}
			v.Set(k, s)
		}
	}
	return v, sha256.Sum256([]byte(v.Encode())), nil
}

// CheckFiles never performs a login or returns credential values.
func (p *Provider) CheckFiles() error {
	if p.config.Type == "bearer_token_file" {
		_, e := read(p.config.TokenFile)
		return e
	}
	_, _, e := p.form()
	return e
}
func (p *Provider) Status() string {
	if p.CheckFiles() != nil {
		return "credentials_missing"
	}
	return "configured"
}
func (p *Provider) Invalidate(rejected string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token == rejected {
		p.token = ""
		p.expires = time.Time{}
		p.renew = time.Time{}
	}
}
func (p *Provider) Token(ctx context.Context) (string, error) {
	if p.config.Type == "bearer_token_file" {
		return read(p.config.TokenFile)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return "", errors.New("authentication cancelled")
	}
	v, fingerprint, e := p.form()
	if e != nil {
		return "", e
	}
	now := time.Now()
	if fingerprint != p.credentials {
		p.token = ""
		p.refresh = ""
		p.retryAt = time.Time{}
		p.credentials = fingerprint
	}
	if p.token != "" && now.Before(p.renew) {
		return p.token, nil
	}
	if now.Before(p.retryAt) {
		return "", errors.New("authentication temporarily unavailable; retry later")
	}
	if p.config.Scope != "" {
		v.Set("scope", p.config.Scope)
	}
	grant := "client_credentials"
	if p.config.Type == "oauth2_password" {
		grant = "password"
	}
	if p.refresh != "" {
		v.Set("grant_type", "refresh_token")
		v.Set("refresh_token", p.refresh)
		v.Del("username")
		v.Del("password")
	} else {
		v.Set("grant_type", grant)
	}
	response, invalid, e := p.fetch(ctx, v)
	if e != nil && invalid && v.Get("grant_type") == "refresh_token" {
		p.refresh = ""
		v, _, e = p.form()
		if e == nil {
			v.Set("grant_type", grant)
			if p.config.Scope != "" {
				v.Set("scope", p.config.Scope)
			}
			response, _, e = p.fetch(ctx, v)
		}
	}
	if e != nil {
		p.retryAt = now.Add(3 * time.Second)
		return "", e
	}
	lifetime := time.Duration(response.ExpiresIn) * time.Second
	if response.AccessToken == "" || !strings.EqualFold(response.TokenType, "bearer") || response.ExpiresIn < 1 || response.ExpiresIn > 86400*365 || strings.ContainsAny(response.AccessToken, "\r\n") {
		p.retryAt = now.Add(3 * time.Second)
		return "", errors.New("invalid OAuth token response")
	}
	p.token = response.AccessToken
	if response.RefreshToken != "" {
		p.refresh = response.RefreshToken
	}
	// Retain an existing refresh token when the provider doesn't rotate it.
	margin := lifetime / 5
	if margin > 30*time.Second {
		margin = 30 * time.Second
	}
	p.expires = time.Now().Add(lifetime)
	p.renew = p.expires.Add(-margin)
	p.retryAt = time.Time{}
	return p.token, nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Error        string `json:"error"`
}

func (p *Provider) fetch(ctx context.Context, v url.Values) (tokenResponse, bool, error) {
	var out tokenResponse
	req, e := http.NewRequestWithContext(ctx, "POST", p.config.TokenURL, strings.NewReader(v.Encode()))
	if e != nil {
		return out, false, errors.New("invalid token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, e := p.http.Do(req)
	if e != nil {
		return out, false, errors.New("token endpoint unavailable")
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if e != nil || len(b) > 65536 {
		return out, false, errors.New("invalid token response")
	}
	if json.Unmarshal(b, &out) != nil {
		return out, false, fmt.Errorf("token endpoint returned HTTP %d with invalid JSON", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		return tokenResponse{}, out.Error == "invalid_grant", fmt.Errorf("token endpoint rejected authentication (HTTP %d)", resp.StatusCode)
	}
	return out, false, nil
}
