package workflow

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/wsq2333/malta/internal/backend"
	"github.com/wsq2333/malta/internal/backend/uce"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/engine/fio"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// readEvidence uses a rooted filesystem handle. Symlinks and parent traversal are
// rejected; path registration is never interpreted as file-integrity proof.
func readEvidence(root, path string, limit int64) ([]byte, error) {
	if !fs.ValidPath(filepath.ToSlash(path)) || strings.Contains(path, "\\") {
		return nil, domain.E("invalid", "unsafe artifact path")
	}
	dir, e := os.OpenRoot(root)
	if e != nil {
		return nil, e
	}
	defer dir.Close()
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i := range parts {
		st, e := dir.Lstat(strings.Join(parts[:i+1], "/"))
		if e != nil {
			return nil, e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return nil, domain.E("invalid", "symlink evidence rejected")
		}
	}
	f, e := dir.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Size() > limit {
		return nil, domain.E("invalid", "artifact size/type rejected")
	}
	data, e := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(data)) > limit {
		return nil, domain.E("invalid", "artifact budget exceeded")
	}
	return data, e
}
func (m *Manager) archive(id, name string, data []byte) error {
	dir := filepath.Join(m.Store.Dir, "workflow-artifacts", id)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	// Content-addressed filenames allow collection retries without replacing evidence.
	dest := filepath.Join(dir, name)
	if old, e := os.ReadFile(dest); e == nil {
		if domain.BytesHash(old) != domain.BytesHash(data) {
			return domain.E("conflict", "archived evidence changed")
		}
		return nil
	}
	f, e := os.CreateTemp(dir, ".collect-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(tmp, dest)
}
func (m *Manager) Artifact(id, path string) ([]byte, error) {
	if _, e := m.Get(id); e != nil {
		return nil, e
	}
	if filepath.Base(path) != path {
		return nil, domain.E("invalid", "only archived filenames accepted")
	}
	return readEvidence(filepath.Join(m.Store.Dir, "workflow-artifacts", id), path, 32<<20)
}
func (m *Manager) collect(r *Run, o backend.Observation, b backend.Backend, cfg uce.Config) ([]Result, bool, bool, bool) {
	results := []Result{}
	complete, invalid, passed := true, false, true
	used := int64(0)
	for _, rule := range r.Snapshot.Definition.Results {
		rr := Result{NodeID: rule.NodeID, Parser: rule.Parser, State: "missing", Verdict: "inconclusive", Metrics: []domain.Metric{}, Files: []EvidenceFile{}}
		var stage backend.Stage
		found := false
		for _, s := range o.Stages {
			if s.NodeID == rule.NodeID && (!found || s.Attempt > stage.Attempt) {
				stage = s
				found = true
			}
		}
		rr.StageID = stage.ID
		rr.Attempt = stage.Attempt
		var e error
		if !found || strings.ToLower(stage.Status) != "succeeded" {
			e = fmt.Errorf("required stage has no successful current attempt")
		} else {
			var outputs []backend.Output
			outputs, e = b.Outputs(m.ctx, r.ExternalRunID, stage)
			if e == nil {
				matched := false
				for _, out := range outputs {
					if out.Name != rule.Output {
						continue
					}
					if matched {
						e = domain.E("invalid", "duplicate output registration")
						break
					}
					matched = true
					if out.RunID != r.ExternalRunID || out.StageID != stage.ID || out.Attempt != stage.Attempt {
						e = domain.E("invalid", "UCE output identity mismatch")
						break
					}
					rr, e = m.verify(r, rule, stage, out.Path, cfg, &used)
				}
				if !matched && e == nil {
					e = fmt.Errorf("required output not published")
				}
			}
		}
		if e != nil {
			rr.Error = e.Error()
			var de *domain.Error
			if errors.As(e, &de) && de.Code == "invalid" {
				rr.State = "invalid"
				if rule.Required {
					invalid = true
				}
			}
			if rule.Required {
				complete = false
				passed = false
			}
		} else if rr.Verdict != "passed" && rule.Required {
			passed = false
		}
		results = append(results, rr)
	}
	return results, complete, invalid, passed
}
func (m *Manager) verify(r *Run, rule ResultRule, stage backend.Stage, remotePath string, cfg uce.Config, used *int64) (Result, error) {
	result := Result{NodeID: rule.NodeID, StageID: stage.ID, Attempt: stage.Attempt, Parser: rule.Parser, State: "missing", Verdict: "inconclusive", Metrics: []domain.Metric{}, Files: []EvidenceFile{}}
	rel, e := filepath.Rel(cfg.RemoteResultsRoot, remotePath)
	if e != nil || !filepath.IsAbs(remotePath) || !fs.ValidPath(filepath.ToSlash(rel)) {
		return result, domain.E("invalid", "output outside approved results root")
	}
	// The UCE contract restricts output paths; also require this exact run identity.
	if !strings.Contains("/"+filepath.ToSlash(rel)+"/", "/"+r.ExternalRunID+"/") {
		return result, domain.E("invalid", "output path belongs to another run")
	}
	raw, e := readEvidence(cfg.LocalResultsRoot, rel, cfg.ArtifactBytes-*used)
	if e != nil {
		return result, e
	}
	*used += int64(len(raw))
	var required map[string]any
	if json.Unmarshal(raw, &required) != nil {
		return result, domain.E("invalid", "invalid evidence JSON")
	}
	for _, key := range []string{"attempt", "eviction_count", "snapshot_hash", "files", "assertions"} {
		if _, ok := required[key]; !ok {
			return result, domain.E("invalid", "missing evidence field: "+key)
		}
	}
	var manifest Evidence
	if e = domain.DecodeJSON(strings.NewReader(string(raw)), &manifest); e != nil {
		return result, domain.E("invalid", "invalid evidence manifest")
	}
	if manifest.SchemaVersion != "malta.evidence/v1" || manifest.UCERunID != r.ExternalRunID || manifest.UCEVersionID != r.ExternalVersionID || manifest.NodeID != rule.NodeID || manifest.StageID != stage.ID || manifest.Attempt != stage.Attempt || manifest.EvictionCount != stage.EvictionCount || manifest.SnapshotHash != r.Snapshot.Hash || manifest.EnvironmentID != r.Snapshot.Environment.ID || manifest.SoftwareVersionID != r.Snapshot.Version.ID || manifest.Parser != rule.Parser {
		return result, domain.E("invalid", "evidence identity/contract mismatch")
	}
	if len(manifest.Files) > 64 {
		return result, domain.E("invalid", "too many evidence files")
	}
	files := map[string][]byte{}
	manifestName := domain.BytesHash(raw) + ".manifest.json"
	if e = m.archive(r.ID, manifestName, raw); e != nil {
		return result, e
	}
	result.Files = append(result.Files, EvidenceFile{manifestName, domain.BytesHash(raw), int64(len(raw))})
	for _, f := range manifest.Files {
		if !fs.ValidPath(f.Path) || files[f.Path] != nil || f.Size < 0 || f.Size > cfg.ArtifactBytes-*used {
			return result, domain.E("invalid", "invalid evidence file declaration")
		}
		data, e := readEvidence(cfg.LocalResultsRoot, filepath.Join(filepath.Dir(rel), f.Path), cfg.ArtifactBytes-*used)
		if e != nil {
			return result, e
		}
		*used += int64(len(data))
		if int64(len(data)) != f.Size || domain.BytesHash(data) != f.SHA256 {
			return result, domain.E("invalid", "evidence checksum or size mismatch")
		}
		files[f.Path] = data
		name := f.SHA256 + "-" + filepath.Base(f.Path)
		if e = m.archive(r.ID, name, data); e != nil {
			return result, e
		}
		result.Files = append(result.Files, EvidenceFile{name, f.SHA256, f.Size})
	}
	pass := true
	result.Assertions = manifest.Assertions
	for _, name := range rule.Assertions {
		v, ok := manifest.Assertions[name]
		if !ok {
			return result, domain.E("invalid", "missing mandatory assertion: "+name)
		}
		if !v {
			pass = false
		}
	}
	switch rule.Parser {
	case "fio-3.33":
		data, ok := files["fio.json"]
		if !ok {
			return result, domain.E("invalid", "fio.json required")
		}
		var v struct {
			Jobs []struct {
				Options map[string]string `json:"job options"`
			} `json:"jobs"`
		}
		if json.Unmarshal(data, &v) != nil || len(v.Jobs) != 1 {
			return result, domain.E("invalid", "one fio aggregate required")
		}
		for k, want := range rule.ExpectedOptions {
			if v.Jobs[0].Options[k] != want {
				return result, domain.E("invalid", "fio actual option mismatch: "+k)
			}
		}
		result.Metrics, e = fio.Parse(data)
		if e != nil {
			return result, domain.E("invalid", "fio evidence rejected: "+e.Error())
		}
	case "junit":
		data, ok := files["junit.xml"]
		if !ok {
			return result, domain.E("invalid", "junit.xml required")
		}
		count, fail, skip, e := parseJUnit(data)
		if e != nil {
			return result, e
		}
		pass = pass && fail == 0 && skip == 0
		result.Metrics = []domain.Metric{{Operation: "tests", Name: "executed", Unit: "count", Value: float64(count), Source: "junit"}, {Operation: "tests", Name: "failed", Unit: "count", Value: float64(fail), Source: "junit"}, {Operation: "tests", Name: "skipped", Unit: "count", Value: float64(skip), Source: "junit"}}
	case "assertions-v1":
		// These are explicitly producer assertions, not independently measured metrics.
		if len(manifest.Metrics) > 0 {
			return result, domain.E("invalid", "assertions-v1 cannot supply unverified performance metrics")
		}
	default:
		return result, domain.E("unsupported", "result parser unavailable")
	}
	result.State = "complete"
	result.Verdict = "passed"
	if !pass {
		result.Verdict = "failed"
	}
	return result, nil
}
func parseJUnit(raw []byte) (int, int, int, error) {
	dec := xml.NewDecoder(strings.NewReader(string(raw)))
	count, failed, skipped := 0, 0, 0
	depth := 0
	inCase := false
	bad, skip := false, false
	seen := map[string]bool{}
	root := ""
	for {
		token, e := dec.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return 0, 0, 0, domain.E("invalid", "invalid JUnit XML")
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				if root != "" || t.Name.Local != "testsuite" && t.Name.Local != "testsuites" {
					return 0, 0, 0, domain.E("invalid", "invalid JUnit root")
				}
				root = t.Name.Local
			}
			if t.Name.Local == "testcase" {
				if inCase {
					return 0, 0, 0, domain.E("invalid", "nested JUnit testcase")
				}
				inCase = true
				bad = false
				skip = false
				name, class := "", ""
				for _, a := range t.Attr {
					if a.Name.Local == "name" {
						name = a.Value
					}
					if a.Name.Local == "classname" {
						class = a.Value
					}
				}
				key := class + "/" + name
				if name == "" || seen[key] {
					return 0, 0, 0, domain.E("invalid", "missing or duplicate JUnit test identity")
				}
				seen[key] = true
				count++
			}
			if inCase && (t.Name.Local == "failure" || t.Name.Local == "error") {
				bad = true
			}
			if inCase && t.Name.Local == "skipped" {
				skip = true
			}
		case xml.EndElement:
			if t.Name.Local == "testcase" {
				if bad {
					failed++
				}
				if skip {
					skipped++
				}
				inCase = false
			}
			depth--
		case xml.Directive:
			return 0, 0, 0, domain.E("invalid", "XML directives not supported")
		}
	}
	if count == 0 || depth != 0 {
		return 0, 0, 0, domain.E("invalid", "JUnit contains no tests or is incomplete")
	}
	return count, failed, skipped, nil
}

// Comparison includes execution conditions and explicit metric names; no automatic
// merging of upload, warmup and read into one throughput value.
func Compare(a, b Run) (map[string]any, error) {
	if a.Status != "succeeded" || b.Status != "succeeded" || a.ResultState != "complete" || b.ResultState != "complete" || a.Verdict != "passed" || b.Verdict != "passed" {
		return nil, domain.E("invalid", "comparison requires two verified successful runs")
	}
	signature := func(r Run) string {
		return domain.Hash(struct {
			Template   string
			Env        domain.Environment
			Profile    Record
			Parameters map[string]any
			Results    []ResultRule
		}{r.Snapshot.TemplateHash, r.Snapshot.Environment, r.Snapshot.Profile, r.Snapshot.Inputs, r.Snapshot.Definition.Results})
	}
	if signature(a) != signature(b) {
		return map[string]any{"comparable": false, "reason": "template, environment, execution profile, parameters or result contract differ"}, nil
	}
	previous := map[string]domain.Metric{}
	for _, r := range a.Results {
		for _, v := range r.Metrics {
			previous[r.NodeID+"/"+v.Operation+"/"+v.Name+"/"+v.Unit] = v
		}
	}
	deltas := []map[string]any{}
	for _, r := range b.Results {
		for _, v := range r.Metrics {
			k := r.NodeID + "/" + v.Operation + "/" + v.Name + "/" + v.Unit
			if x, ok := previous[k]; ok {
				row := map[string]any{"metric": k, "baseline": x.Value, "candidate": v.Value, "delta": v.Value - x.Value}
				if x.Value != 0 {
					row["percent"] = (v.Value - x.Value) / x.Value * 100
				}
				deltas = append(deltas, row)
			}
		}
	}
	return map[string]any{"comparable": true, "metrics": deltas, "warning": "单次样本；差异不自动归因于软件版本"}, nil
}
