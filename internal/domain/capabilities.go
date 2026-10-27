package domain

type Capabilities struct {
	SchemaVersion  string          `json:"schema_version"`
	Version        string          `json:"version"`
	Mode           string          `json:"mode"`
	Executors      []string        `json:"executors"`
	FioVersion     string          `json:"fio_version"`
	MetadataImport string          `json:"metadata_import"`
	Features       map[string]bool `json:"features"`
	Limits         map[string]any  `json:"limits"`
}

func OfflineCapabilities() Capabilities {
	return Capabilities{SchemaVersion: Schema, Version: Version, Mode: "offline", Executors: []string{"local"}, FioVersion: "3.33", MetadataImport: "mdtest-4.0.0-single-task-files", Features: map[string]bool{"case_library": true, "case_revisions": true, "scenario_plan": true, "scenario_sequential": true, "scenario_parallel": true, "scenario_weighted": true, "scenario_pipeline": true, "scenario_metadata_stat": true, "plan": true, "portable_evidence": true, "baseline_comparison": true, "managed_runs": false, "automatic_analysis": false, "metadata_execution": false}, Limits: map[string]any{"artifact_bytes": 32 << 20, "trials": 128, "scenario_schema": ScenarioSchema, "scenario_stage_workers": 256, "scenario_stage_workloads": 16, "native_iodepth": 1, "native_block_bytes": 1 << 20}}
}
