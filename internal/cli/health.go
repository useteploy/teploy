package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/teploy/teploy/internal/config"
	"github.com/teploy/teploy/internal/deploy"
	"github.com/teploy/teploy/internal/state"
)

func newHealthCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Run health check on the running app",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHealth(flags)
		},
	}
}

func runHealth(flags *Flags) error {
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

	current, err := state.Read(ctx, executor, appCfg.App)
	if err != nil || current == nil {
		return fmt.Errorf("no deploy state found for %s — deploy first", appCfg.App)
	}

	fmt.Printf("Running health check on %s (port %d)...\n", appCfg.App, current.CurrentPort)

	deployer := deploy.NewDeployer(executor, os.Stdout)
	if err := deployer.HealthCheckPublic(ctx, current.CurrentPort); err != nil {
		fmt.Printf("Health check FAILED: %v\n", err)
		return err
	}

	fmt.Println("Health check passed")
	return nil
}
