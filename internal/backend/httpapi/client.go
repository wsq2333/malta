// Package httpapi provides credential-safe HTTP for workflow backends.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/wsq2333/malta/internal/backend"
	"github.com/wsq2333/malta/internal/backend/auth"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	URL  string
	HTTP *http.Client
	Auth *auth.Provider
}

func New(c backend.Config) (*Client, error) {
	u, e := url.Parse(c.URL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("backend URL must be an origin without credentials")
	}
	ac, e := c.AuthConfig()
	if e != nil {
		return nil, e
	}
	p, e := auth.New(ac)
	if e != nil {
		return nil, e
	}
	return &Client{URL: strings.TrimRight(c.URL, "/"), Auth: p, HTTP: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Replay only after an explicit 401, never on an ambiguous transport failure.
func (c *Client) Call(ctx context.Context, method, path string, headers map[string]string, body, out any) error {
	raw, e := json.Marshal(body)
	if e != nil {
		return fmt.Errorf("invalid backend request")
	}
	for attempt := 0; attempt < 2; attempt++ {
		token, e := c.Auth.Token(ctx)
		if e != nil {
			return e
		}
		req, e := http.NewRequestWithContext(ctx, method, c.URL+path, bytes.NewReader(raw))
		if e != nil {
			return fmt.Errorf("invalid backend request")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, e := c.HTTP.Do(req)
		if e != nil {
			return fmt.Errorf("backend request failed: %T", e)
		}
		data, e := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
		resp.Body.Close()
		if e != nil || len(data) > 8<<20 {
			return fmt.Errorf("invalid or oversized backend response")
		}
		if resp.StatusCode == 401 && attempt == 0 {
			c.Auth.Invalidate(token)
			continue
		}
		if resp.StatusCode >= 300 {
			return &backend.HTTPError{Status: resp.StatusCode, Code: http.StatusText(resp.StatusCode)}
		}
		if out != nil && json.Unmarshal(data, out) != nil {
			return fmt.Errorf("invalid backend JSON response")
		}
		return nil
	}
	return fmt.Errorf("backend authentication failed")
}
