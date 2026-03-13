package cli

import (
	"context"
	"fmt"
	"sort"

	"github.com/teploy/teploy/internal/config"
	"github.com/teploy/teploy/internal/ssh"
)

// connectForApp resolves the server from app config and establishes an SSH connection.
// Uses the first server from appCfg.Servers if Server is empty.
func connectForApp(ctx context.Context, flags *Flags, appCfg *config.AppConfig) (ssh.Executor, error) {
	serverName := appCfg.Server
	if serverName == "" && len(appCfg.Servers) > 0 {
		serverName = appCfg.Servers[0]
	}
	if serverName == "" {
		return nil, fmt.Errorf("no server specified — set 'server' in teploy.yml")
	}

	host, user, key, err := config.ResolveServer(serverName, flags.Host, flags.User, flags.Key)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Connecting to %s@%s...\n", user, host)
	return ssh.Connect(ctx, ssh.ConnectConfig{
		Host:    host,
		User:    user,
		KeyPath: key,
	})
}

// sortedAccessoryNames returns accessory names sorted for deterministic ordering.
func sortedAccessoryNames(accessories map[string]config.AccessoryConfig) []string {
	names := make([]string, 0, len(accessories))
	for name := range accessories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
