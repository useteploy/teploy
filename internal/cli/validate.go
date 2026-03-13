package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/teploy/teploy/internal/build"
	"github.com/teploy/teploy/internal/config"
	"github.com/teploy/teploy/internal/dns"
	"github.com/teploy/teploy/internal/ssh"
)

type validationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
	Warns  []string `json:"warnings,omitempty"`
}

func newValidateCmd(flags *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Check config and server readiness",
		Long:  "Validate teploy.yml, check server connectivity, Docker, build prerequisites, and DNS.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(flags)
		},
	}
}

func runValidate(flags *Flags) error {
	result := &validationResult{Valid: true}

	// 1. Load config.
	appCfg, err := config.LoadApp(".")
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("config: %v", err))
		return outputResult(flags, result)
	}

	// 2. Check build prerequisites.
	if appCfg.Image == "" {
		mode := build.Detect(".")
		switch mode {
		case build.ModeDockerfile:
			// Dockerfile exists — good.
		case build.ModeNixpacks:
			result.Warns = append(result.Warns, "No Dockerfile found — Nixpacks will be used (requires Nixpacks on server or locally)")
		}
	}

	// 3. Check server connectivity.
	if appCfg.Server == "" && flags.Host == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "no server configured (set 'server' in teploy.yml or use --host)")
	} else {
		host, user, key, err := config.ResolveServer(appCfg.Server, flags.Host, flags.User, flags.Key)
		if err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("server resolution: %v", err))
		} else {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			executor, err := ssh.Connect(ctx, ssh.ConnectConfig{
				Host:    host,
				User:    user,
				KeyPath: key,
			})
			if err != nil {
				result.Valid = false
				result.Errors = append(result.Errors, fmt.Sprintf("SSH connection to %s@%s: %v", user, host, err))
			} else {
				defer executor.Close()

				// Check Docker.
				if _, err := executor.Run(ctx, "docker --version"); err != nil {
					result.Valid = false
					result.Errors = append(result.Errors, "Docker is not installed or not running on server")
				}

				// Check DNS.
				if appCfg.Domain != "" {
					if err := dns.Validate(appCfg.Domain, host, nil); err != nil {
						result.Warns = append(result.Warns, fmt.Sprintf("DNS: %v", err))
					}
				}
			}
		}
	}

	return outputResult(flags, result)
}

func outputResult(flags *Flags, result *validationResult) error {
	if flags.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if result.Valid {
		fmt.Println("Config is valid")
	} else {
		fmt.Println("Validation failed")
	}

	for _, e := range result.Errors {
		fmt.Printf("  ERROR: %s\n", e)
	}
	for _, w := range result.Warns {
		fmt.Printf("  WARN: %s\n", w)
	}

	if !result.Valid {
		return fmt.Errorf("validation failed with %d error(s)", len(result.Errors))
	}
	return nil
}
