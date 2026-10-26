package main

import (
	"fmt"
	"github.com/invopop/jsonschema"
	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
)

func main() {
	for name, v := range map[string]any{"scenario": domain.ScenarioSpec{}, "spec": domain.Spec{}, "environment": domain.Environment{}, "version": domain.SoftwareVersion{}, "plan": domain.Plan{}, "run": domain.Run{}, "bundle": artifact.Bundle{}, "baseline": domain.Baseline{}, "comparison": domain.Comparison{}, "capabilities": domain.Capabilities{}} {
		r := new(jsonschema.Reflector)
		s := r.Reflect(v)
		// Scenario timing is inherited from the stage. Keep the legacy Workload
		// schema (and serialized hashes) unchanged while accepting source templates.
		if def, ok := s.Definitions["ScenarioWorkload"]; ok {
			parameters := *s.Definitions["Workload"]
			parameters.Required = []string{}
			for _, field := range s.Definitions["Workload"].Required {
				if field != "runtime_seconds" && field != "warmup_seconds" {
					parameters.Required = append(parameters.Required, field)
				}
			}
			s.Definitions["ScenarioParameters"] = &parameters
			def.Properties.Set("workload", &jsonschema.Schema{Ref: "#/$defs/ScenarioParameters"})
		}
		s.ID = jsonschema.ID("https://malta.local/schemas/" + name + ".json")
		if err := domain.Write("schemas/"+name+".json", s); err != nil {
			panic(err)
		}
		fmt.Println(name)
	}
}
