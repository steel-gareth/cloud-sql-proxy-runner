package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"cloud-sql-proxy-runner/internal/config"
	"cloud-sql-proxy-runner/internal/preflight"
	"cloud-sql-proxy-runner/internal/proxy"
	"cloud-sql-proxy-runner/internal/secrets"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var showPasswords bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured proxies and their status",
	RunE:  runList,
}

func init() {
	listCmd.Flags().BoolVar(&showPasswords, "show-passwords", false, "show database passwords")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	stateDir := proxy.StateDir()
	daemonRunning := false

	state, err := proxy.ReadState(stateDir)
	if err == nil && proxy.IsRunning(state.PID) {
		daemonRunning = true
	}

	// Fetch passwords if requested
	var passwords map[string]string
	if showPasswords {
		if err := preflight.CheckADC(ctx, preflight.DefaultCredentialFinder); err != nil {
			return err
		}

		client, err := secretmanager.NewClient(ctx)
		if err != nil {
			return fmt.Errorf("creating Secret Manager client: %w", err)
		}
		defer client.Close()

		passwords, err = fetchPasswords(ctx, client, cfg.Proxies)
		if err != nil {
			return err
		}
	}

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 3, ' ', 0)
	if showPasswords {
		fmt.Fprintln(w, "INSTANCE\tPORT\tPROJECT\tSTATUS\tPASSWORD")
	} else {
		fmt.Fprintln(w, "INSTANCE\tPORT\tPROJECT\tSTATUS")
	}

	for _, p := range cfg.Proxies {
		status := "stopped"
		if daemonRunning {
			status = "running"
		}
		if showPasswords {
			pw := passwords[p.Instance]
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n", p.Instance, p.Port, p.Project(), status, pw)
		} else {
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", p.Instance, p.Port, p.Project(), status)
		}
	}
	w.Flush()
	return nil
}

func fetchPasswords(ctx context.Context, client secrets.SecretClient, proxies []config.ProxyEntry) (map[string]string, error) {
	passwords := make(map[string]string)
	g, ctx := errgroup.WithContext(ctx)

	type result struct {
		instance string
		password string
	}
	results := make(chan result, len(proxies))

	for _, p := range proxies {
		p := p
		g.Go(func() error {
			pw, err := secrets.FetchSecret(ctx, client, p.Project(), p.Secret)
			if err != nil {
				return err
			}
			results <- result{instance: p.Instance, password: pw}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	close(results)

	for r := range results {
		passwords[r.instance] = r.password
	}
	return passwords, nil
}
