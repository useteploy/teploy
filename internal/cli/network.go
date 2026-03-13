package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/teploy/teploy/internal/config"
	"github.com/teploy/teploy/internal/network"
	"github.com/teploy/teploy/internal/ssh"
)

func newNetworkCmd(flags *Flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network",
		Short: "Manage cross-server VPN networking",
	}

	cmd.AddCommand(newNetworkSetupCmd(flags))
	cmd.AddCommand(newNetworkStatusCmd(flags))

	return cmd
}

func newNetworkSetupCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Install VPN on all servers and join mesh",
		Long:  "Install the configured VPN provider on each server, join the mesh, and update DNS entries for cross-server communication.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNetworkSetup(flags)
		},
	}
}

func newNetworkStatusCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show mesh connectivity between servers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNetworkStatus(flags)
		},
	}
}

func runNetworkSetup(flags *Flags) error {
	appCfg, err := config.LoadApp(".")
	if err != nil {
		return err
	}

	if appCfg.Network.Provider == "" {
		return fmt.Errorf("no network provider configured — add a [network] block to teploy.yml")
	}

	provider, err := network.NewProvider(network.Config{
		Provider: appCfg.Network.Provider,
		AuthKey:  appCfg.Network.AuthKey,
		Server:   appCfg.Network.Server,
		SetupKey: appCfg.Network.SetupKey,
	})
	if err != nil {
		return err
	}

	servers, err := resolveAllServers(appCfg, flags)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Phase 1: Install and join on each server, collect VPN IPs.
	vpnIPs := make(map[string]string) // server name -> VPN IP
	executors := make([]ssh.Executor, 0, len(servers))
	defer func() {
		for _, exec := range executors {
			exec.Close()
		}
	}()

	for _, s := range servers {
		fmt.Printf("Setting up %s (%s)...\n", s.name, s.host)

		exec, err := ssh.Connect(ctx, ssh.ConnectConfig{
			Host:    s.host,
			User:    s.user,
			KeyPath: flags.Key,
		})
		if err != nil {
			return fmt.Errorf("connecting to %s: %w", s.name, err)
		}
		executors = append(executors, exec)

		fmt.Printf("  Installing %s...\n", appCfg.Network.Provider)
		if err := provider.Install(ctx, exec); err != nil {
			return fmt.Errorf("installing on %s: %w", s.name, err)
		}

		fmt.Printf("  Joining mesh...\n")
		if err := provider.Join(ctx, exec); err != nil {
			return fmt.Errorf("joining mesh on %s: %w", s.name, err)
		}

		ip, err := provider.GetIP(ctx, exec)
		if err != nil {
			return fmt.Errorf("getting VPN IP on %s: %w", s.name, err)
		}
		vpnIPs[s.name] = ip
		fmt.Printf("  VPN IP: %s\n", ip)
	}

	// Phase 2: Update DNS on all servers.
	if len(vpnIPs) > 0 {
		fmt.Println("Updating DNS entries...")
		for i, exec := range executors {
			s := servers[i]
			fmt.Printf("  Updating /etc/hosts on %s...\n", s.name)
			if err := network.UpdateDNS(ctx, exec, vpnIPs); err != nil {
				return fmt.Errorf("updating DNS on %s: %w", s.name, err)
			}
		}
	}

	fmt.Println("Network setup complete")
	return nil
}

func runNetworkStatus(flags *Flags) error {
	appCfg, err := config.LoadApp(".")
	if err != nil {
		return err
	}

	if appCfg.Network.Provider == "" {
		return fmt.Errorf("no network provider configured — add a [network] block to teploy.yml")
	}

	provider, err := network.NewProvider(network.Config{
		Provider: appCfg.Network.Provider,
		AuthKey:  appCfg.Network.AuthKey,
		Server:   appCfg.Network.Server,
		SetupKey: appCfg.Network.SetupKey,
	})
	if err != nil {
		return err
	}

	servers, err := resolveAllServers(appCfg, flags)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fmt.Printf("%-20s  %-16s  %s\n", "SERVER", "VPN IP", "STATUS")

	for _, s := range servers {
		exec, err := ssh.Connect(ctx, ssh.ConnectConfig{
			Host:    s.host,
			User:    s.user,
			KeyPath: flags.Key,
		})
		if err != nil {
			fmt.Printf("%-20s  %-16s  %s\n", s.name, "-", fmt.Sprintf("connection failed: %v", err))
			continue
		}

		ip, ipErr := provider.GetIP(ctx, exec)
		if ipErr != nil {
			ip = "-"
		}

		status, statusErr := provider.Status(ctx, exec)
		if statusErr != nil {
			status = fmt.Sprintf("error: %v", statusErr)
		} else {
			// Trim to first line for table display.
			status = splitFirstLine(status)
		}

		exec.Close()
		fmt.Printf("%-20s  %-16s  %s\n", s.name, ip, status)
	}

	return nil
}

// serverInfo holds resolved server connection details.
type serverInfo struct {
	name string
	host string
	user string
}

// resolveAllServers resolves the server list from app config.
// Uses the servers list if available, otherwise falls back to the single server.
func resolveAllServers(appCfg *config.AppConfig, flags *Flags) ([]serverInfo, error) {
	serverNames := appCfg.Servers
	if len(serverNames) == 0 && appCfg.Server != "" {
		serverNames = []string{appCfg.Server}
	}
	if len(serverNames) == 0 {
		return nil, fmt.Errorf("no servers configured — set 'server' or 'servers' in teploy.yml")
	}

	servers := make([]serverInfo, 0, len(serverNames))
	for _, name := range serverNames {
		host, user, _, err := config.ResolveServer(name, flags.Host, flags.User, flags.Key)
		if err != nil {
			return nil, fmt.Errorf("resolving server %q: %w", name, err)
		}
		servers = append(servers, serverInfo{name: name, host: host, user: user})
	}

	return servers, nil
}

// splitFirstLine returns the first line of a multi-line string.
func splitFirstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}
