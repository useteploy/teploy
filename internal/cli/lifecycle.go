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

func newStopCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop all containers for the app",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLifecycle(flags, "stop")
		},
	}
}

func newStartCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start all stopped containers for the app",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLifecycle(flags, "start")
		},
	}
}

func newRestartCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart all containers for the app",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLifecycle(flags, "restart")
		},
	}
}

func runLifecycle(flags *Flags, action string) error {
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

	lc := deploy.NewLifecycle(executor, os.Stdout)

	stopTimeout := appCfg.StopTimeout
	if stopTimeout == 0 {
		stopTimeout = 10
	}

	switch action {
	case "stop":
		err = lc.Stop(ctx, appCfg.App, stopTimeout)
	case "start":
		err = lc.Start(ctx, appCfg.App)
	case "restart":
		err = lc.Restart(ctx, appCfg.App, stopTimeout)
	}

	// Fire notification (best-effort).
	if notifier := notify.NewNotifier(appCfg.Notifications.Webhook); notifier != nil {
		msg := fmt.Sprintf("%s %s", action, appCfg.App)
		if err != nil {
			msg = fmt.Sprintf("%s failed for %s: %s", action, appCfg.App, err)
		}
		if nErr := notifier.Send(ctx, notify.Payload{
			App:     appCfg.App,
			Server:  executor.Host(),
			Type:    action,
			Success: err == nil,
			Message: msg,
		}); nErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: notification failed: %v\n", nErr)
		}
	}

	return err
}
