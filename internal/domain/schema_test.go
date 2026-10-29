package domain

import (
	"encoding/json"
	js "github.com/santhosh-tekuri/jsonschema/v6"
	"os"
	"testing"
)

func TestSchemaExamples(t *testing.T) {
	p := FixturePlan(t)
	var issue SaveIssue
	if err := Read("../../web/src/issue-example.json", &issue); err != nil {
		t.Fatal(err)
	}
	raw, e := os.ReadFile("../../benchmarks/scenarios/mixed-data.yaml")
	if e != nil {
		t.Fatal(e)
	}
	var source any
	if e = Decode(raw, &source); e != nil {
		t.Fatal(e)
	}
	for _, tt := range []struct {
		name string
		v    any
	}{{"save-issue", issue}, {"issue", Issue{IssueID: issue.IssueID, Revision: 1, IssueFields: issue.IssueFields, CreatedAt: issue.OccurredAt, UpdatedAt: issue.OccurredAt}}, {"spec", p.Spec}, {"environment", p.Environment}, {"version", p.Version}, {"plan", p}, {"scenario", source}} {
		t.Run(tt.name, func(t *testing.T) {
			b, err := os.ReadFile("../../schemas/" + tt.name + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var resource any
			if err = json.Unmarshal(b, &resource); err != nil {
				t.Fatal(err)
			}
			c := js.NewCompiler()
			if err = c.AddResource("https://malta.local/schema.json", resource); err != nil {
				t.Fatal(err)
			}
			s, err := c.Compile("https://malta.local/schema.json")
			if err != nil {
				t.Fatal(err)
			}
			j, _ := json.Marshal(tt.v)
			var v any
			json.Unmarshal(j, &v)
			if err = s.Validate(v); err != nil {
				t.Fatal(err)
			}
			v.(map[string]any)["unexpected"] = true
			if s.Validate(v) == nil {
				t.Fatal("schema permits unknown property")
			}
		})
	}
}
