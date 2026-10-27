package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/wsq2333/malta/internal/api"
	"github.com/wsq2333/malta/internal/domain"
	kube "github.com/wsq2333/malta/internal/executor/kubernetes"
	"github.com/wsq2333/malta/internal/service"
	"github.com/wsq2333/malta/internal/store"
	"github.com/wsq2333/malta/web"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func remote(o *Options, method, path string, body any) (any, error) {
	base, e := url.Parse(o.Server)
	if e != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, domain.E("invalid", "--server must be an http(s) origin without credentials or query")
	}
	data, e := json.Marshal(body)
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequest(method, strings.TrimRight(o.Server, "/")+path, bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	if o.Token != "" {
		req.Header.Set("Authorization", "Bearer "+o.Token)
	}
	client := http.Client{Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	if e != nil {
		return nil, domain.E("server", e.Error())
	}
	defer resp.Body.Close()
	var env struct {
		SchemaVersion string          `json:"schema_version"`
		Data          json.RawMessage `json:"data"`
		Error         *domain.Error   `json:"error"`
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 70<<20)).Decode(&env); e != nil {
		return nil, domain.E("server", "invalid API response")
	}
	if env.Error != nil {
		return nil, env.Error
	}
	if resp.StatusCode >= 300 || env.SchemaVersion != domain.Schema {
		return nil, domain.E("server", "unexpected API status or schema")
	}
	return env.Data, nil
}
func typedRun(v any) (domain.Run, error) {
	var r domain.Run
	b, e := json.Marshal(v)
	if e != nil {
		return r, e
	}
	e = json.Unmarshal(b, &r)
	return r, e
}
func serverCommands(root *cobra.Command, o *Options) {
	var address, fioPath, kubeConfig string
	var allowed []string
	var bytesBudget int64
	var timeout time.Duration
	serve := &cobra.Command{Use: "serve", Args: cobra.NoArgs, Short: "Serve API and manage local or Kubernetes benchmarks", RunE: func(c *cobra.Command, _ []string) error {
		host, _, e := net.SplitHostPort(address)
		if e != nil {
			return domain.E("invalid", e.Error())
		}
		ip := net.ParseIP(host)
		if (ip == nil || !ip.IsLoopback()) && o.Token == "" {
			return domain.E("invalid", "non-loopback bind requires MALTA_TOKEN")
		}
		s, e := store.Open(o.DataDir)
		if e != nil {
			return e
		}
		defer s.Close()
		var backend *kube.Executor
		if kubeConfig != "" {
			var config kube.Config
			if e = domain.Read(kubeConfig, &config); e != nil {
				return e
			}
			backend, e = kube.New(config)
			if e != nil {
				return e
			}
		}
		m, e := service.New(s, service.Config{AllowedRoots: allowed, Fio: fioPath, MaxBytes: bytesBudget, Timeout: timeout, Kubernetes: backend})
		if e != nil {
			return e
		}
		defer m.Close()
		listener, e := net.Listen("tcp", address)
		if e != nil {
			return e
		}
		server := &http.Server{Handler: api.Handler(m, o.Token, frontendHandler()), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		done := make(chan error, 1)
		go func() { done <- server.Serve(listener) }()
		fmt.Fprintln(c.ErrOrStderr(), "Malta listening on", listener.Addr())
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
			return nil
		case e := <-done:
			if e == http.ErrServerClosed {
				return nil
			}
			return e
		}
	}}
	f := serve.Flags()
	f.StringVar(&address, "listen", "127.0.0.1:8080", "Listen address")
	f.StringSliceVar(&allowed, "allow-target", nil, "Allowed existing target roots")
	f.StringVar(&fioPath, "fio", "fio", "fio executable")
	f.Int64Var(&bytesBudget, "max-bytes", 1<<30, "Maximum prepared bytes per run")
	f.DurationVar(&timeout, "run-timeout", time.Hour, "Total per-run deadline")
	f.StringVar(&kubeConfig, "kubernetes-config", "", "Kubernetes executor config JSON (enables Jobs)")
	run := &cobra.Command{Use: "run"}
	run.AddCommand(historyCommand(o), lookupCommand(o))
	var planFile, request, source, executor, caseID string
	var caseRevision int
	start := &cobra.Command{Use: "start", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if o.Server == "" {
			return domain.E("invalid", "run start requires --server")
		}
		var p domain.Plan
		q := service.StartRequest{RequestID: request, Source: source, Executor: executor}
		if caseID != "" {
			if caseRevision < 1 {
				return domain.E("invalid", "--case-revision must be positive")
			}
			q.CaseRef = &domain.CaseRef{ID: caseID, Revision: caseRevision}
		} else {
			if c.Flags().Changed("case-revision") {
				return domain.E("invalid", "--case-revision requires --case")
			}
			if e := domain.Read(planFile, &p); e != nil {
				return e
			}
			q.Plan = p
		}
		v, e := remote(o, "POST", "/api/v1/runs", q)
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}}
	f = start.Flags()
	f.StringVar(&planFile, "plan", "", "Plan file")
	f.StringVar(&request, "request-id", "", "Stable ID reused on retry")
	f.StringVar(&source, "source", "human", "human, agent or ci")
	f.StringVar(&executor, "executor", "local", "Execution backend (local or kubernetes)")
	f.StringVar(&caseID, "case", "", "Saved case ID")
	f.IntVar(&caseRevision, "case-revision", 0, "Pinned saved case revision")
	start.MarkFlagsOneRequired("plan", "case")
	start.MarkFlagsMutuallyExclusive("plan", "case")
	_ = start.MarkFlagRequired("request-id")
	run.AddCommand(start)
	get := &cobra.Command{Use: "get RUN_ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		if e := domain.CheckID(args[0]); e != nil {
			return e
		}
		v, e := access(o, "GET", "/api/v1/runs/"+args[0], nil, func(s *store.Store) (any, error) { return s.Run(args[0]) })
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}}
	run.AddCommand(get)
	cancel := &cobra.Command{Use: "cancel RUN_ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		if e := domain.CheckID(args[0]); e != nil {
			return e
		}
		v, e := remote(o, "POST", "/api/v1/runs/"+args[0]+"/cancel", nil)
		if e != nil {
			return e
		}
		return Output(c.OutOrStdout(), v)
	}}
	run.AddCommand(cancel)
	var waitTimeout time.Duration
	wait := &cobra.Command{Use: "wait RUN_ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		if e := domain.CheckID(args[0]); e != nil {
			return e
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		deadline := time.NewTimer(waitTimeout)
		defer deadline.Stop()
		for {
			v, e := remote(o, "GET", "/api/v1/runs/"+args[0], nil)
			if e != nil {
				return e
			}
			r, e := typedRun(v)
			if e != nil {
				return e
			}
			if domain.Terminal(r.Status) && r.Attempt.ResultState != "collecting" {
				if r.Status == "succeeded" {
					return Output(c.OutOrStdout(), r)
				}
				code := "execution"
				if r.Status == "cancelled" {
					code = "cancelled"
				}
				return domain.E(code, r.ID+": "+r.Status+"; "+r.Error)
			}
			select {
			case <-ctx.Done():
				return domain.E("cancelled", "wait interrupted; managed run continues")
			case <-deadline.C:
				return domain.E("timeout", "wait timed out; managed run continues; query the same run ID")
			case <-time.After(300 * time.Millisecond):
			}
		}
	}}
	wait.Flags().DurationVar(&waitTimeout, "timeout", 5*time.Minute, "Client wait deadline (does not cancel run)")
	run.AddCommand(wait)
	var follow bool
	logs := &cobra.Command{Use: "logs RUN_ID", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		if e := domain.CheckID(args[0]); e != nil {
			return e
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		for {
			v, e := remote(o, "GET", "/api/v1/runs/"+args[0]+"/logs", nil)
			if e != nil {
				return e
			}
			if e = Output(c.OutOrStdout(), v); e != nil {
				return e
			}
			var state struct {
				Status string `json:"status"`
			}
			b, _ := json.Marshal(v)
			_ = json.Unmarshal(b, &state)
			if !follow || domain.Terminal(state.Status) {
				return nil
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
		}
	}}
	logs.Flags().BoolVar(&follow, "follow", false, "Emit bounded log snapshots as NDJSON until terminal")
	run.AddCommand(logs)
	root.AddCommand(serve, run)
}
func frontendHandler() http.Handler { return web.Handler() }
