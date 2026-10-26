package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"slices"
	"time"
)

const Schema = "1.0"
const Version = "0.1.0-dev"

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string     { return e.Message }
func E(code, message string) error { return &Error{code, message} }
func ID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + "-" + hex.EncodeToString(b)
}

var validID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,99}$`)

func CheckID(id string) error {
	if !validID.MatchString(id) {
		return E("invalid", "invalid identifier")
	}
	return nil
}
func Hash(v any) string         { b, _ := json.Marshal(v); return BytesHash(b) }
func BytesHash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

type Machine struct {
	Role        string `json:"role"`
	Model       string `json:"model"`
	Count       int    `json:"count"`
	CPU         string `json:"cpu"`
	MemoryBytes int64  `json:"memory_bytes"`
	Storage     string `json:"storage"`
	Network     string `json:"network"`
}
type Environment struct {
	SchemaVersion string            `json:"schema_version"`
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Kind          string            `json:"kind"`
	Provider      string            `json:"provider"`
	Region        string            `json:"region"`
	Machines      []Machine         `json:"machines"`
	OS            string            `json:"os"`
	Config        map[string]string `json:"config"`
}

func (e Environment) Validate() error {
	if e.SchemaVersion != Schema {
		return E("invalid", "unsupported environment schema")
	}
	if err := CheckID(e.ID); err != nil {
		return err
	}
	if e.Name == "" || !slices.Contains([]string{"cloud", "bare-metal", "local", "kubernetes"}, e.Kind) || len(e.Machines) == 0 {
		return E("invalid", "environment requires name, kind and machines")
	}
	for _, m := range e.Machines {
		if m.Count < 1 || m.Role == "" || m.Model == "" || m.MemoryBytes < 0 {
			return E("invalid", "invalid machine specification")
		}
	}
	return nil
}

type SoftwareVersion struct {
	SchemaVersion string `json:"schema_version"`
	ID            string `json:"id"`
	Filesystem    string `json:"filesystem"`
	Label         string `json:"label"`
	Commit        string `json:"commit"`
	Build         string `json:"build"`
}

func (v SoftwareVersion) Validate() error {
	if v.SchemaVersion != Schema || v.Filesystem == "" || v.Label == "" {
		return E("invalid", "version requires schema, filesystem and label")
	}
	return CheckID(v.ID)
}

type Workload struct {
	Operation      string `json:"operation"`
	FileSizeBytes  int64  `json:"file_size_bytes"`
	FilesPerJob    int    `json:"files_per_job"`
	BlockSizeBytes int64  `json:"block_size_bytes"`
	ReadPercent    int    `json:"read_percent"`
	IOEngine       string `json:"ioengine"`
	Direct         bool   `json:"direct"`
	CacheState     string `json:"cache_state"`
	FsyncEvery     int    `json:"fsync_every"`
	RuntimeSeconds int    `json:"runtime_seconds"`
	WarmupSeconds  int    `json:"warmup_seconds"`
	Seed           int    `json:"seed"`
}
type Matrix struct {
	NumJobs []int `json:"numjobs"`
	IODepth []int `json:"iodepth"`
}
type Spec struct {
	SchemaVersion string            `json:"schema_version"`
	Name          string            `json:"name"`
	Kind          string            `json:"kind"`
	Workload      Workload          `json:"workload"`
	Matrix        Matrix            `json:"matrix"`
	Repeats       int               `json:"repeats"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

func (s Spec) Validate() error {
	if s.SchemaVersion != Schema || s.Name == "" || s.Repeats < 1 || s.Repeats > 20 {
		return E("invalid", "invalid spec schema, name or repeats (1..20)")
	}
	if s.Kind == "metadata" {
		if s.Metadata["source"] != "mdtest-4.0.0" || s.Metadata["mode"] != "files" || s.Metadata["tasks"] != "1" || s.Metadata["files_per_task"] == "" {
			return E("invalid", "metadata conditions are required")
		}
		return nil
	}
	if s.Kind != "data" {
		return E("invalid", "kind must be data or metadata")
	}
	w := s.Workload
	if !slices.Contains([]string{"read", "write", "randread", "randwrite", "randrw"}, w.Operation) {
		return E("invalid", "unsupported I/O operation")
	}
	if w.FileSizeBytes < 4096 || w.FileSizeBytes > 1<<40 || w.BlockSizeBytes < 512 || w.BlockSizeBytes > w.FileSizeBytes || w.FileSizeBytes%w.BlockSizeBytes != 0 || w.BlockSizeBytes%512 != 0 {
		return E("invalid", "file/block sizes must align to 512 bytes and file size must be divisible by block size")
	}
	if w.FilesPerJob < 1 || w.FilesPerJob > 10000 || w.RuntimeSeconds < 1 || w.RuntimeSeconds > 86400 || w.WarmupSeconds < 0 || w.WarmupSeconds > 3600 || w.FsyncEvery < 0 || w.ReadPercent < 0 || w.ReadPercent > 100 {
		return E("invalid", "invalid workload limits")
	}
	if !slices.Contains([]string{"sync", "psync", "libaio", "io_uring"}, w.IOEngine) || !slices.Contains([]string{"warm", "cold-requested", "cold-verified", "unknown"}, w.CacheState) {
		return E("invalid", "unsupported engine or cache state")
	}
	if len(s.Matrix.NumJobs) == 0 || len(s.Matrix.IODepth) == 0 || len(s.Matrix.NumJobs)*len(s.Matrix.IODepth)*s.Repeats > 128 {
		return E("invalid", "matrix must contain 1..128 trials")
	}
	for _, n := range s.Matrix.NumJobs {
		if n < 1 || n > 256 {
			return E("invalid", "numjobs must be 1..256")
		}
	}
	for _, d := range s.Matrix.IODepth {
		if d < 1 || d > 256 {
			return E("invalid", "iodepth must be 1..256")
		}
		if (w.IOEngine == "sync" || w.IOEngine == "psync") && d != 1 {
			return E("invalid", "synchronous engines require iodepth=1")
		}
	}
	if hasDuplicates(s.Matrix.NumJobs) || hasDuplicates(s.Matrix.IODepth) {
		return E("invalid", "matrix values must be unique")
	}
	return nil
}
func hasDuplicates(v []int) bool {
	m := map[int]bool{}
	for _, n := range v {
		if m[n] {
			return true
		}
		m[n] = true
	}
	return false
}

type Point struct {
	ID      string `json:"id"`
	NumJobs int    `json:"numjobs"`
	IODepth int    `json:"iodepth"`
	Repeat  int    `json:"repeat"`
}
type Plan struct {
	SchemaVersion string          `json:"schema_version"`
	Spec          Spec            `json:"spec"`
	Environment   Environment     `json:"environment"`
	Version       SoftwareVersion `json:"version"`
	Target        string          `json:"target"`
	ResourceGroup string          `json:"resource_group"`
	Trials        []Point         `json:"trials"`
	Hash          string          `json:"hash"`
}

func NewPlan(s Spec, e Environment, v SoftwareVersion, target, group string) (Plan, error) {
	p := Plan{SchemaVersion: Schema, Spec: s, Environment: e, Version: v, Target: filepath.Clean(target), ResourceGroup: group}
	if s.Kind == "data" {
		for _, n := range s.Matrix.NumJobs {
			for _, d := range s.Matrix.IODepth {
				for r := 1; r <= s.Repeats; r++ {
					p.Trials = append(p.Trials, Point{fmt.Sprintf("n%d-d%d-r%d", n, d, r), n, d, r})
				}
			}
		}
	}
	if s.Kind == "metadata" {
		for r := 1; r <= s.Repeats; r++ {
			p.Trials = append(p.Trials, Point{fmt.Sprintf("metadata-r%d", r), 1, 1, r})
		}
	}
	p.Hash = p.Digest()
	return p, p.Validate()
}
func (p Plan) Digest() string { p.Hash = ""; return Hash(p) }
func (p Plan) Validate() error {
	if p.SchemaVersion != Schema || p.Hash != p.Digest() {
		return E("invalid", "plan schema or checksum mismatch")
	}
	for _, err := range []error{p.Spec.Validate(), p.Environment.Validate(), p.Version.Validate(), CheckID(p.ResourceGroup)} {
		if err != nil {
			return err
		}
	}
	if !filepath.IsAbs(p.Target) || filepath.Clean(p.Target) == string(filepath.Separator) {
		return E("invalid", "target must be an absolute non-root directory")
	}
	if p.Spec.Kind == "data" {
		var expected []Point
		for _, n := range p.Spec.Matrix.NumJobs {
			for _, d := range p.Spec.Matrix.IODepth {
				for r := 1; r <= p.Spec.Repeats; r++ {
					expected = append(expected, Point{fmt.Sprintf("n%d-d%d-r%d", n, d, r), n, d, r})
				}
			}
		}
		if Hash(expected) != Hash(p.Trials) {
			return E("invalid", "plan trials do not match spec")
		}
	}
	if p.Spec.Kind == "metadata" {
		var expected []Point
		for r := 1; r <= p.Spec.Repeats; r++ {
			expected = append(expected, Point{fmt.Sprintf("metadata-r%d", r), 1, 1, r})
		}
		if Hash(expected) != Hash(p.Trials) {
			return E("invalid", "metadata trials do not match repeats")
		}
	}
	return nil
}

type Metric struct {
	Operation string  `json:"operation"`
	Name      string  `json:"name"`
	Unit      string  `json:"unit"`
	Value     float64 `json:"value"`
	Source    string  `json:"source"`
}

func (m Metric) Validate() error {
	units := map[string]string{"throughput": "bytes/s", "iops": "ops/s", "clat_p99": "ns", "metadata_rate": "ops/s"}
	if units[m.Name] != m.Unit || m.Operation == "" || math.IsNaN(m.Value) || math.IsInf(m.Value, 0) || m.Value < 0 {
		return E("invalid", "invalid metric name/unit/value")
	}
	return nil
}

type Trial struct {
	Point      Point     `json:"point"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Metrics    []Metric  `json:"metrics"`
	Error      string    `json:"error,omitempty"`
}
type Attempt struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	Executor    string    `json:"executor"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	ResultState string    `json:"result_state"`
}
type Run struct {
	SchemaVersion string    `json:"schema_version"`
	ID            string    `json:"id"`
	RequestID     string    `json:"request_id"`
	Source        string    `json:"source"`
	Plan          Plan      `json:"plan"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Attempt       Attempt   `json:"attempt"`
	ToolVersion   string    `json:"tool_version"`
	MaltaVersion  string    `json:"malta_version"`
	Trials        []Trial   `json:"trials"`
	Error         string    `json:"error,omitempty"`
}

func Terminal(s string) bool {
	return slices.Contains([]string{"succeeded", "failed", "cancelled", "interrupted"}, s)
}
func NewRun(p Plan, source string) Run {
	now := time.Now().UTC()
	return Run{SchemaVersion: Schema, ID: ID("run"), Source: source, Plan: p, Status: "pending", CreatedAt: now, UpdatedAt: now, MaltaVersion: Version, Trials: []Trial{}, Attempt: Attempt{ID: ID("attempt"), Status: "pending", Executor: "local", ResultState: "pending"}}
}
func (r Run) Validate() error {
	if r.SchemaVersion != Schema {
		return E("invalid", "unsupported run schema")
	}
	if err := CheckID(r.ID); err != nil {
		return err
	}
	if err := CheckID(r.Attempt.ID); err != nil {
		return err
	}
	if err := r.Plan.Validate(); err != nil {
		return err
	}
	if !slices.Contains([]string{"pending", "preparing", "running", "cancelling", "succeeded", "failed", "cancelled", "interrupted"}, r.Status) {
		return E("invalid", "invalid run status")
	}
	seen := map[string]bool{}
	expected := map[string]Point{}
	for _, p := range r.Plan.Trials {
		expected[p.ID] = p
	}
	for _, t := range r.Trials {
		if p, ok := expected[t.Point.ID]; !ok || p != t.Point {
			return E("invalid", "unexpected trial point")
		}
		if seen[t.Point.ID] {
			return E("invalid", "duplicate trial")
		}
		seen[t.Point.ID] = true
		metrics := map[string]bool{}
		for _, m := range t.Metrics {
			key := m.Operation + "/" + m.Name
			if metrics[key] {
				return E("invalid", "duplicate metric")
			}
			metrics[key] = true
			if err := m.Validate(); err != nil {
				return err
			}
		}
		if r.Status == "succeeded" && (t.Status != "succeeded" || len(t.Metrics) == 0) {
			return E("invalid", "successful run contains incomplete trial")
		}
	}
	if r.Status == "succeeded" && (len(r.Trials) != len(r.Plan.Trials) || len(r.Trials) == 0 || r.ToolVersion == "") {
		return E("invalid", "successful run must contain trials")
	}
	return nil
}
