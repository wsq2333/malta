package cli

import (
	"github.com/spf13/cobra"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/store"
)

func capabilitiesCommand(o *Options) *cobra.Command {
	return &cobra.Command{Use: "capabilities", Short: "Discover contract, tools, executors and server budgets", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if o.Server == "" {
			return Output(c.OutOrStdout(), domain.OfflineCapabilities())
		}
		v, e := remote(o, "GET", "/api/v1/capabilities", nil)
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}}
}
func lookupCommand(o *Options) *cobra.Command {
	var request string
	c := &cobra.Command{Use: "lookup", Short: "Recover a run ID from its stable request ID", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if e := domain.CheckID(request); e != nil {
			return e
		}
		v, e := access(o, "GET", "/api/v1/requests/"+request, nil, func(s *store.Store) (any, error) { return s.ByRequest(request) })
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}}
	c.Flags().StringVar(&request, "request-id", "", "Original stable request ID")
	_ = c.MarkFlagRequired("request-id")
	return c
}
