package cli

import (
	"context"
	"github.com/spf13/cobra"
	"github.com/wsq2333/malta/internal/domain"
	kube "github.com/wsq2333/malta/internal/executor/kubernetes"
	"time"
)

func kubernetesCommand() *cobra.Command {
	var config, plan string
	c := &cobra.Command{Use: "kubernetes", Short: "Inspect Kubernetes executor prerequisites"}
	check := &cobra.Command{Use: "check", Short: "Read-only PVC and node preflight", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		var cfg kube.Config
		if err := domain.Read(config, &cfg); err != nil {
			return err
		}
		e, err := kube.New(cfg)
		if err != nil {
			return err
		}
		if plan != "" {
			var p domain.Plan
			if err = domain.Read(plan, &p); err != nil {
				return err
			}
			if err = p.Validate(); err != nil {
				return err
			}
			if err = e.Validate(p); err != nil {
				return err
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		snapshot, err := e.Preflight(ctx)
		if err != nil {
			return err
		}
		return Output(c.OutOrStdout(), snapshot)
	}}
	check.Flags().StringVar(&config, "config", "", "Executor configuration")
	check.Flags().StringVar(&plan, "plan", "", "Optional plan to validate against the executor")
	_ = check.MarkFlagRequired("config")
	c.AddCommand(check)
	return c
}
