package azuread

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/cenkalti/backoff"
	graph "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = &authStrengthsDataSource{}
	_ datasource.DataSourceWithConfigure = &authStrengthsDataSource{}
)

func NewAuthStrengthsDataSource() datasource.DataSource {
	return &authStrengthsDataSource{}
}

type authStrengthsDataSource struct {
	client *graph.GraphServiceClient
}

type authStrengthsDataSourceModel struct {
	AuthStrIDs   types.List `tfsdk:"ids"`
	AuthStrNames types.List `tfsdk:"names"`
}

func (d *authStrengthsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_strengths"
}

func (d *authStrengthsDataSource) Schema(_ context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "This data source provides the authentication strength policies based on the list of ids" +
			" or names of the policies. Will return all policies if input is empty.",
		Attributes: map[string]schema.Attribute{
			"ids": schema.ListAttribute{
				Description: "The IDs of the authentication strength policy.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"names": schema.ListAttribute{
				Description: "The names of the authentication strength policy.",
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *authStrengthsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	d.client = req.ProviderData.(azureadClients).graphClient
}

func (d *authStrengthsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var authStrengths models.AuthenticationStrengthPolicyCollectionResponseable
	var err error
	var plan, state authStrengthsDataSourceModel
	var policyNames, policyIDs []attr.Value
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if len(plan.AuthStrIDs.Elements()) > 0 && len(plan.AuthStrNames.Elements()) > 0 {
		resp.Diagnostics.AddError(
			"[INPUT ERROR] Invalid Input",
			"Only one of 'ids' or 'names' may be specified, not both.",
		)
		return
	}

	getAuthStrengths := func() error {
		if authStrengths, err = d.client.Policies().AuthenticationStrengthPolicies().Get(context.Background(), nil); err != nil {
			return handleAPIError(err)
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(getAuthStrengths, reconnectBackoff)

	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Unable to Retrieve Authentication Strength Policy",
			"An unexpected error occurred while retrieving the Authentication Strength Policy "+
				"from Microsoft Entra ID via Microsoft Graph API. Please verify that the provided "+
				"Authentication Strength Policy Name is correct and that the API permissions are "+
				"correctly configured.\n\n"+
				"Microsoft Graph API Error: "+err.Error(),
		)
		return
	}

	nameSlice, err := listOfStringsToSlice(plan.AuthStrNames)
	if err != nil {
		resp.Diagnostics.AddError(
			"[INPUT ERROR] Invalid Input",
			err.Error(),
		)
	}
	idSlice, err := listOfStringsToSlice(plan.AuthStrIDs)
	if err != nil {
		resp.Diagnostics.AddError(
			"[INPUT ERROR] Invalid Input",
			err.Error(),
		)
	}

	// Filter policies based on matching name or ID
	if len(nameSlice) != 0 {
		for _, policy := range authStrengths.GetValue() {
			displayName := *policy.GetDisplayName()
			if slices.Contains(nameSlice, displayName) {
				addPolicy(policy, &policyNames, &policyIDs)
			}
		}
	} else if len(idSlice) != 0 {
		for _, policy := range authStrengths.GetValue() {
			id := *policy.GetId()
			if slices.Contains(idSlice, id) {
				addPolicy(policy, &policyNames, &policyIDs)

			}
		}
	} else {
		// If no input is provided, include all policy strengths
		for _, policy := range authStrengths.GetValue() {
			addPolicy(policy, &policyNames, &policyIDs)
		}
	}

	state.AuthStrIDs, diags = types.ListValue(types.StringType, policyIDs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.AuthStrNames, diags = types.ListValue(types.StringType, policyNames)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func addPolicy(policy models.AuthenticationStrengthPolicyable, names, ids *[]attr.Value) {
	*names = append(*names, types.StringValue(*policy.GetDisplayName()))
	*ids = append(*ids, types.StringValue(fmt.Sprintf("/policies/authenticationStrengthPolicies/%s", *policy.GetId())))
}

func listOfStringsToSlice(list types.List) ([]string, error) {
	if list.IsNull() || list.IsUnknown() {
		return []string{}, nil
	}
	var result []string
	for _, v := range list.Elements() {
		strVal, ok := v.(types.String)
		if !ok {
			return nil, fmt.Errorf("expected types.String inside list, got %T", v)
		}
		result = append(result, strVal.ValueString())
	}

	return result, nil
}
