package service

import "github.com/wsq2333/malta/internal/domain"

func (m *Manager) Capabilities() domain.Capabilities {
	c := domain.OfflineCapabilities()
	c.Mode = "server"
	c.Executors = []string{}
	if len(m.Config.AllowedRoots) > 0 {
		c.Executors = append(c.Executors, "local")
	}
	if m.Config.Kubernetes != nil {
		c.Executors = append(c.Executors, "kubernetes")
	}
	c.Features["managed_runs"] = !m.Config.ManagementOnly
	c.Features["management_only"] = m.Config.ManagementOnly
	c.Features["request_lookup"] = true
	c.Limits["prepared_bytes"] = m.Config.MaxBytes
	c.Limits["run_timeout_seconds"] = m.Config.Timeout.Seconds()
	c.Limits["concurrent_runs"] = 1
	if m.Config.ManagementOnly {
		c.Limits["concurrent_runs"] = 0
	}
	return c
}
