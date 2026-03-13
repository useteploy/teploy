package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/useteploy/teploy/internal/config"
	"github.com/useteploy/teploy/internal/deploy"
	"github.com/useteploy/teploy/internal/notify"
)

func newRollbackCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "rollback",
		Short: "Roll back to the previous deploy",
		Long:  "Start the previous version's containers, health check, re-route traffic, and stop the current containers.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRollback(flags)
		},
	}
}

func runRollback(flags *Flags) error {
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

	rollbackErr := deploy.Rollback(ctx, executor, os.Stdout, deploy.RollbackConfig{
		App:         appCfg.App,
		Domain:      appCfg.Domain,
		StopTimeout: appCfg.StopTimeout,
	})

	// Fire notification (best-effort).
	if notifier := notify.NewNotifier(appCfg.Notifications.Webhook); notifier != nil {
		msg := fmt.Sprintf("Rolled back %s", appCfg.App)
		if rollbackErr != nil {
			msg = fmt.Sprintf("Rollback failed for %s: %s", appCfg.App, rollbackErr)
		}
		if nErr := notifier.Send(ctx, notify.Payload{
			App:     appCfg.App,
			Server:  executor.Host(),
			Type:    "rollback",
			Success: rollbackErr == nil,
			Message: msg,
		}); nErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: notification failed: %v\n", nErr)
		}
	}

	return rollbackErr
}
