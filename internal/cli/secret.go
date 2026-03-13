package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"
	"github.com/teploy/teploy/internal/config"
	"github.com/teploy/teploy/internal/secret"
)

func newSecretCmd(flags *Flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Manage encrypted secrets",
	}

	cmd.AddCommand(newSecretSetCmd(flags))
	cmd.AddCommand(newSecretGetCmd(flags))
	cmd.AddCommand(newSecretListCmd(flags))
	cmd.AddCommand(newSecretRotateCmd(flags))

	return cmd
}

func newSecretSetCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "set KEY=value [KEY=value...]",
		Short: "Set one or more encrypted secrets",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pairs := make(map[string]string)
			for _, arg := range args {
				idx := strings.IndexByte(arg, '=')
				if idx < 0 {
					return fmt.Errorf("invalid format: %q (expected KEY=value)", arg)
				}
				pairs[arg[:idx]] = arg[idx+1:]
			}
			return runSecretSet(flags, pairs)
		},
	}
}

func runSecretSet(flags *Flags, pairs map[string]string) error {
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

	mgr := secret.NewManager(executor)
	for k, v := range pairs {
		if err := mgr.Set(ctx, appCfg.App, k, v); err != nil {
			return err
		}
		fmt.Printf("  Set secret %s\n", k)
	}
	return nil
}

func newSecretGetCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "get KEY",
		Short: "Decrypt and display a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSecretGet(flags, args[0])
		},
	}
}

func runSecretGet(flags *Flags, key string) error {
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

	mgr := secret.NewManager(executor)
	val, err := mgr.Get(ctx, appCfg.App, key)
	if err != nil {
		return err
	}
	fmt.Println(val)
	return nil
}

func newSecretListCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all secret keys (values masked)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSecretList(flags)
		},
	}
}

func runSecretList(flags *Flags) error {
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

	mgr := secret.NewManager(executor)
	keys, err := mgr.List(ctx, appCfg.App)
	if err != nil {
		return err
	}

	if len(keys) == 0 {
		fmt.Println("No secrets set")
		return nil
	}

	for _, k := range keys {
		fmt.Printf("%s=***\n", k)
	}
	return nil
}

func newSecretRotateCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "rotate KEY",
		Short: "Generate a new random value for a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSecretRotate(flags, args[0])
		},
	}
}

func runSecretRotate(flags *Flags, key string) error {
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

	mgr := secret.NewManager(executor)
	newVal, err := mgr.Rotate(ctx, appCfg.App, key)
	if err != nil {
		return err
	}
	fmt.Printf("  Rotated %s\n", key)
	fmt.Printf("  New value: %s\n", newVal)
	fmt.Println("  (Restart containers to apply)")
	return nil
}
