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

func executionCommands(root *cobra.Command, o *Options) {
	var spec, env, ver, target, group, output string
	plan := &cobra.Command{Use: "plan", Short: "Validate and expand a workload without running I/O", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		var s domain.Spec
		var e domain.Environment
		var v domain.SoftwareVersion
		for _, x := range []struct {
			p string
			v any
		}{{spec, &s}, {env, &e}, {ver, &v}} {
			if err := domain.Read(x.p, x.v); err != nil {
				return err
			}
		}
		p, err := domain.NewPlan(s, e, v, target, group)
		if err != nil {
			return err
		}
		if err = domain.Write(output, p); err != nil {
			return err
		}
		return Output(c.OutOrStdout(), p)
	}}
	f := plan.Flags()
	f.StringVar(&spec, "spec", "", "Workload JSON/YAML")
	f.StringVar(&env, "environment", "", "Environment JSON/YAML")
	f.StringVar(&ver, "version", "", "Version JSON/YAML")
	f.StringVar(&target, "target", "", "Existing absolute benchmark target directory")
	f.StringVar(&group, "resource-group", "default", "Exclusive resource identity")
	f.StringVar(&output, "out", "plan.json", "Plan destination")
	for _, name := range []string{"spec", "environment", "version", "target"} {
		_ = plan.MarkFlagRequired(name)
	}
	var input, out, bin string
	var timeout time.Duration
	var budget int64
	execute := &cobra.Command{Use: "exec", Short: "Execute a plan in the foreground and retain an evidence bundle", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		var p domain.Plan
		if e := domain.Read(input, &p); e != nil {
			return e
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		r, e := runner.Execute(ctx, domain.NewRun(p, "human"), out, runner.Options{Fio: bin, Timeout: timeout, MaxBytes: budget})
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), r)
	}}
	f = execute.Flags()
	f.StringVar(&input, "plan", "", "Validated plan file")
	f.StringVar(&out, "out", "", "New result directory")
	f.StringVar(&bin, "fio", "fio", "fio 3.33 executable")
	f.DurationVar(&timeout, "timeout", time.Hour, "Total deadline including preparation")
	f.Int64Var(&budget, "max-bytes", 1<<30, "Maximum total prepared bytes across trials")
	_ = execute.MarkFlagRequired("plan")
	_ = execute.MarkFlagRequired("out")
	root.AddCommand(plan, execute)
}
