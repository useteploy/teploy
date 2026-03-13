package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"
	"github.com/useteploy/teploy/internal/config"
	"github.com/useteploy/teploy/internal/ssh"
)

func newExecCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "exec <server> <command>",
		Short: "Run a command on a remote server",
		Long:  "Execute a command on the specified server via SSH. The server can be a name from servers.yml or a raw IP address.",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExec(flags, args)
		},
	}
}

func runExec(flags *Flags, args []string) error {
	serverName := args[0]
	remoteCmd := strings.Join(args[1:], " ")

	host, user, keyPath, err := config.ResolveServer(serverName, flags.Host, flags.User, flags.Key)
	if err != nil {
		return fmt.Errorf("resolving server: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	executor, err := ssh.Connect(ctx, ssh.ConnectConfig{
		Host:    host,
		User:    user,
		KeyPath: keyPath,
	})
	if err != nil {
		return err
	}
	defer executor.Close()

	return executor.RunStream(ctx, remoteCmd, os.Stdout, os.Stderr)
}
