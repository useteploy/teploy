package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/useteploy/teploy/internal/autodeploy"
	"github.com/useteploy/teploy/internal/config"
)

func newAutoDeployCmd(flags *Flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "autodeploy",
		Short: "Manage webhook-triggered auto-deploys",
	}

	cmd.AddCommand(newAutoDeploySetupCmd(flags))
	cmd.AddCommand(newAutoDeployStatusCmd(flags))
	cmd.AddCommand(newAutoDeployRemoveCmd(flags))

	return cmd
}

func newAutoDeploySetupCmd(flags *Flags) *cobra.Command {
	var (
		branch string
		secret string
	)

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Set up webhook auto-deploy",
		Long:  "Installs a webhook listener on the server that triggers deploys on git push.\nConfigure your Git provider to POST to https://yourdomain.com/teploy-webhook/<app>",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAutoDeploySetup(flags, branch, secret)
		},
	}

	cmd.Flags().StringVar(&branch, "branch", "main", "branch to watch for pushes")
	cmd.Flags().StringVar(&secret, "secret", "", "webhook secret for request validation")

	return cmd
}

func runAutoDeploySetup(flags *Flags, branch, secret string) error {
	appCfg, err := config.LoadApp(".")
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	executor, err := connectForApp(ctx, flags, appCfg)
	if err != nil {
		return err
	}
	defer executor.Close()

	mgr := autodeploy.NewManager(executor, os.Stdout)

	cfg := autodeploy.Config{
		App:    appCfg.App,
		Branch: branch,
		Secret: secret,
	}

	if err := mgr.Setup(ctx, cfg); err != nil {
		return err
	}

	if err := mgr.SetupCaddyRoute(ctx, appCfg.App, appCfg.Domain); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not add Caddy route: %v\n", err)
		fmt.Fprintf(os.Stderr, "  You may need to add the webhook route manually\n")
	}

	fmt.Printf("\nAuto-deploy configured for %s\n", appCfg.App)
	fmt.Printf("  Webhook URL: https://%s/teploy-webhook/%s\n", appCfg.Domain, appCfg.App)
	fmt.Printf("  Branch: %s\n", branch)
	if secret != "" {
		fmt.Printf("  Secret: configured\n")
	}
	fmt.Printf("\nAdd this URL to your Git provider's webhook settings (push events only).\n")
	return nil
}

func newAutoDeployStatusCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check auto-deploy status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAutoDeployStatus(flags)
		},
	}
}

func runAutoDeployStatus(flags *Flags) error {
	appCfg, err := config.LoadApp(".")
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	executor, err := connectForApp(ctx, flags, appCfg)
	if err != nil {
		return err
	}
	defer executor.Close()

	mgr := autodeploy.NewManager(executor, os.Stdout)
	active, status, err := mgr.Status(ctx, appCfg.App)
	if err != nil {
		return err
	}

	if active {
		fmt.Printf("Auto-deploy: active (%s)\n", status)
		fmt.Printf("  Webhook URL: https://%s/teploy-webhook/%s\n", appCfg.Domain, appCfg.App)
	} else {
		fmt.Println("Auto-deploy: not configured")
		fmt.Println("  Run 'teploy autodeploy setup' to enable")
	}
	return nil
}

func newAutoDeployRemoveCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "remove",
		Short: "Remove auto-deploy webhook",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAutoDeployRemove(flags)
		},
	}
}

func runAutoDeployRemove(flags *Flags) error {
	appCfg, err := config.LoadApp(".")
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	executor, err := connectForApp(ctx, flags, appCfg)
	if err != nil {
		return err
	}
	defer executor.Close()

	mgr := autodeploy.NewManager(executor, os.Stdout)
	return mgr.Remove(ctx, appCfg.App)
}
