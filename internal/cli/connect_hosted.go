package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

const hostedMCPEndpoint = "https://serenity.sire.run/mcp"

type hostedConnectionStep struct {
	Purpose string   `json:"purpose"`
	Argv    []string `json:"argv,omitempty"`
	Action  string   `json:"action,omitempty"`
}

type hostedConnectionPlan struct {
	Schema           string                 `json:"schema"`
	Host             string                 `json:"host"`
	Endpoint         string                 `json:"endpoint"`
	RequestedScopes  []string               `json:"requested_scopes"`
	Binding          string                 `json:"binding"`
	Recall           string                 `json:"recall"`
	Hooks            string                 `json:"hooks"`
	AutomaticCapture string                 `json:"automatic_capture"`
	Steps            []hostedConnectionStep `json:"steps"`
}

// This command only describes native client operations. It never invokes them,
// reads credentials or host configuration, or treats a plan as qualification.
func newConnectHostedCmd() *cobra.Command {
	var host string
	var writeScope bool
	cmd := &cobra.Command{
		Use:   "hosted",
		Short: "Print a read-only native-client connection plan for hosted MCP",
		Long:  "Print a JSON connection plan for Codex or Claude Code. This does not install, authenticate, contact the service, or enable automatic capture. The separate `serenity hosted` command administers the server.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if host != "codex" && host != "claude-code" {
				return fmt.Errorf("--host must be codex or claude-code")
			}
			scopes := []string{"memory:read"}
			if writeScope {
				scopes = append(scopes, "memory:write")
			}
			plan := hostedConnectionPlan{
				Schema: "serenity.hosted-connection-plan/v1", Host: host,
				Endpoint: hostedMCPEndpoint, RequestedScopes: scopes,
				Binding: "unverified", Recall: "unverified", Hooks: "unqualified", AutomaticCapture: "disabled",
				Steps: []hostedConnectionStep{{
					Purpose: "preflight",
					Action:  "Verify current service accessibility and the intended hosted account and project. Preserve existing hooks and native trust. Inspect any existing serenity MCP entry before running add; these commands do not resolve name conflicts or provide rollback.",
				}},
			}
			if host == "codex" {
				scopeArg := "memory:read"
				if writeScope {
					scopeArg += ",memory:write"
				}
				plan.Steps = append(plan.Steps,
					hostedConnectionStep{Purpose: "configure", Argv: []string{"codex", "mcp", "add", "serenity", "--url", hostedMCPEndpoint}},
					hostedConnectionStep{Purpose: "authenticate", Argv: []string{"codex", "mcp", "login", "serenity", "--scopes", scopeArg}},
				)
			} else {
				plan.Steps = append(plan.Steps,
					hostedConnectionStep{Purpose: "configure", Argv: []string{"claude", "mcp", "add", "--transport", "http", "--scope", "local", "serenity", hostedMCPEndpoint}},
					hostedConnectionStep{Purpose: "authenticate", Action: "Open Claude Code in the intended workspace and use its native /mcp interface to authenticate. Inspect the actual requested scopes and grant only the listed requested_scopes; if the client cannot request that scope set, stop and report the limitation."},
				)
			}
			plan.Steps = append(plan.Steps, hostedConnectionStep{
				Purpose: "acceptance",
				Action:  "Verify the authenticated account/project/scopes and perform a bounded synthetic recall through the actual client. Record failure or unknown status explicitly. MCP configuration and OAuth consent alone do not qualify hooks, history import, automatic capture, deletion, replay, or RFC0003. Load the integration in a fresh session when required by the host.",
			})
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(plan)
		},
	}
	cmd.Flags().StringVar(&host, "host", "", "native client: codex or claude-code (required)")
	cmd.Flags().BoolVar(&writeScope, "write-scope", false, "include memory:write in the proposed consent scopes; grants no authority by itself")
	_ = cmd.MarkFlagRequired("host")
	return cmd
}
