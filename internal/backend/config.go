package backend

import "github.com/wsq2333/malta/internal/backend/auth"

// Config describes a supported workflow endpoint. Backend must be explicit.
type Config struct {
	ID                string       `json:"id"`
	URL               string       `json:"url"`
	TokenFile         string       `json:"token_file"`
	RemoteResultsRoot string       `json:"remote_results_root"`
	LocalResultsRoot  string       `json:"local_results_root"`
	MaxBytes          int64        `json:"max_bytes"`
	TimeoutSeconds    int          `json:"timeout_seconds"`
	ArtifactBytes     int64        `json:"artifact_bytes"`
	Cluster           string       `json:"cluster"`
	Namespace         string       `json:"namespace"`
	Backend           string       `json:"backend,omitempty"`
	Enabled           *bool        `json:"enabled,omitempty"`
	Auth              *auth.Config `json:"auth,omitempty"`
}

func (c Config) Kind() string {
	return c.Backend
}
func (c Config) Active() bool { return c.Enabled == nil || *c.Enabled }
func (c Config) AuthConfig() (auth.Config, error) {
	if c.Auth != nil {
		if c.TokenFile != "" {
			return auth.Config{}, &HTTPError{Status: 400, Code: "use_auth_or_token_file_not_both"}
		}
		return *c.Auth, nil
	}
	return auth.Config{Type: "bearer_token_file", TokenFile: c.TokenFile}, nil
}
