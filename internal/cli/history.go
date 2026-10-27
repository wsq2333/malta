package cli

import (
	"github.com/spf13/cobra"
	"github.com/wsq2333/malta/internal/store"
	"net/url"
	"strconv"
)

func historyCommand(o *Options) *cobra.Command {
	var f store.Filter
	c := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		q := url.Values{"environment": {f.Environment}, "version": {f.Version}, "kind": {f.Kind}, "status": {f.Status}, "from": {f.From}, "to": {f.To}, "limit": {strconv.Itoa(f.Limit)}, "offset": {strconv.Itoa(f.Offset)}}
		v, e := access(o, "GET", "/api/v1/runs?"+q.Encode(), nil, func(s *store.Store) (any, error) { return s.Runs(f) })
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}}
	flags := c.Flags()
	flags.StringVar(&f.Environment, "environment", "", "Environment ID")
	flags.StringVar(&f.Version, "version", "", "Version ID")
	flags.StringVar(&f.Kind, "kind", "", "data or metadata")
	flags.StringVar(&f.Status, "status", "", "Run status")
	flags.StringVar(&f.From, "from", "", "Inclusive RFC3339 start date")
	flags.StringVar(&f.To, "to", "", "Inclusive RFC3339 end date")
	flags.IntVar(&f.Limit, "limit", 100, "Page size, maximum 500")
	flags.IntVar(&f.Offset, "offset", 0, "Pagination offset")
	return c
}
