package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/teploy/teploy/internal/caddy"
	"github.com/teploy/teploy/internal/config"
)

func newMaintenanceCmd(flags *Flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "maintenance",
		Aliases: []string{"maint"},
		Short:   "Toggle maintenance mode",
	}

	cmd.AddCommand(newMaintenanceOnCmd(flags))
	cmd.AddCommand(newMaintenanceOffCmd(flags))

	return cmd
}

func newMaintenanceOnCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "on",
		Short: "Enable maintenance mode (returns 503 to all visitors)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMaintenanceToggle(flags, true)
		},
	}
}

func newMaintenanceOffCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "off",
		Short: "Disable maintenance mode (restore normal traffic)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMaintenanceToggle(flags, false)
		},
	}
}

func runMaintenanceToggle(flags *Flags, enable bool) error {
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

	client := caddy.NewClient(executor)

	if enable {
		fmt.Printf("Enabling maintenance mode for %s (%s)...\n", appCfg.App, appCfg.Domain)
		if err := client.SetMaintenance(ctx, appCfg.App, appCfg.Domain); err != nil {
			return fmt.Errorf("enabling maintenance mode: %w", err)
		}
		fmt.Println("Maintenance mode enabled — all visitors see 503 page")
	} else {
		fmt.Printf("Disabling maintenance mode for %s...\n", appCfg.App)
		if err := client.RemoveMaintenance(ctx, appCfg.App); err != nil {
			return fmt.Errorf("disabling maintenance mode: %w", err)
		}
		fmt.Println("Maintenance mode disabled — traffic restored")
	}

	return nil
}
