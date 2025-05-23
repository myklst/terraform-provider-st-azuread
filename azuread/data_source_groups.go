package azuread

import (
	"context"
	"fmt"
	"strings"

	graph "github.com/microsoftgraph/msgraph-sdk-go"
	graphgroups "github.com/microsoftgraph/msgraph-sdk-go/groups"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = &groupsDataSource{}
	_ datasource.DataSourceWithConfigure = &groupsDataSource{}
)

func NewGroupsDataSource() datasource.DataSource {
	return &groupsDataSource{}
}

type groupsDataSource struct {
	client *graph.GraphServiceClient
}

type groupsDataSourceModel struct {
	DisplayNames      []types.String `tfsdk:"display_names"`
	DisplayNamePrefix types.String   `tfsdk:"display_name_prefix"`
	IgnoreMissing     types.Bool     `tfsdk:"ignore_missing"`
	ReturnAll         types.Bool     `tfsdk:"return_all"`
	MailEnabled       types.Bool     `tfsdk:"mail_enabled"`
	SecurityEnabled   types.Bool     `tfsdk:"security_enabled"`
	Groups            []groupModel   `tfsdk:"groups"`
}

type groupModel struct {
	DisplayName types.String `tfsdk:"display_name"`
	ID          types.String `tfsdk:"id"`
}

func (d *groupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_groups"
}

func (d *groupsDataSource) Schema(_ context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "This data source provides the list of the group",
		Attributes: map[string]schema.Attribute{
			"display_names": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "The display names of the groups. Cannot be used with display_name_prefix or return_all during apply, but will be set automatically after querying the groups.",
			},
			"display_name_prefix": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Prefix to match display names. Cannot be used with `display_names` or `return_all`.",
			},
			"ignore_missing": schema.BoolAttribute{
				Optional:    true,
				Description: "Ignore missing groups if not found. Cannot be set to true when `return_all` is also true.",
			},
			"return_all": schema.BoolAttribute{
				Optional:    true,
				Description: "Return all groups. Cannot be used with `display_name_prefix` or `display_names` and cannot be set to true when `ignore_missing` is also true.",
			},
			"mail_enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether the groups are mail-enabled.",
			},
			"security_enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether the groups are security-enabled.",
			},
			"groups": schema.ListNestedAttribute{
				Description: "List of groups matching the criteria.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"display_name": schema.StringAttribute{
							Computed:    true,
							Description: "The display name of the group.",
						},
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "The ID of the group.",
						},
					},
				},
			},
		},
	}
}

func (d *groupsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	d.client = req.ProviderData.(azureadClients).graphClient
}

func (d *groupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var plan groupsDataSourceModel
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasDisplayNames := plan.DisplayNames != nil && len(plan.DisplayNames) > 0
	hasPrefix := !plan.DisplayNamePrefix.IsNull() && plan.DisplayNamePrefix.ValueString() != ""
	returnAll := !plan.ReturnAll.IsNull() && plan.ReturnAll.ValueBool()
	ignoreMissing := !plan.IgnoreMissing.IsNull() && plan.IgnoreMissing.ValueBool()

	// Start validation.
	// Enforce that only one of `display_names`, `display_name_prefix`, or `return_all` is set during apply.
	setCount := 0
	if hasDisplayNames {
		setCount++
	}
	if hasPrefix {
		setCount++
	}
	if returnAll {
		setCount++
	}
	if setCount > 1 {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			"Only one of `display_names`, `display_name_prefix`, or `return_all` can be set at a time.",
		)
		return
	}
	// Ensure that only one of return_all or ignore_missing is set to true.
	if returnAll && ignoreMissing {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			"`return_all` and `ignore_missing` cannot both be true at the same time.",
		)
		return
	}
	// End validation.

	// Build filters.
	var filters []string
	if hasPrefix {
		filters = append(filters, fmt.Sprintf("startswith(displayName,'%s')", plan.DisplayNamePrefix.ValueString()))
	}
	if !plan.MailEnabled.IsNull() {
		filters = append(filters, fmt.Sprintf("mailEnabled eq %t", plan.MailEnabled.ValueBool()))
	}
	if !plan.SecurityEnabled.IsNull() {
		filters = append(filters, fmt.Sprintf("securityEnabled eq %t", plan.SecurityEnabled.ValueBool()))
	}

	var filterStr *string
	if len(filters) > 0 {
		filter := strings.Join(filters, " and ")
		filterStr = &filter
	}

	queryParams := &graphgroups.GroupsRequestBuilderGetQueryParameters{
		Filter: filterStr,
		Select: []string{"id", "displayName"},
	}
	config := &graphgroups.GroupsRequestBuilderGetRequestConfiguration{
		QueryParameters: queryParams,
	}

	result, err := d.client.Groups().Get(ctx, config)
	if err != nil {
		resp.Diagnostics.AddError("Graph API Error", fmt.Sprintf("Failed to fetch groups: %s", err.Error()))
		return
	}

	var groups []groupModel
	inputNames := make(map[string]struct{})
	for _, dn := range plan.DisplayNames {
		if !dn.IsNull() && dn.ValueString() != "" {
			inputNames[dn.ValueString()] = struct{}{}
		}
	}

	for _, group := range result.GetValue() {
		name := group.GetDisplayName()
		id := group.GetId()
		if name == nil || id == nil {
			continue
		}

		if returnAll || hasPrefix {
			groups = append(groups, groupModel{
				DisplayName: types.StringValue(*name),
				ID:          types.StringValue(*id),
			})
		} else if _, ok := inputNames[*name]; ok {
			groups = append(groups, groupModel{
				DisplayName: types.StringValue(*name),
				ID:          types.StringValue(*id),
			})
			delete(inputNames, *name)
		}
	}

	if hasPrefix && len(groups) == 0 {
		resp.Diagnostics.AddError(
			"No Groups Found",
			fmt.Sprintf("No groups found with display name prefix: %q", plan.DisplayNamePrefix.ValueString()),
		)
		return
	}

	// Handle missing groups.
	if len(inputNames) > 0 && !ignoreMissing {
		missing := make([]string, 0, len(inputNames))
		for name := range inputNames {
			missing = append(missing, name)
		}
		resp.Diagnostics.AddError("Missing Groups", fmt.Sprintf("The following group(s) were not found: %v", missing))
		return
	}

	// Set DisplayNames from result if not provided.
	if !hasDisplayNames {
		var displayNameValues []types.String
		for _, group := range result.GetValue() {
			name := group.GetDisplayName()
			if name != nil {
				displayNameValues = append(displayNameValues, types.StringValue(*name))
			}
		}
		plan.DisplayNames = displayNameValues
	}

	// Set default values when is not set.
	if plan.DisplayNamePrefix.IsNull() {
		plan.DisplayNamePrefix = types.StringValue("")
	}
	if plan.IgnoreMissing.IsNull() {
		plan.IgnoreMissing = types.BoolValue(false)
	}
	if plan.MailEnabled.IsNull() {
		plan.MailEnabled = types.BoolValue(false)
	}
	if plan.ReturnAll.IsNull() {
		plan.ReturnAll = types.BoolValue(false)
	}
	if plan.SecurityEnabled.IsNull() {
		plan.SecurityEnabled = types.BoolValue(false)
	}

	plan.Groups = groups

	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
}
