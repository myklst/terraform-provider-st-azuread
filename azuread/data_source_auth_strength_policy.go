package azuread

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	graph "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"

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
	AuthStrPolicyID   types.String `tfsdk:"id"`
	AuthStrPolicyName types.String `tfsdk:"name"`
}

func (d *authStrengthPolicyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_strength_policy"
}

func (d *authStrengthPolicyDataSource) Schema(_ context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "This data source provides the authentication strength policy based on the id or name of the policy.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "ID of the authentication strength policy.",
				Optional:    true,
			},
			"name": schema.StringAttribute{
				Description: "Name of the authentication strength policy.",
				Optional:    true,
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
	var authStrengthPolicy models.AuthenticationStrengthPolicyable
	var authStrengthPolicies models.AuthenticationStrengthPolicyCollectionResponseable
	var err error
	var plan, state authStrengthPolicyDataSourceModel
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.AuthStrPolicyID != types.StringNull() {
		getAuthStrengthPolicy := func() error {
			if authStrengthPolicy, err = d.client.Policies().AuthenticationStrengthPolicies().ByAuthenticationStrengthPolicyId(plan.AuthStrPolicyID.ValueString()).Get(context.Background(), nil); err != nil {
				return handleAPIError(err)
			}
			return nil
		}

		reconnectBackoff := backoff.NewExponentialBackOff()
		reconnectBackoff.MaxElapsedTime = 30 * time.Second
		err := backoff.Retry(getAuthStrengthPolicy, reconnectBackoff)

		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Unable to Retrieve Authentication Strength Policy",
				"An unexpected error occurred while retrieving the Authentication Strength Policy "+
					"from Microsoft Entra ID via Microsoft Graph API. Please verify that the provided "+
					"Authentication Strength Policy ID is correct and that the API permissions are "+
					"correctly configured.\n\n"+
					"Microsoft Graph API Error: "+err.Error(),
			)
			return
		}

		if types.StringValue(*authStrengthPolicy.GetId()) == plan.AuthStrPolicyID {
			state.AuthStrPolicyID = plan.AuthStrPolicyID
			state.AuthStrPolicyName = types.StringValue(*authStrengthPolicy.GetDisplayName())
		}

	} else {
		getAuthStrengthPolicies := func() error {
			if authStrengthPolicies, err = d.client.Policies().AuthenticationStrengthPolicies().Get(context.Background(), nil); err != nil {
				return handleAPIError(err)
			}
			return nil
		}

		reconnectBackoff := backoff.NewExponentialBackOff()
		reconnectBackoff.MaxElapsedTime = 30 * time.Second
		err := backoff.Retry(getAuthStrengthPolicies, reconnectBackoff)

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

		if authStrengthPolicies != nil {
			for _, policy := range authStrengthPolicies.GetValue() {
				if types.StringValue(*policy.GetDisplayName()) == plan.AuthStrPolicyName {
					state.AuthStrPolicyName = plan.AuthStrPolicyName
					state.AuthStrPolicyID = types.StringValue(fmt.Sprintf("/policies/authenticationStrengthPolicies/%s", *policy.GetId()))
					break
				}
			}
		}
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func handleAPIError(err error) error {
	var graphErr *odataerrors.ODataError

	if errors.As(err, &graphErr) {
		if isAbleToRetry(graphErr.GetStatusCode()) {
			return err
		} else {
			return backoff.Permanent(err)
		}
	} else {
		return backoff.Permanent(err)
	}
}
