package copilotreview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

// NewCmdCopilotReview creates the `copilot-review` command, which manages
// Copilot code review settings and policies for repositories, organizations,
// and enterprises.
func NewCmdCopilotReview(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "copilot-review <command>",
		Short: "Manage Copilot code review settings (preview)",
		Long: heredoc.Docf(`
			Manage Copilot code review settings, runners, and policies for repositories,
			organizations, and enterprises.

			These commands configure Copilot code review; they do not request reviews. To request
			a review from Copilot, use %[1]sgh pr edit --add-reviewer @copilot%[1]s.

			Managing Copilot code review in the GitHub CLI is in preview and subject to change
			without notice.
		`, "`"),
		Example: heredoc.Doc(`
			# View the Copilot code review settings for the current repository
			$ gh copilot-review settings view

			# Leave low-severity comments out of reviews in a repository
			$ gh copilot-review settings edit -R OWNER/REPO --severity-filter low

			# Enable Copilot code review for an organization
			$ gh copilot-review policy set --org ORG --state enabled
		`),
	}

	cmd.AddCommand(newCmdSettings(f))
	cmd.AddCommand(newCmdRunner(f))
	cmd.AddCommand(newCmdPolicy(f))

	return cmd
}

type row struct {
	label string
	value string
}

// printRows prints label/value pairs, with a title and bold labels on a TTY
// and lowercase tab-separated labels otherwise.
func printRows(ios *iostreams.IOStreams, title string, rows []row) {
	cs := ios.ColorScheme()
	if ios.IsStdoutTTY() {
		fmt.Fprintf(ios.Out, "%s\n\n", cs.Bold(title))
	}
	for _, r := range rows {
		if ios.IsStdoutTTY() {
			fmt.Fprintf(ios.Out, "%s %s\n", cs.Bold(r.label+":"), r.value)
		} else {
			fmt.Fprintf(ios.Out, "%s:\t%s\n", strings.ToLower(r.label), r.value)
		}
	}
}

func printSuccessf(ios *iostreams.IOStreams, format string, args ...any) {
	if ios.IsStdoutTTY() {
		fmt.Fprintf(ios.Out, "%s %s\n", ios.ColorScheme().SuccessIcon(), fmt.Sprintf(format, args...))
	}
}

func enabledText(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

func orNotSet(s *string) string {
	if s == nil {
		return "not set"
	}
	return *s
}

// checkJSONFields rejects --json fields that the selected scope does not
// return, since the field list on --json is shared across scopes.
func checkJSONFields(exporter cmdutil.Exporter, allowed []string, scope string) error {
	if exporter == nil {
		return nil
	}
	for _, field := range exporter.Fields() {
		if !slices.Contains(allowed, field) {
			return cmdutil.FlagErrorf("field %q is not available for %s; available fields: %s", field, scope, strings.Join(allowed, ", "))
		}
	}
	return nil
}

// defaultHost returns the host for organization and enterprise requests,
// which are not tied to a repository.
func defaultHost(config func() (gh.Config, error)) (string, error) {
	cfg, err := config()
	if err != nil {
		return "", err
	}
	host, _ := cfg.Authentication().DefaultHost()
	return host, nil
}

// callAPI sends body as JSON, when non-nil, and decodes the response into data.
func callAPI(httpClient func() (*http.Client, error), host, method string, path []string, body, data any) error {
	client, err := httpClient()
	if err != nil {
		return err
	}
	p, err := safeurl.JoinPath(path...)
	if err != nil {
		return err
	}
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	return api.NewClientFromHTTP(client).REST(host, method, p.String(), r, data)
}

// checkNotBlank rejects scope flags passed with an empty value, which would
// otherwise fall back to another scope or produce an invalid path.
func checkNotBlank(cmd *cobra.Command, names ...string) error {
	for _, name := range names {
		if v, _ := cmd.Flags().GetString(name); cmd.Flags().Changed(name) && v == "" {
			return cmdutil.FlagErrorf("`--%s` cannot be blank", name)
		}
	}
	return nil
}
