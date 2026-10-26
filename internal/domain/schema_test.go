package domain

import (
	"encoding/json"
	js "github.com/santhosh-tekuri/jsonschema/v6"
	"os"
	"testing"
)

func TestSchemaExamples(t *testing.T) {
	p := FixturePlan(t)
	for _, tt := range []struct {
		name string
		v    any
	}{{"spec", p.Spec}, {"environment", p.Environment}, {"version", p.Version}, {"plan", p}} {
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
