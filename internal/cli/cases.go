package cli

import (
	"github.com/spf13/cobra"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/store"
	"strconv"
)

func caseCommands(root *cobra.Command, o *Options) {
	parent := &cobra.Command{Use: "case", Short: "Manage immutable benchmark case revisions"}
	root.AddCommand(parent)
	parent.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		v, e := access(o, "GET", "/api/v1/cases", nil, func(s *store.Store) (any, error) { return s.Cases() })
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}})
	var file, id string
	save := &cobra.Command{Use: "save", Short: "Create a case, or append a revision with --id", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		var q domain.SaveCase
		if e := domain.Read(file, &q); e != nil {
			return e
		}
		path := "/api/v1/cases"
		if id != "" {
			if e := domain.CheckID(id); e != nil {
				return e
			}
			path += "/" + id + "/versions"
		}
		v, e := access(o, "POST", path, q, func(s *store.Store) (any, error) { return s.SaveCase(id, q) })
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}}
	save.Flags().StringVar(&file, "file", "", "Case JSON/YAML: name, description, change_note, base_revision, plan")
	save.Flags().StringVar(&id, "id", "", "Existing case ID (omit to create)")
	_ = save.MarkFlagRequired("file")
	parent.AddCommand(save)
	parent.AddCommand(&cobra.Command{Use: "history CASE_ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		if e := domain.CheckID(args[0]); e != nil {
			return e
		}
		v, e := access(o, "GET", "/api/v1/cases/"+args[0]+"/versions", nil, func(s *store.Store) (any, error) { return s.CaseVersions(args[0]) })
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}})
	parent.AddCommand(&cobra.Command{Use: "get CASE_ID REVISION", Args: cobra.ExactArgs(2), RunE: func(c *cobra.Command, args []string) error {
		if e := domain.CheckID(args[0]); e != nil {
			return e
		}
		n, e := strconv.Atoi(args[1])
		if e != nil || n < 1 {
			return domain.E("invalid", "revision must be positive")
		}
		v, e := access(o, "GET", "/api/v1/cases/"+args[0]+"/versions/"+strconv.Itoa(n), nil, func(s *store.Store) (any, error) { return s.CaseVersion(args[0], n) })
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}})
}
