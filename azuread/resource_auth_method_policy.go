package azuread

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	graph "github.com/microsoftgraph/msgraph-sdk-go"
	graphModels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

var (
	_ resource.Resource              = &authMethodPolicyResource{}
	_ resource.ResourceWithConfigure = &authMethodPolicyResource{}
)

func NewAuthMethodPolicyResource() resource.Resource {
	return &authMethodPolicyResource{}
}

type authMethodPolicyResource struct {
	client *graph.GraphServiceClient
}

type authMethodPolicyResourceModel struct {
	State            types.String   `tfsdk:"state"`
	Type             types.String   `tfsdk:"type"`
	ExcludedGroupIDs []types.String `tfsdk:"excluded_group_ids"`
}

func (r *authMethodPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_method_policy"
}

func (r *authMethodPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an authentication method policy on Microsoft Entra ID. Currently, " +
			" QR code and Hardware OATH Tokens are not supported in Microsoft Graph API",
		Attributes: map[string]schema.Attribute{
			"state": schema.StringAttribute{
				Description: "Whether the authentication method policy is enabled in the tenant. " +
					"Possible values are `enabled` or `disabled`.",
				Required: true,
			},
			"type": schema.StringAttribute{
				Description: "The type of the authentication method policy. Possible values are " +
					"`Email`, `Fido2`, `Microsoft` `Authenticator`, `Voice`, `Sms`, `SoftwareOath`" +
					"`TemporaryAccessPass`, `X509Certificate`",
				Required: true,
			},
			"excluded_group_ids": schema.ListAttribute{
				Description: "A list of group IDs to exclude from the authentication method policy.",
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *authMethodPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.client = req.ProviderData.(azureadClients).graphClient
}

// Will overwrite existing excluded groups
func (r *authMethodPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state authMethodPolicyResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	createDiags := r.createAuthMethodPolicy(&plan, &state)
	resp.Diagnostics.Append(createDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *authMethodPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *authMethodPolicyResourceModel
	var authenticationMethodConfigurations graphModels.AuthenticationMethodConfigurationable
	var err error
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	getAuthMethodPolicy := func() error {
		authenticationMethodConfigurations, err = r.client.Policies().
			AuthenticationMethodsPolicy().
			AuthenticationMethodConfigurations().
			ByAuthenticationMethodConfigurationId(state.Type.ValueString()).
			Get(context.Background(), nil)

		if err != nil {
			return handleAPIError(err)
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(getAuthMethodPolicy, reconnectBackoff)

	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Unable to Read Authentication Method Policy",
			"An unexpected error occurred while creating the Authentication Method Policy "+
				"from Microsoft Entra ID via Microsoft Graph API. Please verify that the provided "+
				"inputs are correct and that the API permissions are "+
				"correctly configured.\n\n"+
				"Microsoft Graph API Error: "+err.Error(),
		)
		return
	}

	var excludedGroupIDs []types.String

	excludeTargets := authenticationMethodConfigurations.GetExcludeTargets()
	for _, target := range excludeTargets {
		if target.GetId() != nil {
			excludedGroupIDs = append(excludedGroupIDs, types.StringValue(*target.GetId()))
		}
	}

	state.Type = types.StringValue(*authenticationMethodConfigurations.GetId())
	state.State = types.StringValue(authenticationMethodConfigurations.GetState().String())
	state.ExcludedGroupIDs = excludedGroupIDs

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *authMethodPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state authMethodPolicyResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteDiags := r.deleteAuthMethodPolicy(&state)
	resp.Diagnostics.Append(deleteDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	createDiags := r.createAuthMethodPolicy(&plan, &state)
	resp.Diagnostics.Append(createDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *authMethodPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *authMethodPolicyResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteDiags := r.deleteAuthMethodPolicy(state)
	resp.Diagnostics.Append(deleteDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *authMethodPolicyResource) createAuthMethodPolicy(plan, state *authMethodPolicyResourceModel) diag.Diagnostics {
	excludedGroups := []graphModels.ExcludeTargetable{} //declares a non nill empty slice
	targetType := graphModels.GROUP_AUTHENTICATIONMETHODTARGETTYPE

	for _, excludedGroupID := range plan.ExcludedGroupIDs {
		excludedGroup := graphModels.NewExcludeTarget()
		excludedGroup.SetId(StringPtr(excludedGroupID.ValueString()))
		excludedGroup.SetTargetType(&targetType)
		excludedGroups = append(excludedGroups, excludedGroup)
	}

	requestBody := r.getAuthMethodReqBody(plan.Type.ValueString())
	if requestBody == nil {
		return diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"[API ERROR] Invalid Authentication Method Type",
				fmt.Sprintf("'%v' for invalid, only acceptable values are 'Fido2', "+
					"'MicrosoftAuthenticator', 'Voice', 'Sms', 'SoftwareOath', "+
					"'TemporaryAccessPass' and 'X509Certificate'.",
					plan.Type.ValueString()),
			),
		}
	}
	authMethodPolicyState, getStateDiags := r.getState(plan.State.ValueString())
	if getStateDiags != nil {
		return getStateDiags
	}
	requestBody.SetState(&authMethodPolicyState)
	requestBody.SetExcludeTargets(excludedGroups)

	updateAuthMethodPolicy := func() error {
		_, err := r.client.Policies().
			AuthenticationMethodsPolicy().
			AuthenticationMethodConfigurations().
			ByAuthenticationMethodConfigurationId(plan.Type.ValueString()).
			Patch(context.Background(), requestBody, nil)

		if err != nil {
			return handleAPIError(err)
		}

		return err
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(updateAuthMethodPolicy, reconnectBackoff)

	if err != nil {
		return diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"[API ERROR] Unable to Create Authentication Method Policy",
				"An unexpected error occurred while creating the Authentication Method Policy "+
					"from Microsoft Entra ID via Microsoft Graph API. Please verify that the provided "+
					"inputs are correct and that the API permissions are "+
					"correctly configured.\n\n"+
					"Microsoft Graph API Error: "+err.Error(),
			),
		}
	}

	*state = *plan

	return nil
}

func (r *authMethodPolicyResource) deleteAuthMethodPolicy(state *authMethodPolicyResourceModel) diag.Diagnostics {
	requestBody := r.getAuthMethodReqBody(state.Type.ValueString())
	authMethodPolicyState, getStateDiags := r.getState("disabled")
	if getStateDiags != nil {
		return getStateDiags
	}

	requestBody.SetState(&authMethodPolicyState)
	emptyTarget := []graphModels.ExcludeTargetable{}
	requestBody.SetExcludeTargets(emptyTarget)

	deleteAuthMethodPolicy := func() error {
		_, err := r.client.Policies().
			AuthenticationMethodsPolicy().
			AuthenticationMethodConfigurations().
			ByAuthenticationMethodConfigurationId(state.Type.ValueString()).
			Patch(context.Background(), requestBody, nil)

		if err != nil {
			return handleAPIError(err)
		}

		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(deleteAuthMethodPolicy, reconnectBackoff)

	if err != nil {
		return diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"[API ERROR] Unable to Delete Authentication Method Policy",
				"An unexpected error occurred while deleting the Authentication Method Policy "+
					"from Microsoft Entra ID via Microsoft Graph API. "+
					"Microsoft Graph API Error: "+err.Error(),
			),
		}
	}

	return nil
}

func StringPtr(s string) *string {
	return &s
}

func (r *authMethodPolicyResource) getState(authMethodState string) (graphModels.AuthenticationMethodState, diag.Diagnostics) {
	var state graphModels.AuthenticationMethodState

	switch authMethodState {
	case "enabled":
		state = graphModels.ENABLED_AUTHENTICATIONMETHODSTATE
		return state, nil
	case "disabled":
		state = graphModels.DISABLED_AUTHENTICATIONMETHODSTATE
		return state, nil
	default:
		return state, diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"[API ERROR] Invalid Authentication Method State",
				fmt.Sprintf("'%v' is invalid, only acceptable values are 'enabled' and 'disabled' only.",
					authMethodState),
			),
		}
	}
}

func (r *authMethodPolicyResource) getAuthMethodReqBody(authMethodType string) graphModels.AuthenticationMethodConfigurationable {
	var requestBody graphModels.AuthenticationMethodConfigurationable

	switch authMethodType {
	case "Email":
		requestBody = graphModels.NewEmailAuthenticationMethodConfiguration()
	case "Fido2":
		requestBody = graphModels.NewFido2AuthenticationMethodConfiguration()
	case "MicrosoftAuthenticator":
		requestBody = graphModels.NewMicrosoftAuthenticatorAuthenticationMethodConfiguration()
	case "Voice":
		requestBody = graphModels.NewVoiceAuthenticationMethodConfiguration()
	case "Sms":
		requestBody = graphModels.NewSmsAuthenticationMethodConfiguration()
	case "SoftwareOath":
		requestBody = graphModels.NewSoftwareOathAuthenticationMethodConfiguration()
	case "TemporaryAccessPass":
		requestBody = graphModels.NewTemporaryAccessPassAuthenticationMethodConfiguration()
	case "X509Certificate":
		requestBody = graphModels.NewX509CertificateAuthenticationMethodConfiguration()
	default:
		requestBody = nil
	}

	return requestBody
}
