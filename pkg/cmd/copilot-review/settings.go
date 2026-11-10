package copilotreview

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

var (
	repoSettingsFields = []string{"availableReviewEfforts", "customInstructionsEnabled", "mcpEnabled", "reviewEffort", "severityFilter"}
	orgSettingsFields  = []string{"availableReviewEfforts", "defaultReviewEffort"}
	allSettingsFields  = []string{"availableReviewEfforts", "customInstructionsEnabled", "defaultReviewEffort", "mcpEnabled", "reviewEffort", "severityFilter"}
)

type repoSettings struct {
	ReviewEffort struct {
		Configured *string `json:"configured"`
		Effective  string  `json:"effective"`
	} `json:"review_effort"`
	AvailableReviewEfforts    []string `json:"available_review_efforts"`
	SeverityFilter            string   `json:"severity_filter"`
	CustomInstructionsEnabled bool     `json:"custom_instructions_enabled"`
	MCPEnabled                bool     `json:"mcp_enabled"`
}

func (s *repoSettings) ExportData(fields []string) map[string]any {
	data := cmdutil.StructExportData(s, fields)
	if _, ok := data["reviewEffort"]; ok {
		data["reviewEffort"] = map[string]any{
			"configured": s.ReviewEffort.Configured,
			"effective":  s.ReviewEffort.Effective,
		}
	}
	return data
}

type orgSettings struct {
	DefaultReviewEffort struct {
		Configured    *string `json:"configured"`
		Effective     string  `json:"effective"`
		GitHubDefault string  `json:"github_default"`
		Editable      bool    `json:"editable"`
		ManagedBy     string  `json:"managed_by"`
	} `json:"default_review_effort"`
	AvailableReviewEfforts []string `json:"available_review_efforts"`
}

func (s *orgSettings) ExportData(fields []string) map[string]any {
	data := cmdutil.StructExportData(s, fields)
	if _, ok := data["defaultReviewEffort"]; ok {
		e := s.DefaultReviewEffort
		data["defaultReviewEffort"] = map[string]any{
			"configured":    e.Configured,
			"effective":     e.Effective,
			"githubDefault": e.GitHubDefault,
			"editable":      e.Editable,
			"managedBy":     e.ManagedBy,
		}
	}
	return data
}

type settingsOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Exporter   cmdutil.Exporter

	OrgName                   string
	ReviewEffort              *string
	SeverityFilter            string
	CustomInstructionsEnabled *bool
	MCPEnabled                *bool
}

// target returns the API host, path, and display name for the selected
// repository or organization.
func (o *settingsOptions) target() (host string, path []string, name string, err error) {
	if o.OrgName != "" {
		host, err = defaultHost(o.Config)
		return host, []string{"orgs", o.OrgName, "copilot", "code-review", "settings"}, o.OrgName, err
	}
	repo, err := o.BaseRepo()
	if err != nil {
		return "", nil, "", err
	}
	return repo.RepoHost(), []string{"repos", repo.RepoOwner(), repo.RepoName(), "copilot", "code-review", "settings"}, ghrepo.FullName(repo), nil
}

func newCmdSettings(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings <command>",
		Short: "View and edit Copilot code review settings",
		Long: heredoc.Docf(`
			View and edit Copilot code review settings for a repository or an organization.

			Repository settings tune how Copilot code review reviews pull requests in the repository.
			Organization settings set the default review effort for repositories that don't choose
			their own. These settings do not enable Copilot code review; see %[1]sgh copilot-review policy%[1]s.
		`, "`"),
	}
	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(newCmdSettingsView(f))
	cmd.AddCommand(newCmdSettingsEdit(f))

	return cmd
}

func newCmdSettingsView(f *cmdutil.Factory) *cobra.Command {
	opts := &settingsOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "view",
		Short: "View Copilot code review settings",
		Long: heredoc.Doc(`
			View the Copilot code review settings for a repository or an organization.

			Without --org, shows the settings for the current repository or the one selected with --repo.
			Repository settings require the admin role; organization settings require an organization owner.
		`),
		Example: heredoc.Doc(`
			# View the settings for the current repository
			$ gh copilot-review settings view

			# View the settings for an organization
			$ gh copilot-review settings view --org ORG

			# Print the effective review effort for a repository
			$ gh copilot-review settings view -R OWNER/REPO --json reviewEffort --jq .reviewEffort.effective
		`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			if err := checkNotBlank(cmd, "org"); err != nil {
				return err
			}
			if err := cmdutil.MutuallyExclusive("specify only one of `--repo` or `--org`", cmd.Flags().Changed("repo"), opts.OrgName != ""); err != nil {
				return err
			}
			if opts.OrgName != "" {
				if err := checkJSONFields(opts.Exporter, orgSettingsFields, "organizations"); err != nil {
					return err
				}
			} else if err := checkJSONFields(opts.Exporter, repoSettingsFields, "repositories"); err != nil {
				return err
			}

			return settingsViewRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "View the settings for an `organization`")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, allSettingsFields)

	return cmd
}

func settingsViewRun(opts *settingsOptions) error {
	host, path, name, err := opts.target()
	if err != nil {
		return err
	}
	title := fmt.Sprintf("Copilot code review settings for %s", name)

	if opts.OrgName != "" {
		var s orgSettings
		if err := callAPI(opts.HttpClient, host, "GET", path, nil, &s); err != nil {
			return err
		}
		if opts.Exporter != nil {
			return opts.Exporter.Write(opts.IO, &s)
		}
		e := s.DefaultReviewEffort
		printRows(opts.IO, title, []row{
			{"Default review effort", effortText(e.Configured, e.Effective)},
			{"GitHub default", e.GitHubDefault},
			{"Available review efforts", strings.Join(s.AvailableReviewEfforts, ", ")},
			{"Managed by", e.ManagedBy},
			{"Editable", fmt.Sprint(e.Editable)},
		})
		return nil
	}

	var s repoSettings
	if err := callAPI(opts.HttpClient, host, "GET", path, nil, &s); err != nil {
		return err
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, &s)
	}
	printRows(opts.IO, title, []row{
		{"Review effort", effortText(s.ReviewEffort.Configured, s.ReviewEffort.Effective)},
		{"Available review efforts", strings.Join(s.AvailableReviewEfforts, ", ")},
		{"Severity filter", s.SeverityFilter},
		{"Custom instructions", enabledText(s.CustomInstructionsEnabled)},
		{"MCP servers", enabledText(s.MCPEnabled)},
	})
	return nil
}

// effortText describes the effective review effort and where it comes from.
// The API reports a different effective effort when the configured one is
// unavailable.
func effortText(configured *string, effective string) string {
	switch {
	case configured == nil:
		return effective + " (inherited)"
	case *configured != effective:
		return fmt.Sprintf("%s (configured %s is unavailable)", effective, *configured)
	default:
		return effective
	}
}

func newCmdSettingsEdit(f *cmdutil.Factory) *cobra.Command {
	opts := &settingsOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit Copilot code review settings",
		Long: heredoc.Docf(`
			Edit the Copilot code review settings for a repository or an organization.

			Settings that you don't specify are not changed. Use %[1]s--review-effort inherit%[1]s to clear
			the review effort so it is inherited from the organization, the enterprise, or GitHub's default.
			Only review efforts that are available to the repository or organization are accepted; see
			%[1]sgh copilot-review settings view%[1]s.

			Organizations can only set %[1]s--review-effort%[1]s, which is the default for repositories that
			don't choose their own.
		`, "`"),
		Example: heredoc.Doc(`
			# Set the review effort for the current repository
			$ gh copilot-review settings edit --review-effort balanced

			# Inherit the review effort and stop using MCP servers in a repository
			$ gh copilot-review settings edit -R OWNER/REPO --review-effort inherit --mcp=false

			# Set the default review effort for an organization
			$ gh copilot-review settings edit --org ORG --review-effort lite
		`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			if err := checkNotBlank(cmd, "org"); err != nil {
				return err
			}
			if err := cmdutil.MutuallyExclusive("specify only one of `--repo` or `--org`", cmd.Flags().Changed("repo"), opts.OrgName != ""); err != nil {
				return err
			}
			repoOnly := opts.SeverityFilter != "" || opts.CustomInstructionsEnabled != nil || opts.MCPEnabled != nil
			if opts.ReviewEffort == nil && !repoOnly {
				return cmdutil.FlagErrorf("specify at least one setting to change")
			}
			if opts.OrgName != "" && repoOnly {
				return cmdutil.FlagErrorf("only `--review-effort` can be set for an organization")
			}

			return settingsEditRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "Edit the settings for an `organization`")
	cmdutil.NilStringFlag(cmd, &opts.ReviewEffort, "review-effort", "", "Set the review `effort`, or \"inherit\" to clear it")
	cmdutil.StringEnumFlag(cmd, &opts.SeverityFilter, "severity-filter", "", "", []string{"no_filter", "low", "low_and_medium"}, "Leave review comments of these severities out of reviews")
	cmdutil.NilBoolFlag(cmd, &opts.CustomInstructionsEnabled, "custom-instructions", "", "Use the repository's custom instructions in reviews")
	cmdutil.NilBoolFlag(cmd, &opts.MCPEnabled, "mcp", "", "Let reviews use MCP servers")

	return cmd
}

func settingsEditRun(opts *settingsOptions) error {
	host, path, name, err := opts.target()
	if err != nil {
		return err
	}

	body := map[string]any{}
	if opts.OrgName != "" {
		body["default_review_effort"] = *opts.ReviewEffort
	} else {
		if opts.ReviewEffort != nil {
			body["review_effort"] = *opts.ReviewEffort
		}
		if opts.SeverityFilter != "" {
			body["severity_filter"] = opts.SeverityFilter
		}
		if opts.CustomInstructionsEnabled != nil {
			body["custom_instructions_enabled"] = *opts.CustomInstructionsEnabled
		}
		if opts.MCPEnabled != nil {
			body["mcp_enabled"] = *opts.MCPEnabled
		}
	}

	if err := callAPI(opts.HttpClient, host, "PATCH", path, body, nil); err != nil {
		return err
	}
	printSuccessf(opts.IO, "Updated Copilot code review settings for %s", name)
	return nil
}
