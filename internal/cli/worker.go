package cli

import (
	"context"
	"github.com/spf13/cobra"
	"github.com/wsq2333/malta/internal/domain"
	"github.com/wsq2333/malta/internal/runner"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func workerCommand() *cobra.Command {
	var input, results, fio string
	var timeout time.Duration
	var max int64
	c := &cobra.Command{Use: "worker", Short: "Execute a controller-created Kubernetes attempt once", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		var r domain.Run
		if err := domain.Read(input, &r); err != nil {
			return err
		}
		if err := r.Validate(); err != nil {
			return err
		}
		if r.Attempt.Executor != "kubernetes" || r.Plan.Target != "/bench" {
			return domain.E("invalid", "worker requires a Kubernetes run targeting /bench")
		}
		out, err := runner.ClaimAttempt(results, r, os.Getenv("MALTA_POD_UID"))
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		final, err := runner.Execute(ctx, r, out, runner.Options{Fio: fio, Timeout: timeout, MaxBytes: max})
		if err != nil {
			return err
		}
		return Output(c.OutOrStdout(), final)
	}}
	c.Flags().StringVar(&input, "run", "/etc/malta/run.json", "Controller-issued run")
	c.Flags().StringVar(&results, "results", "/results", "Shared results mount")
	c.Flags().StringVar(&fio, "fio", "fio", "fio executable")
	c.Flags().DurationVar(&timeout, "timeout", time.Hour, "Execution deadline")
	c.Flags().Int64Var(&max, "max-bytes", 1<<30, "Prepared data budget")
	return c
}
