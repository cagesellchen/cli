package copilotreview

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

var (
	orgPolicyFields        = []string{"configuredState", "editable", "effectiveState", "managedBy"}
	enterprisePolicyFields = []string{"policyState"}
	allPolicyFields        = []string{"configuredState", "editable", "effectiveState", "managedBy", "policyState"}

	orgPolicyStates        = []string{"enabled", "disabled"}
	enterprisePolicyStates = []string{"enabled_for_all_organizations", "disabled_for_all_organizations", "configured_by_organization_admins"}
)

// policy holds either an organization policy or an enterprise policy; the
// fields that the selected scope does not return stay empty.
type policy struct {
	ConfiguredState *string `json:"configured_state"`
	EffectiveState  string  `json:"effective_state"`
	Editable        bool    `json:"editable"`
	ManagedBy       string  `json:"managed_by"`
	PolicyState     *string `json:"policy_state"`
}

func (p *policy) ExportData(fields []string) map[string]any {
	return cmdutil.StructExportData(p, fields)
}

type policyOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	Exporter   cmdutil.Exporter

	OrgName        string
	EnterpriseName string
	State          string
}

// target returns the API path and display name for the selected organization
// or enterprise.
func (o *policyOptions) target() ([]string, string) {
	if o.OrgName != "" {
		return []string{"orgs", o.OrgName, "copilot", "policies", "code-review"}, o.OrgName
	}
	return []string{"enterprises", o.EnterpriseName, "copilot", "policies", "code-review"}, o.EnterpriseName
}

func newCmdPolicy(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy <command>",
		Short: "View and set the Copilot code review policy",
		Long: heredoc.Doc(`
			View and set whether Copilot code review is enabled for an organization or an enterprise.

			An enterprise can enable or disable Copilot code review for all of its organizations, or
			let organization owners decide.
		`),
	}

	cmd.AddCommand(newCmdPolicyView(f))
	cmd.AddCommand(newCmdPolicySet(f))

	return cmd
}

func addPolicyScopeFlags(cmd *cobra.Command, opts *policyOptions, verb string) {
	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", verb+" the policy for an `organization`")
	cmd.Flags().StringVarP(&opts.EnterpriseName, "enterprise", "e", "", verb+" the policy for an `enterprise`")
	cmd.MarkFlagsOneRequired("org", "enterprise")
	cmd.MarkFlagsMutuallyExclusive("org", "enterprise")
}

func newCmdPolicyView(f *cmdutil.Factory) *cobra.Command {
	opts := &policyOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "view",
		Short: "View the Copilot code review policy",
		Long: heredoc.Doc(`
			View the Copilot code review policy for an organization or an enterprise.

			For an organization, shows the configured and effective policy, and whether the
			organization or its enterprise manages it. Requires an organization owner.

			For an enterprise, shows the configured policy. Requires an enterprise owner.
		`),
		Example: heredoc.Doc(`
			# View the policy for an organization
			$ gh copilot-review policy view --org ORG

			# View the policy for an enterprise
			$ gh copilot-review policy view --enterprise ENTERPRISE
		`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkNotBlank(cmd, "org", "enterprise"); err != nil {
				return err
			}
			if opts.OrgName != "" {
				if err := checkJSONFields(opts.Exporter, orgPolicyFields, "organizations"); err != nil {
					return err
				}
			} else if err := checkJSONFields(opts.Exporter, enterprisePolicyFields, "enterprises"); err != nil {
				return err
			}
			return policyViewRun(opts)
		},
	}

	addPolicyScopeFlags(cmd, opts, "View")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, allPolicyFields)

	return cmd
}

func policyViewRun(opts *policyOptions) error {
	host, err := defaultHost(opts.Config)
	if err != nil {
		return err
	}
	path, name := opts.target()

	var p policy
	if err := callAPI(opts.HttpClient, host, "GET", path, nil, &p); err != nil {
		return err
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, &p)
	}

	title := fmt.Sprintf("Copilot code review policy for %s", name)
	if opts.OrgName != "" {
		printRows(opts.IO, title, []row{
			{"Effective state", p.EffectiveState},
			{"Configured state", orNotSet(p.ConfiguredState)},
			{"Managed by", p.ManagedBy},
			{"Editable", fmt.Sprint(p.Editable)},
		})
	} else {
		printRows(opts.IO, title, []row{
			{"Policy state", orNotSet(p.PolicyState)},
		})
	}
	return nil
}

func newCmdPolicySet(f *cmdutil.Factory) *cobra.Command {
	opts := &policyOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set the Copilot code review policy",
		Long: heredoc.Docf(`
			Set the Copilot code review policy for an organization or an enterprise.

			For an organization, %[1]s--state%[1]s is one of: %[2]s.
			The request fails when the organization's enterprise manages the policy.
			Requires an organization owner.

			For an enterprise, %[1]s--state%[1]s is one of: %[3]s.
			Requires an enterprise owner.
		`, "`", strings.Join(orgPolicyStates, ", "), strings.Join(enterprisePolicyStates, ", ")),
		Example: heredoc.Doc(`
			# Enable Copilot code review for an organization
			$ gh copilot-review policy set --org ORG --state enabled

			# Let organization owners decide whether to enable Copilot code review
			$ gh copilot-review policy set --enterprise ENTERPRISE --state configured_by_organization_admins
		`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkNotBlank(cmd, "org", "enterprise"); err != nil {
				return err
			}
			valid, scope := orgPolicyStates, "an organization"
			if opts.EnterpriseName != "" {
				valid, scope = enterprisePolicyStates, "an enterprise"
			}
			if !slices.Contains(valid, opts.State) {
				return cmdutil.FlagErrorf("invalid `--state` %q for %s; valid values: %s", opts.State, scope, strings.Join(valid, ", "))
			}
			return policySetRun(opts)
		},
	}

	addPolicyScopeFlags(cmd, opts, "Set")
	cmd.Flags().StringVarP(&opts.State, "state", "s", "", "The policy `state`")
	_ = cmd.MarkFlagRequired("state")

	return cmd
}

func policySetRun(opts *policyOptions) error {
	host, err := defaultHost(opts.Config)
	if err != nil {
		return err
	}
	path, name := opts.target()

	key := "state"
	if opts.EnterpriseName != "" {
		key = "policy_state"
	}
	if err := callAPI(opts.HttpClient, host, "PUT", path, map[string]any{key: opts.State}, nil); err != nil {
		return err
	}
	printSuccessf(opts.IO, "Set Copilot code review policy for %s to %s", name, opts.State)
	return nil
}
