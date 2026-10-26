package cli

import (
	"encoding/json"
	"github.com/spf13/cobra"
	"github.com/wsq2333/malta/internal/artifact"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/engine/metadata"
	"github.com/wsq2333/malta/internal/store"
	"io"
	"os"
)

func access(o *Options, method, path string, body any, local func(*store.Store) (any, error)) (any, error) {
	if o.Server != "" {
		return remote(o, method, path, body)
	}
	s, e := store.Open(o.DataDir)
	if e != nil {
		return nil, e
	}
	defer s.Close()
	return local(s)
}
func dataCommands(root *cobra.Command, o *Options) {
	for _, kind := range []string{"environment", "version"} {
		kind := kind
		var parent *cobra.Command
		if kind == "version" {
			for _, c := range root.Commands() {
				if c.Name() == "version" {
					parent = c
				}
			}
		} else {
			parent = &cobra.Command{Use: kind}
			root.AddCommand(parent)
		}
		apiPath := "/api/v1/" + kind + "s"
		var file string
		register := &cobra.Command{Use: "register", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
			var value any
			var id string
			if kind == "environment" {
				var v domain.Environment
				if e := domain.Read(file, &v); e != nil {
					return e
				}
				if e := v.Validate(); e != nil {
					return e
				}
				value = v
				id = v.ID
			} else {
				var v domain.SoftwareVersion
				if e := domain.Read(file, &v); e != nil {
					return e
				}
				if e := v.Validate(); e != nil {
					return e
				}
				value = v
				id = v.ID
			}
			v, e := access(o, "POST", apiPath, value, func(s *store.Store) (any, error) { return value, s.Register(kind, id, value) })
			if e != nil {
				return e
			}
			return Output(c.OutOrStdout(), v)
		}}
		register.Flags().StringVar(&file, "file", "", "Snapshot JSON/YAML")
		_ = register.MarkFlagRequired("file")
		list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
			v, e := access(o, "GET", apiPath, nil, func(s *store.Store) (any, error) { return s.Objects(kind) })
			if e != nil {
				return e
			}
			return Output(c.OutOrStdout(), v)
		}}
		show := &cobra.Command{Use: "show ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
			if e := domain.CheckID(args[0]); e != nil {
				return e
			}
			v, e := access(o, "GET", apiPath+"/"+args[0], nil, func(s *store.Store) (any, error) {
				var v json.RawMessage
				e := s.Object(kind, args[0], &v)
				return v, e
			})
			if e != nil {
				return e
			}
			return Output(c.OutOrStdout(), v)
		}}
		parent.AddCommand(register, list, show)
	}
	result := &cobra.Command{Use: "result"}
	result.AddCommand(historyCommand(o))
	var dir, exportDir, planFile string
	var rawFiles []string
	imp := &cobra.Command{Use: "import", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		b, e := artifact.Read(dir)
		if e != nil {
			return e
		}
		v, e := access(o, "POST", "/api/v1/imports", b, func(s *store.Store) (any, error) { return b.Run, s.Import(b) })
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}}
	imp.Flags().StringVar(&dir, "bundle", "", "Result bundle directory")
	_ = imp.MarkFlagRequired("bundle")
	exp := &cobra.Command{Use: "export RUN_ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		if e := domain.CheckID(args[0]); e != nil {
			return e
		}
		v, e := access(o, "GET", "/api/v1/runs/"+args[0]+"/bundle", nil, func(s *store.Store) (any, error) { return s.Bundle(args[0]) })
		if e != nil {
			return e
		}
		data, e := json.Marshal(v)
		if e != nil {
			return e
		}
		b, e := artifact.Decode(data)
		if e != nil {
			return e
		}
		if e = artifact.Write(exportDir, b); e != nil {
			return e
		}
		return Output(c.OutOrStdout(), map[string]string{"directory": exportDir, "run_id": b.Run.ID})
	}}
	exp.Flags().StringVar(&exportDir, "out", "", "New output directory")
	_ = exp.MarkFlagRequired("out")
	meta := &cobra.Command{Use: "import-mdtest", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		var p domain.Plan
		if e := domain.Read(planFile, &p); e != nil {
			return e
		}
		var raws [][]byte
		for _, name := range rawFiles {
			f, e := os.Open(name)
			if e != nil {
				return e
			}
			raw, e := io.ReadAll(io.LimitReader(f, artifact.MaxBytes+1))
			f.Close()
			if e != nil {
				return e
			}
			raws = append(raws, raw)
		}
		b, e := metadata.Import(p, raws)
		if e != nil {
			return e
		}
		v, e := access(o, "POST", "/api/v1/imports", b, func(s *store.Store) (any, error) { return b.Run, s.Import(b) })
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}}
	meta.Flags().StringVar(&planFile, "plan", "", "Metadata plan")
	meta.Flags().StringSliceVar(&rawFiles, "raw", nil, "Single-iteration raw outputs, one per repeat")
	_ = meta.MarkFlagRequired("plan")
	_ = meta.MarkFlagRequired("raw")
	result.AddCommand(imp, exp, meta)
	baseline := &cobra.Command{Use: "baseline"}
	var name, runID string
	set := &cobra.Command{Use: "set", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		v, e := access(o, "POST", "/api/v1/baselines", map[string]string{"name": name, "run_id": runID}, func(s *store.Store) (any, error) { return s.SetBaseline(name, runID) })
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}}
	set.Flags().StringVar(&name, "name", "", "Baseline display name")
	set.Flags().StringVar(&runID, "run", "", "Successful run ID")
	_ = set.MarkFlagRequired("name")
	_ = set.MarkFlagRequired("run")
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		v, e := access(o, "GET", "/api/v1/baselines", nil, func(s *store.Store) (any, error) { return s.Objects("baseline") })
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}}
	baseline.AddCommand(set, list)
	var a, b string
	compare := &cobra.Command{Use: "compare", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		v, e := access(o, "POST", "/api/v1/comparisons", map[string]string{"baseline": a, "candidate": b}, func(s *store.Store) (any, error) { return s.Compare(a, b) })
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}}
	compare.Flags().StringVar(&a, "baseline", "", "Baseline run ID")
	compare.Flags().StringVar(&b, "candidate", "", "Candidate run ID")
	_ = compare.MarkFlagRequired("baseline")
	_ = compare.MarkFlagRequired("candidate")
	root.AddCommand(result, baseline, compare)
}
