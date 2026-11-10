package copilotreview

import (
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/httpmock"
)

const (
	orgPolicyPath        = "orgs/ORG/copilot/policies/code-review"
	enterprisePolicyPath = "enterprises/ENT/copilot/policies/code-review"
)

func TestPolicyView(t *testing.T) {
	orgPolicy := `{"configured_state": null, "effective_state": "enabled", "editable": false, "managed_by": "enterprise"}`

	runCmdTests(t, []cmdTest{
		{
			name: "organization tty",
			args: []string{"policy", "view", "--org", "ORG"},
			tty:  true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", orgPolicyPath), httpmock.StringResponse(orgPolicy))
			},
			wantOut: heredoc.Doc(`
				Copilot code review policy for ORG

				Effective state: enabled
				Configured state: not set
				Managed by: enterprise
				Editable: false
			`),
		},
		{
			name: "organization json",
			args: []string{"policy", "view", "--org", "ORG", "--json", "configuredState,effectiveState"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", orgPolicyPath), httpmock.StringResponse(orgPolicy))
			},
			wantOut: `{"configuredState":null,"effectiveState":"enabled"}` + "\n",
		},
		{
			name: "enterprise non-tty",
			args: []string{"policy", "view", "-e", "ENT"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", enterprisePolicyPath), httpmock.StringResponse(`{"policy_state": "configured_by_organization_admins"}`))
			},
			wantOut: "policy state:\tconfigured_by_organization_admins\n",
		},
		{
			name: "enterprise json",
			args: []string{"policy", "view", "--enterprise", "ENT", "--json", "policyState"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", enterprisePolicyPath), httpmock.StringResponse(`{"policy_state": null}`))
			},
			wantOut: `{"policyState":null}` + "\n",
		},
		{
			name:        "organization-only json field for enterprise",
			args:        []string{"policy", "view", "--enterprise", "ENT", "--json", "managedBy"},
			wantErr:     `field "managedBy" is not available for enterprises; available fields: policyState`,
			wantFlagErr: true,
		},
		{
			name:    "no scope",
			args:    []string{"policy", "view"},
			wantErr: "at least one of the flags in the group [org enterprise] is required",
		},
		{
			name:    "both scopes",
			args:    []string{"policy", "view", "--org", "ORG", "--enterprise", "ENT"},
			wantErr: "if any flags in the group [org enterprise] are set none of the others can be; [enterprise org] were all set",
		},
		{
			name:        "blank org",
			args:        []string{"policy", "view", "--org", ""},
			wantErr:     "`--org` cannot be blank",
			wantFlagErr: true,
		},
		{
			name:        "enterprise uses the default host",
			args:        []string{"policy", "view", "--enterprise", "ENT", "--json", "policyState"},
			defaultHost: "ghe.example.com",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("GET", "api/v3/"+enterprisePolicyPath), "ghe.example.com"), httpmock.StringResponse(`{"policy_state": "enabled_for_all_organizations"}`))
			},
			wantOut: `{"policyState":"enabled_for_all_organizations"}` + "\n",
		},
	})
}

func TestPolicySet(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name: "organization tty",
			args: []string{"policy", "set", "--org", "ORG", "--state", "enabled"},
			tty:  true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PUT", orgPolicyPath), wantBody(t, http.StatusNoContent, map[string]any{
					"state": "enabled",
				}))
			},
			wantOut: "✓ Set Copilot code review policy for ORG to enabled\n",
		},
		{
			name: "enterprise non-tty",
			args: []string{"policy", "set", "--enterprise", "ENT", "--state", "disabled_for_all_organizations"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PUT", enterprisePolicyPath), wantBody(t, http.StatusNoContent, map[string]any{
					"policy_state": "disabled_for_all_organizations",
				}))
			},
			wantOut: "",
		},
		{
			name: "organization policy managed by enterprise",
			args: []string{"policy", "set", "--org", "ORG", "--state", "disabled"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PUT", orgPolicyPath), jsonError(http.StatusConflict, "The Copilot code review policy is managed by the enterprise."))
			},
			wantErr: "HTTP 409: The Copilot code review policy is managed by the enterprise. (https://api.github.com/orgs/ORG/copilot/policies/code-review)",
		},
		{
			name:        "enterprise state for organization",
			args:        []string{"policy", "set", "--org", "ORG", "--state", "enabled_for_all_organizations"},
			wantErr:     `invalid ` + "`--state`" + ` "enabled_for_all_organizations" for an organization; valid values: enabled, disabled`,
			wantFlagErr: true,
		},
		{
			name:        "organization state for enterprise",
			args:        []string{"policy", "set", "--enterprise", "ENT", "--state", "enabled"},
			wantErr:     `invalid ` + "`--state`" + ` "enabled" for an enterprise; valid values: enabled_for_all_organizations, disabled_for_all_organizations, configured_by_organization_admins`,
			wantFlagErr: true,
		},
		{
			name:    "missing state",
			args:    []string{"policy", "set", "--org", "ORG"},
			wantErr: `required flag(s) "state" not set`,
		},
		{
			name:        "blank enterprise",
			args:        []string{"policy", "set", "--enterprise", "", "--state", "enabled"},
			wantErr:     "`--enterprise` cannot be blank",
			wantFlagErr: true,
		},
	})
}
