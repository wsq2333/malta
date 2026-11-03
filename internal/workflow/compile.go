package workflow

import (
	"embed"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	js "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/wsq2333/malta/internal/domain"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Schemas pinned to UCE source 0506eff. Server registration remains authoritative.
//
//go:embed schemas/*.json
var schemas embed.FS

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
func epCheck(raw json.RawMessage) error {
	var v map[string]any
	if e := json.Unmarshal(raw, &v); e != nil {
		return domain.E("invalid", "EP must be a JSON object")
	}
	c := js.NewCompiler()
	for _, name := range []string{"authoring-execution-plan-0.1.0.json", "pod-spec-0.1.0.json"} {
		data, _ := schemas.ReadFile("schemas/" + name)
		var r any
		_ = json.Unmarshal(data, &r)
		if e := c.AddResource("https://malta.local/"+name, r); e != nil {
			return e
		}
	}
	s, e := c.Compile("https://malta.local/authoring-execution-plan-0.1.0.json")
	if e != nil {
		return e
	}
	if e = s.Validate(v); e != nil {
		return domain.E("invalid", e.Error())
	}
	if v["artifact_contract"] != "paths-v1" {
		return domain.E("invalid", "workflow tests require paths-v1 evidence")
	}
	dags := v["dag_definitions"].([]any)
	dag := dags[0].(map[string]any)
	if len(array(dag["nodes"])) > 64 {
		return domain.E("invalid", "maximum 64 workflow nodes")
	}
	if dag["dag_version_id"] != v["root_dag_version_id"] {
		return domain.E("invalid", "EP root identity differs")
	}
	ops := map[string]map[string]any{}
	for _, o := range v["operator_definitions"].([]any) {
		op := o.(map[string]any)
		id := op["operator_version_id"].(string)
		if ops[id] != nil {
			return domain.E("invalid", "duplicate operator identity")
		}
		if ex, ok := op["execution"].(map[string]any); ok {
			for _, k := range []string{"maxRetry", "retryLimit"} {
				if n, ok := ex[k].(float64); ok && n != 0 {
					return domain.E("invalid", "workflow tests require execution maxRetry=0")
				}
			}
		}
		ops[id] = op
	}
	nodes := map[string]map[string]any{}
	for _, n := range dag["nodes"].([]any) {
		node := n.(map[string]any)
		id := node["definition_node_id"].(string)
		if nodes[id] != nil || node["kind"] != "operator" || ops[node["target_version_id"].(string)] == nil {
			return domain.E("invalid", "node/operator references must resolve uniquely")
		}
		for _, b := range object(node["input_bindings"]) {
			if object(b)["kind"] == "node_output" {
				return domain.E("invalid", "paths-v1 requires input_path dependencies, not node_output")
			}
		}
		nodes[id] = node
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return domain.E("invalid", "DAG cycle")
		}
		if done[id] {
			return nil
		}
		visiting[id] = true
		for _, p := range array(nodes[id]["input_path"]) {
			ref := p.(map[string]any)
			parent, _ := ref["from_node"].(string)
			if nodes[parent] == nil {
				return domain.E("invalid", "unknown input_path node")
			}
			op := ops[nodes[parent]["target_version_id"].(string)]
			found := false
			for _, out := range array(op["output_path"]) {
				if out.(map[string]any)["name"] == ref["output"] {
					found = true
				}
			}
			if !found {
				return domain.E("invalid", "input_path output missing")
			}
			if e := visit(parent); e != nil {
				return e
			}
		}
		visiting[id] = false
		done[id] = true
		return nil
	}
	for id := range nodes {
		if e := visit(id); e != nil {
			return e
		}
	}
	return nil
}
func array(v any) []any { a, _ := v.([]any); return a }
func validateDefinition(d Definition) error {
	if d.SchemaVersion != Schema || d.Backend != "uce" {
		return domain.E("unsupported", "only workflow/v1alpha1 with UCE EP is executable")
	}
	if d.TimeoutSeconds < 1 || d.TimeoutSeconds > 86400 || len(d.Results) == 0 || len(d.Results) > 64 {
		return domain.E("invalid", "require timeout 1..86400 and 1..64 result contracts")
	}
	if e := epCheck(d.Template); e != nil {
		return e
	}
	if len(d.Parameters) == 0 {
		return domain.E("invalid", "parameter schema required")
	}
	// Validate schema without requiring defaults to fill every mandatory parameter.
	var ps map[string]any
	if json.Unmarshal(d.Parameters, &ps) != nil || ps["type"] != "object" || ps["additionalProperties"] != false {
		return domain.E("invalid", "parameters requires a closed object schema")
	}
	if !noRefs(ps) {
		return domain.E("invalid", "external schema references forbidden")
	}
	var ep map[string]any
	_ = json.Unmarshal(d.Template, &ep)
	dag := array(ep["dag_definitions"])[0].(map[string]any)
	nodes := map[string]map[string]any{}
	ops := map[string]map[string]any{}
	for _, n := range array(dag["nodes"]) {
		v := n.(map[string]any)
		nodes[v["definition_node_id"].(string)] = v
	}
	for _, o := range array(ep["operator_definitions"]) {
		v := o.(map[string]any)
		ops[v["operator_version_id"].(string)] = v
	}
	seen := map[string]bool{}
	required := 0
	for _, r := range d.Results {
		key := r.NodeID + "/" + r.Output
		if seen[key] || nodes[r.NodeID] == nil || !slices.Contains([]string{"fio-3.33", "assertions-v1", "junit"}, r.Parser) {
			return domain.E("invalid", "invalid result contract")
		}
		seen[key] = true
		found := false
		op := ops[nodes[r.NodeID]["target_version_id"].(string)]
		for _, v := range array(op["output_path"]) {
			if v.(map[string]any)["name"] == r.Output {
				found = true
			}
		}
		if !found {
			return domain.E("invalid", "result output not declared in EP")
		}
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
	if required == 0 {
		return domain.E("invalid", "at least one mandatory result required")
	}
	for _, b := range d.Bindings {
		if b.Source != "system" && b.Source != "profile" {
			return domain.E("invalid", "binding source must be system or profile")
		}
		if !strings.HasPrefix(b.Pointer, "/operator_definitions/") || !strings.Contains(b.Pointer, "/manifest/spec/") {
			return domain.E("invalid", "bindings only target existing operator manifest spec fields")
		}
	}
	return nil
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
	if e = validateDefinition(s.Definition); e != nil {
		return s, e
	}
	if p.Backend != "uce" || !slices.Contains(p.SystemIDs, s.System.ID) || sys.SoftwareVersionID != q.VersionID {
		return s, domain.E("invalid", "profile/system/software selection mismatch")
	}
	if e = m.Store.Object("environment", sys.EnvironmentID, &s.Environment); e != nil {
		return s, e
	}
	if e = m.Store.Object("version", q.VersionID, &s.Version); e != nil {
		return s, e
	}
	if s.Environment.Config["cluster"] != p.Cluster || s.Environment.Config["node"] != p.Node {
		return s, domain.E("invalid", "environment snapshot differs from execution profile cluster/node")
	}
	cfg, ok := m.connections[p.Connection]
	if !ok {
		return s, domain.E("invalid", "UCE connection is not configured")
	}
	if cfg.Cluster != p.Cluster || cfg.Namespace != p.Namespace || !slices.Contains(cfg.Subtypes, p.Subtype) {
		return s, domain.E("invalid", "profile differs from approved UCE route scope")
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
	if e = schemaCheck(s.Definition.Parameters, s.Inputs); e != nil {
		return s, e
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
	if e = validatePlacement(ep.(map[string]any), p, cfg.RemoteResultsRoot); e != nil {
		return s, e
	}
	s.TemplateHash = domain.Hash(json.RawMessage(s.Definition.Template))
	// Content-derived publish identities make changed bindings new immutable versions.
	bindingHash := domain.Hash(ep)
	root := ep.(map[string]any)
	dag := array(root["dag_definitions"])[0].(map[string]any)
	newID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("malta-ep:"+bindingHash)).String()
	root["root_dag_version_id"] = newID
	dag["dag_version_id"] = newID
	s.EP, _ = json.Marshal(ep)
	if e = epCheck(s.EP); e != nil {
		return s, e
	}
	s.EPHash = domain.Hash(s.EP)
	// Validate root input names/types in addition to the Malta form schema.
	declared := map[string]bool{}
	inputIDs := map[string]bool{}
	for _, v := range array(dag["inputs"]) {
		i := v.(map[string]any)
		name := i["name"].(string)
		id := i["id"].(string)
		if declared[name] || inputIDs[id] {
			return s, domain.E("invalid", "duplicate root input")
		}
		declared[name], inputIDs[id] = true, true
		if name == "malta_context" {
			if i["type"] != "json" {
				return s, domain.E("invalid", "malta_context must be json")
			}
			continue
		}
		value, present := s.Inputs[name]
		if !present {
			value, present = i["default"]
			if present {
				return s, domain.E("invalid", "declare EP defaults in Malta defaults too: "+name)
			}
		}
		if !present && i["required"] != false {
			return s, domain.E("invalid", "missing EP input: "+name)
		}
		if present && !inputType(i["type"].(string), value) {
			return s, domain.E("invalid", "EP input type differs: "+name)
		}
	}
	if !declared["malta_context"] {
		return s, domain.E("invalid", "EP must declare the reserved malta_context JSON input")
	}
	for k := range s.Inputs {
		if !declared[k] {
			return s, domain.E("invalid", "parameter not declared by EP: "+k)
		}
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
func inputType(kind string, v any) bool {
	if strings.HasPrefix(kind, "list[") {
		list, ok := v.([]any)
		if !ok {
			return false
		}
		for _, item := range list {
			if !inputType(strings.TrimSuffix(strings.TrimPrefix(kind, "list["), "]"), item) {
				return false
			}
		}
		return true
	}
	switch kind {
	case "json":
		return v != nil
	case "bool", "boolean":
		_, ok := v.(bool)
		return ok
	case "str", "string":
		_, ok := v.(string)
		return ok
	case "object", "dict":
		_, ok := v.(map[string]any)
		return ok
	case "array", "list":
		_, ok := v.([]any)
		return ok
	case "float", "number":
		f, ok := v.(float64)
		return ok && !math.IsNaN(f) && !math.IsInf(f, 0)
	case "int", "integer":
		f, ok := v.(float64)
		return ok && math.Abs(f) <= 1<<53 && math.Trunc(f) == f
	}
	return false
}

// Pin every stage to the declared client and verify a writable shared result mount.
// This verifies the submitted manifest, not live cluster/PVC contents or routing.
func validatePlacement(ep map[string]any, p Profile, root string) error {
	pvc, _ := p.Settings["results_pvc"].(string)
	mount, _ := p.Settings["results_mount"].(string)
	if pvc == "" || mount != root {
		return domain.E("invalid", "profile results_pvc and results_mount must match the approved result root")
	}
	for _, v := range array(ep["operator_definitions"]) {
		op := object(v)
		spec := object(object(op["manifest"])["spec"])
		if spec == nil {
			spec = object(op["spec"])
		}
		if object(spec["nodeSelector"])["kubernetes.io/hostname"] != p.Node {
			return domain.E("invalid", "every stage must select the declared client node")
		}
		volumes := map[string]bool{}
		for _, v := range array(spec["volumes"]) {
			vol := object(v)
			claim := object(vol["persistentVolumeClaim"])
			if claim["claimName"] == pvc && claim["readOnly"] != true {
				volumes[fmt.Sprint(vol["name"])] = true
			}
		}
		mounted := false
		for _, c := range array(spec["containers"]) {
			for _, v := range array(object(c)["volumeMounts"]) {
				m := object(v)
				if volumes[fmt.Sprint(m["name"])] && m["mountPath"] == root && m["readOnly"] != true && m["subPath"] == nil && m["subPathExpr"] == nil {
					mounted = true
				}
			}
		}
		if !mounted || filepath.Clean(root) != root {
			return domain.E("invalid", "stage lacks the declared writable results PVC mount")
		}
	}
	return nil
}
