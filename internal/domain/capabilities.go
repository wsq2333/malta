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
	return Capabilities{SchemaVersion: Schema, Version: Version, Mode: "offline", Executors: []string{"local"}, FioVersion: "3.33", MetadataImport: "mdtest-4.0.0-single-task-files", Features: map[string]bool{"scenario_plan": true, "scenario_sequential": true, "scenario_parallel": true, "scenario_weighted": false, "scenario_pipeline": false, "plan": true, "portable_evidence": true, "baseline_comparison": true, "managed_runs": false, "automatic_analysis": false, "metadata_execution": false}, Limits: map[string]any{"artifact_bytes": 32 << 20, "trials": 128}}
}
