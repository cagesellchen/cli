package copilotreview

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

type cmdTest struct {
	name        string
	args        []string
	tty         bool
	defaultHost string
	stubs       func(*httpmock.Registry)
	wantOut     string
	wantErr     string
	wantFlagErr bool
}

func runCmdTests(t *testing.T, tests []cmdTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GH_REPO", "")
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.stubs != nil {
				tt.stubs(reg)
			}

			cfg := config.NewMockConfig()
			if tt.defaultHost != "" {
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetDefaultHost(tt.defaultHost, "GH_HOST")
					return authCfg
				}
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			f := &cmdutil.Factory{
				IOStreams:  ios,
				HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
				Config:     func() (gh.Config, error) { return cfg, nil },
				BaseRepo:   func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil },
			}

			cmd := NewCmdCopilotReview(f)
			cmd.SetArgs(tt.args)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			_, err := cmd.ExecuteC()
			require.Empty(t, stderr.String())

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				if tt.wantFlagErr {
					var flagErr *cmdutil.FlagError
					require.ErrorAs(t, err, &flagErr)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantOut, stdout.String())
		})
	}
}

// wantBody returns a responder that checks the request body against want.
func wantBody(t *testing.T, status int, want map[string]any) httpmock.Responder {
	return httpmock.RESTPayload(status, "{}", func(got map[string]any) {
		require.Equal(t, want, got)
	})
}

func jsonError(status int, message string) httpmock.Responder {
	return httpmock.WithHeader(httpmock.StatusStringResponse(status, `{"message":"`+message+`"}`), "Content-Type", "application/json")
}
