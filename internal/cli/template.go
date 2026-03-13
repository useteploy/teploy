package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/teploy/teploy/internal/config"
	tmpl "github.com/teploy/teploy/internal/template"
)

func newTemplateCmd(flags *Flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Deploy from community templates",
	}

	cmd.AddCommand(newTemplateListCmd(flags))
	cmd.AddCommand(newTemplateInfoCmd(flags))
	cmd.AddCommand(newTemplateDeployCmd(flags))

	return cmd
}

func newTemplateListCmd(_ *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available templates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			reg := tmpl.NewRegistry()
			templates, err := reg.List(ctx)
			if err != nil {
				return err
			}

			if len(templates) == 0 {
				fmt.Println("No templates available")
				return nil
			}

			for _, t := range templates {
				accs := ""
				if len(t.Accessories) > 0 {
					accs = fmt.Sprintf(" [%s]", joinStrings(t.Accessories))
				}
				fmt.Printf("  %-20s %s%s\n", t.Name, t.Description, accs)
			}
			return nil
		},
	}
}

func newTemplateInfoCmd(_ *Flags) *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show template details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()

			reg := tmpl.NewRegistry()
			content, err := reg.Fetch(ctx, args[0], nil)
			if err != nil {
				return err
			}

			fmt.Println(content)
			return nil
		},
	}
}

func newTemplateDeployCmd(flags *Flags) *cobra.Command {
	var domain, server string

	cmd := &cobra.Command{
		Use:   "deploy <name>",
		Short: "Deploy from a template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if domain == "" {
				return fmt.Errorf("--domain is required")
			}
			return runTemplateDeploy(flags, args[0], domain, server)
		},
	}

	cmd.Flags().StringVar(&domain, "domain", "", "domain for the app (required)")
	cmd.Flags().StringVar(&server, "server", "", "server to deploy to")

	return cmd
}

func runTemplateDeploy(flags *Flags, name, domain, server string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Fetch and render template.
	reg := tmpl.NewRegistry()
	content, err := reg.Fetch(ctx, name, map[string]string{
		"domain": domain,
	})
	if err != nil {
		return err
	}

	// Parse as AppConfig.
	appCfg, err := config.ParseAppBytes([]byte(content))
	if err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	// Override domain and server.
	appCfg.Domain = domain
	if server != "" {
		appCfg.Server = server
	}

	// Write to teploy.yml in current directory.
	if err := os.WriteFile("teploy.yml", []byte(content), 0644); err != nil {
		return fmt.Errorf("writing teploy.yml: %w", err)
	}

	fmt.Printf("Template %q written to teploy.yml\n", name)
	fmt.Printf("  App: %s\n", appCfg.App)
	fmt.Printf("  Domain: %s\n", domain)
	if len(appCfg.Accessories) > 0 {
		fmt.Println("  Accessories:")
		for accName, acc := range appCfg.Accessories {
			fmt.Printf("    %s (%s)\n", accName, acc.Image)
		}
	}
	fmt.Println("\nRun 'teploy deploy' to deploy this template.")
	return nil
}

func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}
