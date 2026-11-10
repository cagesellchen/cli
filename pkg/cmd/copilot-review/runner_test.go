package copilotreview

import (
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/httpmock"
)

const runnerPath = "orgs/ORG/copilot/code-review/runner"

func TestRunnerView(t *testing.T) {
	labeled := `{
		"runner_type": "labeled",
		"runner_labels": ["linux", "gpu"],
		"runner_group_name": "copilot",
		"allow_repository_customization": true,
		"configured_by": "code_review"
	}`

	runCmdTests(t, []cmdTest{
		{
			name: "tty",
			args: []string{"runner", "view", "--org", "ORG"},
			tty:  true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", runnerPath), httpmock.StringResponse(labeled))
			},
			wantOut: heredoc.Doc(`
				Copilot code review runner for ORG

				Runner type: labeled
				Runner labels: linux, gpu
				Runner group: copilot
				Repository customization: true
				Configured by: code_review
			`),
		},
		{
			name: "standard non-tty",
			args: []string{"runner", "view", "-o", "ORG"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", runnerPath), httpmock.StringResponse(`{
					"runner_type": "standard",
					"runner_labels": [],
					"runner_group_name": null,
					"allow_repository_customization": false,
					"configured_by": "coding_agent"
				}`))
			},
			wantOut: heredoc.Doc(`
				runner type:	standard
				runner labels:	none
				runner group:	not set
				repository customization:	false
				configured by:	coding_agent
			`),
		},
		{
			name: "json",
			args: []string{"runner", "view", "--org", "ORG", "--json", "runnerType,runnerLabels,runnerGroupName"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", runnerPath), httpmock.StringResponse(labeled))
			},
			wantOut: `{"runnerGroupName":"copilot","runnerLabels":["linux","gpu"],"runnerType":"labeled"}` + "\n",
		},
		{
			name:    "missing org",
			args:    []string{"runner", "view"},
			wantErr: `required flag(s) "org" not set`,
		},
		{
			name:        "blank org",
			args:        []string{"runner", "view", "--org", ""},
			wantErr:     "`--org` cannot be blank",
			wantFlagErr: true,
		},
	})
}

func TestRunnerSet(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name: "labeled tty",
			args: []string{"runner", "set", "--org", "ORG", "--type", "labeled", "--label", "linux,gpu", "--group", "copilot", "--allow-repo-customization=false"},
			tty:  true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PUT", runnerPath), wantBody(t, http.StatusOK, map[string]any{
					"runner_type":                    "labeled",
					"runner_labels":                  []any{"linux", "gpu"},
					"runner_group_name":              "copilot",
					"allow_repository_customization": false,
				}))
			},
			wantOut: "✓ Set Copilot code review runner for ORG\n",
		},
		{
			name: "standard non-tty",
			args: []string{"runner", "set", "--org", "ORG", "--type", "standard"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PUT", runnerPath), wantBody(t, http.StatusOK, map[string]any{
					"runner_type": "standard",
				}))
			},
			wantOut: "",
		},
		{
			name:        "labeled without labels or group",
			args:        []string{"runner", "set", "--org", "ORG", "--type", "labeled"},
			wantErr:     "`--type labeled` requires `--label` or `--group`",
			wantFlagErr: true,
		},
		{
			name:        "standard with group",
			args:        []string{"runner", "set", "--org", "ORG", "--type", "standard", "--group", "copilot"},
			wantErr:     "`--label` and `--group` require `--type labeled`",
			wantFlagErr: true,
		},
		{
			name:    "missing type",
			args:    []string{"runner", "set", "--org", "ORG"},
			wantErr: `required flag(s) "type" not set`,
		},
		{
			name:        "blank org",
			args:        []string{"runner", "set", "--org", "", "--type", "standard"},
			wantErr:     "`--org` cannot be blank",
			wantFlagErr: true,
		},
		{
			name:        "uses the default host",
			args:        []string{"runner", "set", "--org", "ORG", "--type", "standard"},
			defaultHost: "ghe.example.com",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("PUT", "api/v3/"+runnerPath), "ghe.example.com"), wantBody(t, http.StatusOK, map[string]any{
					"runner_type": "standard",
				}))
			},
		},
	})
}
