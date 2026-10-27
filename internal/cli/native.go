package cli

import (
	"context"
	"github.com/spf13/cobra"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/engine/native"
)

func nativeCommand() *cobra.Command {
	var input string
	c := &cobra.Command{Use: "internal-native", Hidden: true, Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		var q native.Request
		if err := domain.Read(input, &q); err != nil {
			return err
		}
		return native.Execute(context.Background(), q)
	}}
	c.Flags().StringVar(&input, "input", "", "Runner-issued native stage request")
	c.MarkFlagRequired("input")
	return c
}
