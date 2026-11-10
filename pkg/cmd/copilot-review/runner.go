package copilotreview

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

var runnerFields = []string{"allowRepositoryCustomization", "configuredBy", "runnerGroupName", "runnerLabels", "runnerType"}

type runner struct {
	RunnerType                   string   `json:"runner_type"`
	RunnerLabels                 []string `json:"runner_labels"`
	RunnerGroupName              *string  `json:"runner_group_name"`
	AllowRepositoryCustomization bool     `json:"allow_repository_customization"`
	ConfiguredBy                 string   `json:"configured_by"`
}

func (r *runner) ExportData(fields []string) map[string]any {
	return cmdutil.StructExportData(r, fields)
}

type runnerOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	Exporter   cmdutil.Exporter

	OrgName                      string
	RunnerType                   string
	Labels                       []string
	Group                        string
	AllowRepositoryCustomization *bool
}

func (o *runnerOptions) path() []string {
	return []string{"orgs", o.OrgName, "copilot", "code-review", "runner"}
}

func newCmdRunner(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runner <command>",
		Short: "View and set the Copilot code review runner for an organization",
		Long: heredoc.Doc(`
			View and set the runners that Copilot code review uses in an organization.

			Until an organization sets a Copilot code review runner, Copilot code review uses the
			organization's Copilot coding agent runner configuration.
		`),
	}

	cmd.AddCommand(newCmdRunnerView(f))
	cmd.AddCommand(newCmdRunnerSet(f))

	return cmd
}

func newCmdRunnerView(f *cmdutil.Factory) *cobra.Command {
	opts := &runnerOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "view",
		Short: "View the Copilot code review runner for an organization",
		Long: heredoc.Doc(`
			View the runner configuration that Copilot code review uses in an organization.

			Requires an organization owner.
		`),
		Example: heredoc.Doc(`
			$ gh copilot-review runner view --org ORG
		`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkNotBlank(cmd, "org"); err != nil {
				return err
			}
			return runnerViewRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "The `organization` to view")
	_ = cmd.MarkFlagRequired("org")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, runnerFields)

	return cmd
}

func runnerViewRun(opts *runnerOptions) error {
	host, err := defaultHost(opts.Config)
	if err != nil {
		return err
	}
	var r runner
	if err := callAPI(opts.HttpClient, host, "GET", opts.path(), nil, &r); err != nil {
		return err
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, &r)
	}

	labels := "none"
	if len(r.RunnerLabels) > 0 {
		labels = strings.Join(r.RunnerLabels, ", ")
	}
	printRows(opts.IO, fmt.Sprintf("Copilot code review runner for %s", opts.OrgName), []row{
		{"Runner type", r.RunnerType},
		{"Runner labels", labels},
		{"Runner group", orNotSet(r.RunnerGroupName)},
		{"Repository customization", fmt.Sprint(r.AllowRepositoryCustomization)},
		{"Configured by", r.ConfiguredBy},
	})
	return nil
}

func newCmdRunnerSet(f *cmdutil.Factory) *cobra.Command {
	opts := &runnerOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set the Copilot code review runner for an organization",
		Long: heredoc.Docf(`
			Set the runner configuration that Copilot code review uses in an organization.

			A %[1]sstandard%[1]s runner uses GitHub-hosted runners. A %[1]slabeled%[1]s runner uses runners that
			match %[1]s--label%[1]s or %[1]s--group%[1]s, and needs at least one of them. This replaces the whole
			runner configuration. When %[1]s--allow-repo-customization%[1]s is omitted, the current value is kept.

			Requires an organization owner.
		`, "`"),
		Example: heredoc.Doc(`
			# Use GitHub-hosted runners
			$ gh copilot-review runner set --org ORG --type standard

			# Use self-hosted runners from a runner group, and stop repositories from choosing their own
			$ gh copilot-review runner set --org ORG --type labeled --group copilot-runners --allow-repo-customization=false
		`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkNotBlank(cmd, "org"); err != nil {
				return err
			}
			hasTarget := len(opts.Labels) > 0 || opts.Group != ""
			if opts.RunnerType == "labeled" && !hasTarget {
				return cmdutil.FlagErrorf("`--type labeled` requires `--label` or `--group`")
			}
			if opts.RunnerType == "standard" && hasTarget {
				return cmdutil.FlagErrorf("`--label` and `--group` require `--type labeled`")
			}
			return runnerSetRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "The `organization` to configure")
	cmdutil.StringEnumFlag(cmd, &opts.RunnerType, "type", "", "", []string{"standard", "labeled"}, "Runner type")
	cmd.Flags().StringSliceVar(&opts.Labels, "label", nil, "Use runners with this `label`")
	cmd.Flags().StringVar(&opts.Group, "group", "", "Use runners from this runner `group`")
	cmdutil.NilBoolFlag(cmd, &opts.AllowRepositoryCustomization, "allow-repo-customization", "", "Let repositories choose their own runners")
	_ = cmd.MarkFlagRequired("org")
	_ = cmd.MarkFlagRequired("type")

	return cmd
}

func runnerSetRun(opts *runnerOptions) error {
	host, err := defaultHost(opts.Config)
	if err != nil {
		return err
	}

	body := map[string]any{"runner_type": opts.RunnerType}
	if len(opts.Labels) > 0 {
		body["runner_labels"] = opts.Labels
	}
	if opts.Group != "" {
		body["runner_group_name"] = opts.Group
	}
	if opts.AllowRepositoryCustomization != nil {
		body["allow_repository_customization"] = *opts.AllowRepositoryCustomization
	}

	if err := callAPI(opts.HttpClient, host, "PUT", opts.path(), body, nil); err != nil {
		return err
	}
	printSuccessf(opts.IO, "Set Copilot code review runner for %s", opts.OrgName)
	return nil
}
