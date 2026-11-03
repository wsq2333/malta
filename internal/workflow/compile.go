package workflow

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	js "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/wsq2333/malta/internal/domain"
	"math"
	"slices"
	"strconv"
	"strings"
)

func noRefs(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, v := range x {
			if k == "$ref" || k == "$dynamicRef" {
				return false
			}
			if !noRefs(v) {
				return false
			}
		}
	case []any:
		for _, v := range x {
			if !noRefs(v) {
				return false
			}
		}
	}
	return true
}
func schemaCheck(schema json.RawMessage, v any) error {
	var resource any
	if e := json.Unmarshal(schema, &resource); e != nil {
		return domain.E("invalid", "invalid parameter schema")
	}
	if !noRefs(resource) {
		return domain.E("invalid", "parameter schema references are not supported")
	}
	c := js.NewCompiler()
	if e := c.AddResource("https://malta.local/parameters.json", resource); e != nil {
		return e
	}
	s, e := c.Compile("https://malta.local/parameters.json")
	if e != nil {
		return domain.E("invalid", e.Error())
	}
	if e = s.Validate(v); e != nil {
		return domain.E("invalid", e.Error())
	}
	return nil
}
func array(v any) []any { a, _ := v.([]any); return a }

// validateDefinition validates Malta metadata, not the backend's workflow language.
func validateDefinition(d Definition) error {
	if len(d.Implementations) > 0 {
		if d.Backend != "" || hasJSON(d.Template) || len(d.Bindings) > 0 || len(d.Results) > 0 {
			return domain.E("invalid", "use implementations or a single backend/template, not both")
		}
		for kind := range d.Implementations {
			v, e := definitionFor(d, kind)
			if e != nil {
				return e
			}
			if e = validateDefinition(v); e != nil {
				return e
			}
		}
		return nil
	}
	if d.SchemaVersion != Schema || !slices.Contains([]string{"uce", "argo"}, d.Backend) {
		return domain.E("unsupported", "require workflow/v1alpha1 and uce or argo")
	}
	if d.TimeoutSeconds < 0 || d.TimeoutSeconds > 86400 || len(d.Results) > 64 {
		return domain.E("invalid", "timeout must be 0..86400; maximum 64 result rules")
	}
	if _, e := nativeTemplate(d); e != nil {
		return e
	}
	if hasJSON(d.Parameters) {
		var ps map[string]any
		if json.Unmarshal(d.Parameters, &ps) != nil || ps["type"] != "object" || ps["additionalProperties"] != false || !noRefs(ps) {
			return domain.E("invalid", "optional parameters requires a closed object schema without external references")
		}
	}
	seen := map[string]bool{}
	required := 0
	for _, r := range d.Results {
		key := r.NodeID + "/" + r.Output
		if r.NodeID == "" || r.Output == "" || seen[key] || !slices.Contains([]string{"fio-3.33", "assertions-v1", "junit"}, r.Parser) {
			return domain.E("invalid", "invalid result contract")
		}
		seen[key] = true
		if r.Required {
			required++
		}
		if r.Parser == "assertions-v1" && len(r.Assertions) == 0 {
			return domain.E("invalid", "assertion result requires named assertions")
		}
		if r.Parser == "fio-3.33" {
			for _, k := range []string{"rw", "bs", "numjobs", "iodepth", "ioengine", "direct", "runtime", "allow_file_create"} {
				if r.ExpectedOptions[k] == "" && r.OptionParameters[k] == "" {
					return domain.E("invalid", "fio contract must verify "+k)
				}
			}
			if r.ExpectedOptions["allow_file_create"] != "0" {
				return domain.E("invalid", "existing dataset requires allow_file_create=0")
			}
		}
	}
	if len(d.Results) > 0 && required == 0 {
		return domain.E("invalid", "at least one mandatory result required when parsing is configured")
	}
	for _, b := range d.Bindings {
		if b.Source != "system" && b.Source != "profile" {
			return domain.E("invalid", "binding source must be system or profile")
		}
		if (d.Backend == "uce" && (!strings.HasPrefix(b.Pointer, "/operator_definitions/") || !strings.Contains(b.Pointer, "/manifest/spec/"))) || (d.Backend == "argo" && !strings.HasPrefix(b.Pointer, "/spec/")) {
			return domain.E("invalid", "bindings must target native workload spec fields")
		}
	}
	return nil
}
func nativeTemplate(d Definition) (map[string]any, error) {
	var root map[string]any
	if json.Unmarshal(d.Template, &root) != nil || root == nil {
		return nil, domain.E("invalid", "native template must be a JSON object")
	}
	if d.Backend == "argo" {
		if root["apiVersion"] != "argoproj.io/v1alpha1" || !slices.Contains([]any{"Workflow", "WorkflowTemplate"}, root["kind"]) || object(root["spec"]) == nil {
			return nil, domain.E("invalid", "require a native Argo Workflow or WorkflowTemplate")
		}
		if root["status"] != nil {
			return nil, domain.E("invalid", "provide an Argo definition without runtime status")
		}
	} else {
		id, _ := root["root_dag_version_id"].(string)
		if id == "" {
			return nil, domain.E("invalid", "UCE EP requires root_dag_version_id")
		}
		if rootDAG(root) == nil {
			return nil, domain.E("invalid", "UCE root DAG definition is missing")
		}
	}
	return root, nil
}
func rootDAG(root map[string]any) map[string]any {
	for _, v := range array(root["dag_definitions"]) {
		d := object(v)
		if d["dag_version_id"] == root["root_dag_version_id"] {
			return d
		}
	}
	return nil
}
func declaresContext(root map[string]any, kind string) bool {
	if kind == "argo" {
		for _, v := range array(object(object(root["spec"])["arguments"])["parameters"]) {
			if object(v)["name"] == "malta_context" {
				return true
			}
		}
	} else {
		for _, v := range array(rootDAG(root)["inputs"]) {
			if object(v)["name"] == "malta_context" && object(v)["type"] == "json" {
				return true
			}
		}
	}
	return false
}
func replace(root any, pointer string, value any) error {
	if !strings.HasPrefix(pointer, "/") {
		return domain.E("invalid", "binding must be a JSON pointer")
	}
	parts := strings.Split(pointer[1:], "/")
	cur := root
	for i, p := range parts {
		p = strings.ReplaceAll(strings.ReplaceAll(p, "~1", "/"), "~0", "~")
		last := i == len(parts)-1
		switch x := cur.(type) {
		case map[string]any:
			v, ok := x[p]
			if !ok {
				return domain.E("invalid", "binding target does not exist: "+pointer)
			}
			if last {
				x[p] = value
				return nil
			}
			cur = v
		case []any:
			n, e := strconv.Atoi(p)
			if e != nil || n < 0 || n >= len(x) {
				return domain.E("invalid", "invalid binding array index")
			}
			if last {
				x[n] = value
				return nil
			}
			cur = x[n]
		default:
			return domain.E("invalid", "binding crosses scalar")
		}
	}
	return nil
}
func number(v any) (int64, error) {
	f, ok := v.(float64)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) || f < 1 || f > 1<<53 || f != math.Trunc(f) {
		return 0, domain.E("invalid", "dataset parameters must be positive exact integers")
	}
	return int64(f), nil
}
func (m *Manager) compile(q Selection) (Snapshot, error) {
	var s Snapshot
	var e error
	s.Selection = q
	if s.Case, e = m.Record("cases", q.Case); e != nil {
		return s, e
	}
	if s.System, e = m.Record("systems", q.System); e != nil {
		return s, e
	}
	if s.Profile, e = m.Record("profiles", q.Profile); e != nil {
		return s, e
	}
	var sys System
	var p Profile
	_ = json.Unmarshal(s.Case.Body, &s.Definition)
	_ = json.Unmarshal(s.System.Body, &sys)
	_ = json.Unmarshal(s.Profile.Body, &p)
	if s.Definition, e = definitionFor(s.Definition, p.Backend); e != nil {
		return s, e
	}
	if e = validateDefinition(s.Definition); e != nil {
		return s, e
	}
	if !slices.Contains(p.SystemIDs, s.System.ID) || sys.SoftwareVersionID != q.VersionID {
		return s, domain.E("invalid", "profile/system/software selection mismatch")
	}
	if e = m.Store.Object("environment", sys.EnvironmentID, &s.Environment); e != nil {
		return s, e
	}
	if e = m.Store.Object("version", q.VersionID, &s.Version); e != nil {
		return s, e
	}
	if s.Environment.Config["cluster"] != p.Cluster || (p.Node != "" && s.Environment.Config["node"] != p.Node) {
		return s, domain.E("invalid", "environment snapshot differs from execution profile cluster/node")
	}
	cfg, ok := m.connections[p.Connection]
	if !ok {
		return s, domain.E("invalid", "workflow connection is not configured")
	}
	if !cfg.Active() || cfg.Kind() != p.Backend || cfg.Cluster != p.Cluster || cfg.Namespace != p.Namespace || (p.Backend == "uce" && !slices.Contains(cfg.Subtypes, p.Subtype)) {
		return s, domain.E("invalid", "profile differs from enabled backend/route scope")
	}
	if len(s.Definition.Results) > 0 && (cfg.RemoteResultsRoot == "" || cfg.LocalResultsRoot == "") {
		return s, domain.E("invalid", "result parsing requires verified shared result roots")
	}
	s.ConnectionHash = connectionHash(cfg)
	if _, ok := q.Parameters["malta_context"]; ok {
		return s, domain.E("invalid", "malta_context is reserved")
	}
	s.Inputs = map[string]any{}
	for k, v := range s.Definition.Defaults {
		s.Inputs[k] = v
	}
	for k, v := range q.Parameters {
		s.Inputs[k] = v
	}
	if hasJSON(s.Definition.Parameters) {
		if e = schemaCheck(s.Definition.Parameters, s.Inputs); e != nil {
			return s, e
		}
	}
	if s.Definition.Dataset.FileCountParameter != "" || s.Definition.Dataset.FileSizeParameter != "" {
		size, e := number(s.Inputs[s.Definition.Dataset.FileSizeParameter])
		if e != nil {
			return s, e
		}
		count, e := number(s.Inputs[s.Definition.Dataset.FileCountParameter])
		if e != nil {
			return s, e
		}
		if size > math.MaxInt64/count {
			return s, domain.E("invalid", "dataset size overflows")
		}
		s.PreparedBytes = size * count
	}
	if s.Definition.TimeoutSeconds == 0 {
		s.Definition.TimeoutSeconds = min(600, cfg.TimeoutSeconds)
	}
	if s.PreparedBytes > cfg.MaxBytes || s.Definition.TimeoutSeconds > cfg.TimeoutSeconds {
		return s, domain.E("invalid", "workflow exceeds connection data or time budget")
	}
	var ep any
	_ = json.Unmarshal(s.Definition.Template, &ep)
	for _, b := range s.Definition.Bindings {
		settings := sys.Settings
		if b.Source == "profile" {
			settings = p.Settings
		}
		v, ok := settings[b.Key]
		if !ok {
			return s, domain.E("invalid", "missing binding setting: "+b.Key)
		}
		if e = replace(ep, b.Pointer, v); e != nil {
			return s, e
		}
	}
	root := object(ep)
	if p.Backend == "argo" {
		if ns := object(root["metadata"])["namespace"]; ns != nil && ns != p.Namespace {
			return s, domain.E("invalid", "Argo template namespace differs from execution configuration")
		}
	} else {
		// Binding changes require a new immutable UCE root version; leave native DAG semantics untouched.
		dag := rootDAG(root)
		if dag == nil {
			return s, domain.E("invalid", "bound EP root missing")
		}
		id := uuid.NewSHA1(uuid.NameSpaceURL, []byte("malta-ep:"+domain.Hash(ep))).String()
		root["root_dag_version_id"] = id
		dag["dag_version_id"] = id
	}
	if len(s.Definition.Results) > 0 && !declaresContext(root, p.Backend) {
		return s, domain.E("invalid", "result parsing requires a declared malta_context input")
	}
	s.TemplateHash = domain.Hash(json.RawMessage(s.Definition.Template))
	s.EP, _ = json.Marshal(ep)
	s.EPHash = domain.Hash(s.EP)
	bound := s.Definition
	bound.Template = s.EP
	if e = validateDefinition(bound); e != nil {
		return s, e
	}
	for i, r := range s.Definition.Results {
		if r.ExpectedOptions == nil {
			r.ExpectedOptions = map[string]string{}
		}
		for option, param := range r.OptionParameters {
			v, ok := s.Inputs[param]
			if !ok {
				return s, domain.E("invalid", "missing fio option parameter")
			}
			r.ExpectedOptions[option] = fmt.Sprint(v)
		}
		s.Definition.Results[i] = r
	}
	s.Hash = domain.Hash(s)
	return s, nil
}

func object(v any) map[string]any { m, _ := v.(map[string]any); return m }

func hasJSON(v json.RawMessage) bool { return len(v) > 0 && strings.TrimSpace(string(v)) != "null" }
