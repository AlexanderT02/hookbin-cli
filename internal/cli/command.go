package cli

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hookbin/hookbin-cli/internal/agent"
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	root := &cobra.Command{Use: "hookbin", Short: "Forward webhooks to a local server", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(&cobra.Command{Use: "version", Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(cmd.OutOrStdout(), agent.Version)
	}})
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
