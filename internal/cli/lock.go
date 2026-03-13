package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"os/user"

	"github.com/spf13/cobra"
	"github.com/teploy/teploy/internal/config"
	"github.com/teploy/teploy/internal/state"
)

func newLockCmd(flags *Flags) *cobra.Command {
	var message string

	cmd := &cobra.Command{
		Use:   "lock",
		Short: "Freeze deploys for the app",
		Long:  "Place a manual deploy lock on the app. All deploys are blocked until 'teploy unlock' is run.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLock(flags, message)
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "reason for locking")
	return cmd
}

func runLock(flags *Flags, message string) error {
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

	if err := state.EnsureAppDir(ctx, executor, appCfg.App); err != nil {
		return err
	}

	username := "unknown"
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	if err := state.AcquireManualLock(ctx, executor, appCfg.App, username, message); err != nil {
		return err
	}

	fmt.Printf("Locked %s", appCfg.App)
	if message != "" {
		fmt.Printf(": %s", message)
	}
	fmt.Println()
	return nil
}

func newUnlockCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "unlock",
		Short: "Release deploy lock for the app",
		Long:  "Remove the manual deploy lock, allowing deploys to proceed.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnlock(flags)
		},
	}
}

func runUnlock(flags *Flags) error {
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

	// Check if locked first for user feedback.
	info, _ := state.ReadLock(ctx, executor, appCfg.App)
	if info == nil {
		fmt.Printf("%s is not locked\n", appCfg.App)
		return nil
	}

	state.ReleaseLock(ctx, executor, appCfg.App)
	fmt.Printf("Unlocked %s\n", appCfg.App)
	return nil
}
