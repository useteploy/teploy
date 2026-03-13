package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
	"github.com/teploy/teploy/internal/config"
	"github.com/teploy/teploy/internal/preview"
)

func newPreviewCmd(flags *Flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Manage preview environments",
	}

	cmd.AddCommand(newPreviewDeployCmd(flags))
	cmd.AddCommand(newPreviewListCmd(flags))
	cmd.AddCommand(newPreviewDestroyCmd(flags))
	cmd.AddCommand(newPreviewPruneCmd(flags))

	return cmd
}

func newPreviewDeployCmd(flags *Flags) *cobra.Command {
	var ttl string

	cmd := &cobra.Command{
		Use:   "deploy <branch>",
		Short: "Deploy a branch preview",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreviewDeploy(flags, args[0], ttl)
		},
	}

	cmd.Flags().StringVar(&ttl, "ttl", "72h", "time-to-live before auto-expiry")

	return cmd
}

func runPreviewDeploy(flags *Flags, branch, ttlStr string) error {
	appCfg, err := config.LoadApp(".")
	if err != nil {
		return err
	}

	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return fmt.Errorf("invalid TTL: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	executor, err := connectForApp(ctx, flags, appCfg)
	if err != nil {
		return err
	}
	defer executor.Close()

	version, err := gitShortHash()
	if err != nil {
		return fmt.Errorf("could not determine version from git: %w", err)
	}

	// Use the app's current image or build tag.
	image := appCfg.Image
	if image == "" {
		image = appCfg.App + "-build-" + version
	}

	mgr := preview.NewManager(executor, os.Stdout)
	return mgr.Deploy(ctx, preview.DeployConfig{
		App:     appCfg.App,
		Domain:  appCfg.Domain,
		Branch:  branch,
		Image:   image,
		Version: version,
		TTL:     ttl,
	})
}

func newPreviewListCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List active previews",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreviewList(flags)
		},
	}
}

func runPreviewList(flags *Flags) error {
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

	mgr := preview.NewManager(executor, os.Stdout)
	previews, err := mgr.List(ctx, appCfg.App)
	if err != nil {
		return err
	}

	if len(previews) == 0 {
		fmt.Println("No active previews")
		return nil
	}

	for _, p := range previews {
		expired := ""
		if time.Now().UTC().After(p.ExpiresAt) {
			expired = " (expired)"
		}
		fmt.Printf("  %s → https://%s%s\n", p.Branch, p.Domain, expired)
		fmt.Printf("    Container: %s  Port: %d  Expires: %s\n",
			p.Container, p.Port, p.ExpiresAt.Format(time.RFC3339))
	}
	return nil
}

func newPreviewDestroyCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "destroy <branch>",
		Short: "Tear down a preview environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreviewDestroy(flags, args[0])
		},
	}
}

func runPreviewDestroy(flags *Flags, branch string) error {
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

	mgr := preview.NewManager(executor, os.Stdout)
	return mgr.Destroy(ctx, appCfg.App, branch)
}

func newPreviewPruneCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "prune",
		Short: "Remove expired previews",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreviewPrune(flags)
		},
	}
}

func runPreviewPrune(flags *Flags) error {
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

	mgr := preview.NewManager(executor, os.Stdout)
	n, err := mgr.Prune(ctx, appCfg.App)
	if err != nil {
		return err
	}
	if n == 0 {
		fmt.Println("No expired previews to prune")
	} else {
		fmt.Printf("Pruned %d expired preview(s)\n", n)
	}
	return nil
}
