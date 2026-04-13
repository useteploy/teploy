package cli

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/useteploy/teploy/internal/ui"
)

func newUICmd() *cobra.Command {
	var port int
	var noOpen bool

	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Launch the web dashboard",
		Long:  "Starts a local web server with a dashboard UI for managing deployments, servers, and apps.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := fmt.Sprintf("127.0.0.1:%d", port)
			srv := ui.NewServer(addr)

			// Handle graceful shutdown on Ctrl+C.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt)
			go func() {
				<-sigCh
				fmt.Fprintln(os.Stderr, "\nShutting down...")
				srv.Stop()
			}()

			if !noOpen {
				go ui.OpenBrowser(fmt.Sprintf("http://%s", addr))
			}

			return srv.Start()
		},
	}

	cmd.Flags().IntVar(&port, "port", 3456, "port to listen on")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "don't open browser automatically")

	return cmd
}
