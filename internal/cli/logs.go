package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/teploy/teploy/internal/config"
	"github.com/teploy/teploy/internal/docker"
	"github.com/teploy/teploy/internal/state"
)

func newLogsCmd(flags *Flags) *cobra.Command {
	var (
		process string
		lines   int
	)

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Tail container logs",
		Long:  "Stream Docker logs from the running container. Defaults to the web process.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(flags, process, lines)
		},
	}

	cmd.Flags().StringVar(&process, "process", "web", "process type to view logs for")
	cmd.Flags().IntVar(&lines, "lines", 50, "number of historical log lines")

	return cmd
}

func runLogs(flags *Flags, process string, lines int) error {
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

	// Read current state to find the running version.
	current, err := state.Read(ctx, executor, appCfg.App)
	if err != nil || current == nil {
		return fmt.Errorf("no deploy state found for %s — deploy first", appCfg.App)
	}

	containerName := docker.ContainerName(appCfg.App, process, current.CurrentHash)
	cmd := fmt.Sprintf("docker logs -f --tail %d %s", lines, containerName)
	return executor.RunStream(ctx, cmd, os.Stdout, os.Stderr)
}
