// Package kubernetes implements a single-client Job executor using a bounded kubectl transport.
package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/wsq2333/malta/internal/domain"
)

type Config struct {
	Context        string `json:"context"`
	Namespace      string `json:"namespace"`
	Cluster        string `json:"cluster"`
	Node           string `json:"node"`
	BenchmarkPVC   string `json:"benchmark_pvc"`
	ResultsPVC     string `json:"results_pvc"`
	ResultsDir     string `json:"results_dir"`
	Image          string `json:"image"`
	ServiceAccount string `json:"service_account"`
	CPUMillis      int    `json:"cpu_millis"`
	MemoryBytes    int64  `json:"memory_bytes"`
	Kubectl        string `json:"kubectl,omitempty"`
}
type Transport interface {
	Do(context.Context, any, ...string) ([]byte, error)
}
type Command struct{ Config Config }

func (c Command) Do(ctx context.Context, input any, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	// Leave kubectl request timeout unset: overriding it disables client-go
	// in-cluster fallback in the pinned client. The process context bounds every call.
	prefix := []string{"--namespace", c.Config.Namespace}
	if c.Config.Context != "" {
		prefix = append(prefix, "--context", c.Config.Context)
	}
	binary := c.Config.Kubectl
	if binary == "" {
		binary = "kubectl"
	}
	cmd := exec.CommandContext(ctx, binary, append(prefix, args...)...)
	if input != nil {
		data, e := json.Marshal(input)
		if e != nil {
			return nil, e
		}
		cmd.Stdin = bytes.NewReader(data)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, e := cmd.Output()
	if e != nil {
		return nil, fmt.Errorf("kubectl %s: %w: %.2048s", args[0], e, stderr.String())
	}
	return data, nil
}

var dns = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)
var digest = regexp.MustCompile(`^[^\s]+@sha256:[a-f0-9]{64}$`)

func (c Config) Validate() error {
	for _, v := range []string{c.Namespace, c.Node, c.BenchmarkPVC, c.ResultsPVC, c.ServiceAccount} {
		if len(v) > 253 || !dns.MatchString(v) {
			return domain.E("invalid", "Kubernetes names must be explicit DNS names")
		}
	}
	if c.Cluster == "" || c.BenchmarkPVC == c.ResultsPVC || !filepath.IsAbs(c.ResultsDir) || filepath.Clean(c.ResultsDir) == "/" || !digest.MatchString(c.Image) || c.CPUMillis < 1 || c.CPUMillis > 64000 || c.MemoryBytes < 64<<20 || c.MemoryBytes > 128<<30 {
		return domain.E("invalid", "require cluster identity, separate PVCs, absolute results directory, digest-pinned image and bounded resources")
	}
	return nil
}

type Executor struct {
	Config    Config
	Transport Transport
}

func New(c Config) (*Executor, error) {
	if c.Context == "" && os.Getenv("KUBERNETES_SERVICE_HOST") == "" {
		return nil, domain.E("invalid", "host execution requires an explicit Kubernetes context")
	}
	if e := c.Validate(); e != nil {
		return nil, e
	}
	return &Executor{c, Command{c}}, nil
}
func (e *Executor) Validate(p domain.Plan) error {
	if err := e.Config.Validate(); err != nil {
		return err
	}
	if p.Target != "/bench" || p.Environment.Kind != "kubernetes" {
		return domain.E("invalid", "Kubernetes plans require target /bench and a kubernetes environment")
	}
	for key, want := range map[string]string{"cluster": e.Config.Cluster, "node": e.Config.Node, "benchmark_pvc": e.Config.BenchmarkPVC} {
		if p.Environment.Config[key] != want {
			return domain.E("invalid", "environment config must match executor: "+key)
		}
	}
	return nil
}
func (e *Executor) get(ctx context.Context, kind, name string, v any) error {
	b, err := e.Transport.Do(ctx, nil, "get", kind, name, "-o", "json")
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// Preflight is repeated before submission. Scheduling remains the final check of PV topology.
func (e *Executor) Preflight(ctx context.Context) (map[string]any, error) {
	snapshot := map[string]any{"executor": e.Config}
	for _, name := range []string{e.Config.BenchmarkPVC, e.Config.ResultsPVC} {
		var pvc struct {
			Metadata struct {
				UID             string
				ResourceVersion string
			}
			Spec struct {
				AccessModes      []string `json:"accessModes"`
				VolumeName       string   `json:"volumeName"`
				VolumeMode       string   `json:"volumeMode"`
				StorageClassName string   `json:"storageClassName"`
			}
			Status struct{ Phase string }
		}
		if err := e.get(ctx, "pvc", name, &pvc); err != nil {
			return nil, err
		}
		allowed := false
		for _, m := range pvc.Spec.AccessModes {
			if m == "ReadWriteMany" || (name == e.Config.BenchmarkPVC && m == "ReadWriteOnce") {
				allowed = true
			}
		}
		if !allowed || pvc.Status.Phase != "Bound" || pvc.Spec.VolumeMode == "Block" {
			return nil, domain.E("invalid", "PVC must be Bound filesystem; results require ReadWriteMany: "+name)
		}
		snapshot[name] = pvc
	}
	var node struct {
		Metadata struct {
			UID    string
			Labels map[string]string
		}
		Spec   struct{ Unschedulable bool }
		Status struct {
			Conditions  []struct{ Type, Status string }
			NodeInfo    map[string]any    `json:"nodeInfo"`
			Capacity    map[string]string `json:"capacity"`
			Allocatable map[string]string `json:"allocatable"`
		}
	}
	if err := e.get(ctx, "node", e.Config.Node, &node); err != nil {
		return nil, err
	}
	ready := false
	for _, condition := range node.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			ready = true
		}
	}
	if !ready || node.Spec.Unschedulable || node.Metadata.Labels["kubernetes.io/hostname"] != e.Config.Node {
		return nil, domain.E("invalid", "configured node is not Ready and schedulable")
	}
	snapshot["node"] = node
	return snapshot, nil
}
