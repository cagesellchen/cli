package copilotreview

import (
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/httpmock"
)

const repoSettingsJSON = `{
	"review_effort": {"configured": null, "effective": "balanced"},
	"available_review_efforts": ["lite", "balanced"],
	"severity_filter": "low",
	"custom_instructions_enabled": true,
	"mcp_enabled": false
}`

const orgSettingsJSON = `{
	"default_review_effort": {
		"configured": "deep",
		"effective": "balanced",
		"github_default": "balanced",
		"editable": false,
		"managed_by": "enterprise"
	},
	"available_review_efforts": ["lite", "balanced"]
}`

func TestSettingsView(t *testing.T) {
	repoPath := "repos/OWNER/REPO/copilot/code-review/settings"
	orgPath := "orgs/ORG/copilot/code-review/settings"

	runCmdTests(t, []cmdTest{
		{
			name: "repository tty",
			args: []string{"settings", "view"},
			tty:  true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", repoPath), httpmock.StringResponse(repoSettingsJSON))
			},
			wantOut: heredoc.Doc(`
				Copilot code review settings for OWNER/REPO

				Review effort: balanced (inherited)
				Available review efforts: lite, balanced
				Severity filter: low
				Custom instructions: enabled
				MCP servers: disabled
			`),
		},
		{
			name: "repository override non-tty",
			args: []string{"settings", "view", "-R", "other/repo"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "repos/other/repo/copilot/code-review/settings"), httpmock.StringResponse(repoSettingsJSON))
			},
			wantOut: heredoc.Doc(`
				review effort:	balanced (inherited)
				available review efforts:	lite, balanced
				severity filter:	low
				custom instructions:	enabled
				mcp servers:	disabled
			`),
		},
		{
			name: "repository json",
			args: []string{"settings", "view", "--json", "reviewEffort,mcpEnabled"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", repoPath), httpmock.StringResponse(repoSettingsJSON))
			},
			wantOut: `{"mcpEnabled":false,"reviewEffort":{"configured":null,"effective":"balanced"}}` + "\n",
		},
		{
			name: "organization tty",
			args: []string{"settings", "view", "--org", "ORG"},
			tty:  true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", orgPath), httpmock.StringResponse(orgSettingsJSON))
			},
			wantOut: heredoc.Doc(`
				Copilot code review settings for ORG

				Default review effort: balanced (configured deep is unavailable)
				GitHub default: balanced
				Available review efforts: lite, balanced
				Managed by: enterprise
				Editable: false
			`),
		},
		{
			name: "organization json",
			args: []string{"settings", "view", "-o", "ORG", "--json", "defaultReviewEffort"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", orgPath), httpmock.StringResponse(orgSettingsJSON))
			},
			wantOut: `{"defaultReviewEffort":{"configured":"deep","editable":false,"effective":"balanced","githubDefault":"balanced","managedBy":"enterprise"}}` + "\n",
		},
		{
			name:        "repo and org",
			args:        []string{"settings", "view", "-R", "o/r", "--org", "ORG"},
			wantErr:     "specify only one of `--repo` or `--org`",
			wantFlagErr: true,
		},
		{
			name:        "repository-only json field for organization",
			args:        []string{"settings", "view", "--org", "ORG", "--json", "mcpEnabled"},
			wantErr:     `field "mcpEnabled" is not available for organizations; available fields: availableReviewEfforts, defaultReviewEffort`,
			wantFlagErr: true,
		},
		{
			name:        "organization-only json field for repository",
			args:        []string{"settings", "view", "--json", "defaultReviewEffort"},
			wantErr:     `field "defaultReviewEffort" is not available for repositories; available fields: availableReviewEfforts, customInstructionsEnabled, mcpEnabled, reviewEffort, severityFilter`,
			wantFlagErr: true,
		},
		{
			name: "not found",
			args: []string{"settings", "view"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", repoPath), jsonError(http.StatusNotFound, "Not Found"))
			},
			wantErr: "HTTP 404: Not Found (https://api.github.com/repos/OWNER/REPO/copilot/code-review/settings)",
		},
		{
			name:        "blank org",
			args:        []string{"settings", "view", "--org", ""},
			wantErr:     "`--org` cannot be blank",
			wantFlagErr: true,
		},
		{
			name:        "repository on another host uses the repository host",
			args:        []string{"settings", "view", "-R", "ghe.example.com/OWNER/REPO", "--json", "mcpEnabled"},
			defaultHost: "github.com",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("GET", "api/v3/"+repoPath), "ghe.example.com"), httpmock.StringResponse(repoSettingsJSON))
			},
			wantOut: `{"mcpEnabled":false}` + "\n",
		},
		{
			name:        "organization uses the default host",
			args:        []string{"settings", "view", "--org", "ORG", "--json", "availableReviewEfforts"},
			defaultHost: "ghe.example.com",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("GET", "api/v3/"+orgPath), "ghe.example.com"), httpmock.StringResponse(orgSettingsJSON))
			},
			wantOut: `{"availableReviewEfforts":["lite","balanced"]}` + "\n",
		},
	})
}

func TestSettingsEdit(t *testing.T) {
	repoPath := "repos/OWNER/REPO/copilot/code-review/settings"
	orgPath := "orgs/ORG/copilot/code-review/settings"

	runCmdTests(t, []cmdTest{
		{
			name: "repository all settings tty",
			args: []string{"settings", "edit", "--review-effort", "inherit", "--severity-filter", "low_and_medium", "--custom-instructions=false", "--mcp"},
			tty:  true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", repoPath), wantBody(t, http.StatusOK, map[string]any{
					"review_effort":               "inherit",
					"severity_filter":             "low_and_medium",
					"custom_instructions_enabled": false,
					"mcp_enabled":                 true,
				}))
			},
			wantOut: "✓ Updated Copilot code review settings for OWNER/REPO\n",
		},
		{
			name: "repository one setting non-tty",
			args: []string{"settings", "edit", "--mcp=false"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", repoPath), wantBody(t, http.StatusOK, map[string]any{
					"mcp_enabled": false,
				}))
			},
			wantOut: "",
		},
		{
			name: "organization",
			args: []string{"settings", "edit", "--org", "ORG", "--review-effort", "lite"},
			tty:  true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", orgPath), wantBody(t, http.StatusOK, map[string]any{
					"default_review_effort": "lite",
				}))
			},
			wantOut: "✓ Updated Copilot code review settings for ORG\n",
		},
		{
			name: "organization setting managed by enterprise",
			args: []string{"settings", "edit", "--org", "ORG", "--review-effort", "lite"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", orgPath), jsonError(http.StatusConflict, "The default review effort is managed by the enterprise."))
			},
			wantErr: "HTTP 409: The default review effort is managed by the enterprise. (https://api.github.com/orgs/ORG/copilot/code-review/settings)",
		},
		{
			name:        "no settings",
			args:        []string{"settings", "edit"},
			wantErr:     "specify at least one setting to change",
			wantFlagErr: true,
		},
		{
			name:        "repository-only setting for organization",
			args:        []string{"settings", "edit", "--org", "ORG", "--mcp"},
			wantErr:     "only `--review-effort` can be set for an organization",
			wantFlagErr: true,
		},
		{
			name:        "repo and org",
			args:        []string{"settings", "edit", "-R", "o/r", "--org", "ORG", "--review-effort", "lite"},
			wantErr:     "specify only one of `--repo` or `--org`",
			wantFlagErr: true,
		},
		{
			name:    "invalid severity filter",
			args:    []string{"settings", "edit", "--severity-filter", "high"},
			wantErr: `invalid argument "high" for "--severity-filter" flag: valid values are {no_filter|low|low_and_medium}`,
		},
		{
			name:        "blank org",
			args:        []string{"settings", "edit", "--org", "", "--review-effort", "lite"},
			wantErr:     "`--org` cannot be blank",
			wantFlagErr: true,
		},
		{
			name:        "organization uses the default host",
			args:        []string{"settings", "edit", "--org", "ORG", "--review-effort", "inherit"},
			defaultHost: "ghe.example.com",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("PATCH", "api/v3/"+orgPath), "ghe.example.com"), wantBody(t, http.StatusOK, map[string]any{
					"default_review_effort": "inherit",
				}))
			},
		},
	})
}
