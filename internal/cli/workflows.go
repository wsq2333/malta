package cli

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/wsq2333/malta/internal/domain"
	"net/url"
)

func workflowCommands(root *cobra.Command, o *Options) {
	command := &cobra.Command{Use: "workflow", Short: "Manage native workflow cases, preflight, submission and verified results"}
	add := func(name, path, method string, args cobra.PositionalArgs, withFile bool) {
		var file, history, backendFilter, connectionFilter string
		var limit, offset int
		c := &cobra.Command{Use: name, Args: args, RunE: func(c *cobra.Command, a []string) error {
			if o.Server == "" {
				return domain.E("invalid", "workflow commands require --server")
			}
			p := path
			if len(a) > 0 {
				p += url.PathEscape(a[0])
			}
			if name == "records KIND" && history != "" {
				p += "?id=" + url.QueryEscape(history)
			}
			if name == "list" {
				p += fmt.Sprintf("?limit=%d&offset=%d", limit, offset)
				if backendFilter != "" {
					p += "&backend=" + url.QueryEscape(backendFilter)
				}
				if connectionFilter != "" {
					p += "&connection=" + url.QueryEscape(connectionFilter)
				}
			}
			if name == "link RUN_ID" || name == "issues RUN_ID" {
				p += "/issues"
			}
			var body json.RawMessage
			if withFile {
				if file == "" {
					return domain.E("invalid", "--file required")
				}
				if e := domain.Read(file, &body); e != nil {
					return e
				}
			}
			v, e := remote(o, method, "/api/v1/workflow/"+p, body)
			if e != nil {
				return e
			}
			return Output(c.OutOrStdout(), v)
		}}
		if name == "records KIND" {
			c.Flags().StringVar(&history, "id", "", "List immutable history for this record ID")
		}
		if name == "list" {
			c.Flags().StringVar(&backendFilter, "backend", "", "Filter uce or argo before pagination")
			c.Flags().StringVar(&connectionFilter, "connection", "", "Filter connection ID before pagination")
			c.Flags().IntVar(&limit, "limit", 20, "Page size (1..100)")
			c.Flags().IntVar(&offset, "offset", 0, "Page offset")
		}
		if withFile {
			c.Flags().StringVar(&file, "file", "", "JSON request file")
		}
		command.AddCommand(c)
	}
	add("connections", "connections", "GET", cobra.NoArgs, false)
	add("records KIND", "records/", "GET", cobra.ExactArgs(1), false)
	add("save KIND", "records/", "POST", cobra.ExactArgs(1), true)
	add("preflight", "preflight", "POST", cobra.NoArgs, true)
	add("start", "runs", "POST", cobra.NoArgs, true)
	add("list", "runs", "GET", cobra.NoArgs, false)
	add("get RUN_ID", "runs/", "GET", cobra.ExactArgs(1), false)
	add("lookup REQUEST_ID", "requests/", "GET", cobra.ExactArgs(1), false)
	for _, verb := range []string{"cancel", "collect"} {
		verb := verb
		command.AddCommand(&cobra.Command{Use: verb + " RUN_ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, a []string) error {
			if o.Server == "" {
				return domain.E("invalid", "--server required")
			}
			v, e := remote(o, "POST", "/api/v1/workflow/runs/"+url.PathEscape(a[0])+"/"+verb, nil)
			if e != nil {
				return e
			}
			return Output(c.OutOrStdout(), v)
		}})
	}
	add("link RUN_ID", "runs/", "POST", cobra.ExactArgs(1), true)
	add("issues RUN_ID", "runs/", "GET", cobra.ExactArgs(1), false)
	add("compare", "compare", "POST", cobra.NoArgs, true)
	root.AddCommand(command)
}
