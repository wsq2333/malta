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
		s.ID = jsonschema.ID("https://malta.local/schemas/" + name + ".json")
		if err := domain.Write("schemas/"+name+".json", s); err != nil {
			panic(err)
		}
		fmt.Println(name)
	}
}
