package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hookbin/hookbin-cli/internal/agent"
	"github.com/hookbin/hookbin-cli/internal/updater"
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	root := &cobra.Command{Use: "hookbin", Short: "Forward webhooks to a local server", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(&cobra.Command{Use: "version", Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(cmd.OutOrStdout(), agent.Version)
	}})
	var checkOnly bool
	update := &cobra.Command{
		Use: "update", Short: "Install the latest CLI release", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			release, available, err := updater.Check(cmd.Context(), agent.Version)
			if err != nil {
				return err
			}
			if !available {
				fmt.Fprintf(cmd.OutOrStdout(), "hookbin %s is up to date\n", agent.Version)
				return nil
			}
			if checkOnly {
				fmt.Fprintf(cmd.OutOrStdout(), "hookbin %s is available; run hookbin update to install it\n", release.Version())
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updating hookbin %s → %s...\n", agent.Version, release.Version())
			if err := updater.Install(cmd.Context(), release); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "hookbin %s installed; restart the CLI to use it\n", release.Version())
			return nil
		},
	}
	update.Flags().BoolVar(&checkOnly, "check", false, "Check for a new release without installing it")
	root.AddCommand(update)
	var secret, target, server string
	var timeout time.Duration
	var insecure bool
	listen := &cobra.Command{
		Use: "listen <slug>", Short: "Connect to a bin and forward webhooks", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if secret == "" {
				secret = os.Getenv("HOOKBIN_SECRET")
			}
			if secret == "" {
				return errors.New("provide --secret or HOOKBIN_SECRET")
			}
			if target == "" {
				return errors.New("provide --forward-to")
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if agent.Version != "dev" {
				go func() {
					checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					defer cancel()
					release, available, err := updater.Check(checkCtx, agent.Version)
					if err == nil && available && checkCtx.Err() == nil {
						agent.NotifyUpdate(release.Version())
					}
				}()
			}
			return agent.Listen(ctx, server, args[0], secret, target, timeout, insecure)
		},
	}
	listen.Flags().StringVarP(&secret, "secret", "s", "", "Bin secret (or HOOKBIN_SECRET)")
	listen.Flags().StringVarP(&target, "forward-to", "f", "", "Local target URL")
	listen.Flags().StringVar(&server, "server", "https://hookbin.madebyflow.de", "Hookbin server URL")
	listen.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "Local request timeout")
	listen.Flags().BoolVar(&insecure, "insecure", false, "Allow self-signed HTTPS on the local target")
	root.AddCommand(listen)
	return root
}
