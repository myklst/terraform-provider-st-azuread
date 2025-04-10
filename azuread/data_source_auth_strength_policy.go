package azuread

import (
	"context"
	"fmt"
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
	_ datasource.DataSource              = &authStrengthPolicyDataSource{}
	_ datasource.DataSourceWithConfigure = &authStrengthPolicyDataSource{}
)

func NewAuthStrengthPolicyDataSource() datasource.DataSource {
	return &authStrengthPolicyDataSource{}
}

type authStrengthPolicyDataSource struct {
	client *graph.GraphServiceClient
}

type authStrengthPolicyDataSourceModel struct {
	AuthStrPolicyIDs   types.List `tfsdk:"ids"`
	AuthStrPolicyNames types.List `tfsdk:"names"`
}

func (d *authStrengthPolicyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_strength_policy"
}

func (d *authStrengthPolicyDataSource) Schema(_ context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
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

func (d *authStrengthPolicyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	d.client = req.ProviderData.(azureadClients).graphClient
}

func (d *authStrengthPolicyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var authStrengthPolicies models.AuthenticationStrengthPolicyCollectionResponseable
	var err error
	var plan, state authStrengthPolicyDataSourceModel
	var policyNames, policyIDs []attr.Value
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	getAuthStrengthPolicies := func() error {
		if authStrengthPolicies, err = d.client.Policies().AuthenticationStrengthPolicies().Get(context.Background(), nil); err != nil {
			return handleAPIError(err)
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(getAuthStrengthPolicies, reconnectBackoff)

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

	nameSet := map[string]struct{}{}
	idSet := map[string]struct{}{}

	if !plan.AuthStrPolicyNames.IsNull() && !plan.AuthStrPolicyNames.IsUnknown() {
		for _, v := range plan.AuthStrPolicyNames.Elements() {
			name := v.(types.String).ValueString()
			nameSet[name] = struct{}{}
		}

	} else if !plan.AuthStrPolicyIDs.IsNull() && !plan.AuthStrPolicyIDs.IsUnknown() {
		for _, v := range plan.AuthStrPolicyIDs.Elements() {
			id := v.(types.String).ValueString()
			idSet[id] = struct{}{}
		}
	}

	// Filter policies based on matching name or ID
	if len(nameSet) != 0 {
		for _, policy := range authStrengthPolicies.GetValue() {
			displayName := *policy.GetDisplayName()
			if _, match := nameSet[displayName]; len(nameSet) > 0 && match {
				addPolicy(policy, &policyNames, &policyIDs)
			}
		}
	} else if len(idSet) != 0 {
		for _, policy := range authStrengthPolicies.GetValue() {
			id := *policy.GetId()
			if _, match := idSet[id]; len(idSet) > 0 && match {
				addPolicy(policy, &policyNames, &policyIDs)
			}
		}
	} else {
		// If no input is provided, include all policies
		for _, policy := range authStrengthPolicies.GetValue() {
			addPolicy(policy, &policyNames, &policyIDs)
		}
	}

	state.AuthStrPolicyIDs, diags = types.ListValue(types.StringType, policyIDs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.AuthStrPolicyNames, diags = types.ListValue(types.StringType, policyNames)
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
