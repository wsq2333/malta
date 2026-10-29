package cli

import (
	"github.com/spf13/cobra"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/store"
)

func issueCommands(root *cobra.Command, o *Options) {
	cmd := &cobra.Command{Use: "issue", Short: "Track benchmark issues and immutable revision history"}
	root.AddCommand(cmd)
	cmd.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		v, e := access(o, "GET", "/api/v1/issues", nil, func(s *store.Store) (any, error) { return s.Issues() })
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}})
	var file string
	save := &cobra.Command{Use: "save --file FILE", Short: "Create (base_revision=0) or append an issue revision", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		var q domain.SaveIssue
		if e := domain.Read(file, &q); e != nil {
			return domain.E("invalid", e.Error())
		}
		if e := q.Validate(); e != nil {
			return e
		}
		v, e := access(o, "POST", "/api/v1/issues", q, func(s *store.Store) (any, error) { return s.SaveIssue(q) })
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}}
	save.Flags().StringVar(&file, "file", "", "Issue JSON with issue_id, base_revision, type, description and resolution fields")
	_ = save.MarkFlagRequired("file")
	cmd.AddCommand(save)
	for _, name := range []string{"get", "history"} {
		cmd.AddCommand(&cobra.Command{Use: name + " ISSUE_ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
			id := args[0]
			if e := domain.CheckID(id); e != nil {
				return e
			}
			path := "/api/v1/issues/" + id
			if name == "history" {
				path += "/history"
			}
			v, e := access(o, "GET", path, nil, func(s *store.Store) (any, error) {
				if name == "history" {
					return s.IssueHistory(id)
				}
				return s.Issue(id)
			})
			if e != nil {
				return e
			}
			return Output(c.OutOrStdout(), v)
		}})
	}
}
