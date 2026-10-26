package domain

import "testing"

func scenarioFixture(t *testing.T) Plan {
	t.Helper()
	old := FixturePlan(t)
	var s ScenarioSpec
	if err := Read("../../benchmarks/scenarios/mixed-data.yaml", &s); err != nil {
		t.Fatal(err)
	}
	p, err := NewScenarioPlan(s, old.Environment, old.Version, old.Target, old.ResourceGroup)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestScenarioPlan(t *testing.T) {
	p := scenarioFixture(t)
	bytes, seconds := p.Budget()
	if bytes != 144<<20 || seconds != 36 || len(p.Trials) != 6 {
		t.Fatalf("budget %d/%d points %d", bytes, seconds, len(p.Trials))
	}
	p.Spec.Scenario.Stages[0].Workloads[1].ID = "small-read"
	p.Hash = p.Digest()
	if p.Validate() == nil {
		t.Fatal("duplicate accepted")
	}
}
func TestLegacyPlanHash(t *testing.T) {
	p := FixturePlan(t)
	if p.Hash != "1dcfc4a4c5c90e1656d6aef407b9b1dffca68d9e2a588b517e3864455feb2021" {
		t.Fatalf("legacy hash changed: %s", p.Hash)
	}
}
func TestScenarioPlanTampering(t *testing.T) {
	p := scenarioFixture(t)
	p.Trials[0].WorkloadID = "other"
	p.Hash = p.Digest()
	if p.Validate() == nil {
		t.Fatal("tampered points accepted")
	}
}
