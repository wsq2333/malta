package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/wsq2333/malta/internal/domain"
	"io"
	"os"
)

type Envelope struct {
	SchemaVersion string        `json:"schema_version"`
	Data          any           `json:"data,omitempty"`
	Error         *domain.Error `json:"error,omitempty"`
}
type Options struct {
	Format         string
	NonInteractive bool
	Server         string
	Token          string
	DataDir        string
}

func Output(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(Envelope{SchemaVersion: domain.Schema, Data: v})
}
func ExitCode(err error) int {
	var e *domain.Error
	if errors.As(err, &e) {
		switch e.Code {
		case "invalid", "unsupported":
			return 2
		case "timeout":
			return 4
		case "cancelled":
			return 5
		case "conflict":
			return 6
		case "unauthorized":
			return 7
		}
	}
	return 3
}
func NewRoot(out, errOut io.Writer) *cobra.Command {
	o := &Options{}
	r := &cobra.Command{Use: "malta", Short: "Agent-enhanced filesystem benchmark platform", SilenceErrors: true, SilenceUsage: true, PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if o.Format != "json" {
			return domain.E("invalid", "only --format json is supported")
		}
		return nil
	}}
	r.SetOut(out)
	r.SetErr(errOut)
	f := r.PersistentFlags()
	f.StringVar(&o.Format, "format", "json", "Output format (json)")
	f.BoolVar(&o.NonInteractive, "non-interactive", false, "Never prompt for input")
	f.StringVar(&o.Server, "server", "", "Malta HTTP API URL (omit for offline commands)")
	f.StringVar(&o.Token, "token", os.Getenv("MALTA_TOKEN"), "API token (prefer MALTA_TOKEN environment variable)")
	f.StringVar(&o.DataDir, "data-dir", ".malta", "Local metadata and artifact directory")
	r.AddCommand(&cobra.Command{Use: "version", Short: "Show CLI and schema versions", RunE: func(c *cobra.Command, args []string) error {
		return Output(c.OutOrStdout(), map[string]string{"version": domain.Version, "schema_version": domain.Schema})
	}})
	return r
}
func Execute(args []string, out, errOut io.Writer) int {
	r := NewRoot(out, errOut)
	r.SetArgs(args)
	if err := r.Execute(); err != nil {
		var e *domain.Error
		if !errors.As(err, &e) {
			e = &domain.Error{Code: "invalid", Message: err.Error()}
		}
		if er := json.NewEncoder(out).Encode(Envelope{SchemaVersion: domain.Schema, Error: e}); er != nil {
			fmt.Fprintln(errOut, er)
		}
		return ExitCode(e)
	}
	return 0
}
