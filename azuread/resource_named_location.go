package azuread

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"

	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	graphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

var (
	_ resource.Resource              = &namedLocationResource{}
	_ resource.ResourceWithConfigure = &namedLocationResource{}
)

func NewNamedLocationResource() resource.Resource {
	return &namedLocationResource{}
}

type namedLocationResource struct {
	client *msgraphsdk.GraphServiceClient
}

type namedLocationResourceModel struct {
	ID          types.String  `tfsdk:"id"`
	DisplayName types.String  `tfsdk:"display_name"`
	IP          *ipBlock      `tfsdk:"ip"`
	Country     *countryBlock `tfsdk:"country"`
}

type ipBlock struct {
	IPRanges types.List `tfsdk:"ip_ranges"`
	Trusted  types.Bool `tfsdk:"trusted"`
}

type countryBlock struct {
	CountriesAndRegions               types.List   `tfsdk:"countries_and_regions"`
	CountryLookupMethod               types.String `tfsdk:"country_lookup_method"`
	IncludeUnknownCountriesAndRegions types.Bool   `tfsdk:"include_unknown_countries_and_regions"`
}

func (r *namedLocationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_named_location"
}

func (r *namedLocationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Named Location within Azure Active Directory.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID for the named location.",
			},
			"display_name": schema.StringAttribute{
				Required:    true,
				Description: "The display name for the named location.",
			},
		},
		Blocks: map[string]schema.Block{
			"ip": schema.SingleNestedBlock{
				Description: "IP-based named location settings.",
				Attributes: map[string]schema.Attribute{
					"ip_ranges": schema.ListAttribute{
						ElementType: types.StringType,
						Optional:    true,
						Description: "List of IP ranges in CIDR format.",
					},
					"trusted": schema.BoolAttribute{
						Optional:    true,
						Description: "Whether the named location is trusted. Defaults to 'false'.",
					},
				},
			},
			"country": schema.SingleNestedBlock{
				Description: "Country-based named location settings.",
				Attributes: map[string]schema.Attribute{
					"countries_and_regions": schema.ListAttribute{
						ElementType: types.StringType,
						Optional:    true,
						Description: "List of countries and/or regions in two-letter format specified by ISO 3166-2.",
					},
					"include_unknown_countries_and_regions": schema.BoolAttribute{
						Optional:    true,
						Description: "Whether IP addresses that don't map to a country or region should be included in the named location. Defaults to 'false'.",
					},
					"country_lookup_method": schema.StringAttribute{
						Optional:    true,
						Description: "Method of detecting country the user is located in. Possible values are 'clientIpAddress' for IP-based location and 'authenticatorAppGps' for Authenticator app GPS-based location. Defaults to 'clientIpAddress'.",
					},
				},
			},
		},
	}
}

func (r *namedLocationResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.client = req.ProviderData.(azureadClients).graphClient
}

func (r *namedLocationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan namedLocationResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// To check whether is country-based named location or IP-based named location.
	if plan.IP != nil {
		err := r.createIPNamedLocation(&plan)
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to create IP named location.",
				err.Error(),
			)
			return
		}
	} else if plan.Country != nil {
		err := r.createCountryNamedLocation(&plan, resp)
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to create country named location.",
				err.Error(),
			)
			return
		}
	}

	state := namedLocationResourceModel{
		ID:          plan.ID,
		DisplayName: plan.DisplayName,
		IP:          plan.IP,
		Country:     plan.Country,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *namedLocationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state namedLocationResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Since the ID in state is '/identity/conditionalAccess/namedLocations/{id}', the ID use in here need to be ID only whitout '/identity/conditionalAccess/namedLocations/'.
	fullID := state.ID.ValueString()
	parts := strings.Split(fullID, "/")
	namedLocationId := parts[len(parts)-1]

	result, err := r.client.Identity().
		ConditionalAccess().
		NamedLocations().
		ByNamedLocationId(namedLocationId).
		Get(ctx, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Microsoft Graph Error",
			fmt.Sprintf("Failed to retrieve named location: %s", err.Error()),
		)
		return
	}

	if result == nil || result.GetOdataType() == nil {
		resp.Diagnostics.AddError(
			"Invalid Response",
			"Named location does not exist or has an invalid @odata.type.",
		)
		return
	}

	odataType := *result.GetOdataType()
	switch odataType {
	case "#microsoft.graph.countryNamedLocation":
		countryNamedLocation, ok := result.(graphmodels.CountryNamedLocationable)
		if !ok {
			resp.Diagnostics.AddError(
				"Type Assertion Failed",
				"Could not convert result to CountryNamedLocationable.",
			)
			return
		}

		if val := countryNamedLocation.GetDisplayName(); val != nil {
			state.DisplayName = types.StringValue(*val)
		}
		if val := countryNamedLocation.GetIncludeUnknownCountriesAndRegions(); val != nil {
			state.Country.IncludeUnknownCountriesAndRegions = types.BoolValue(*val)
		}
		if val := countryNamedLocation.GetCountryLookupMethod(); val != nil {
			var method string

			switch *val {
			case graphmodels.CountryLookupMethodType(0):
				method = "clientIpAddress"
			case graphmodels.CountryLookupMethodType(1):
				method = "authenticatorAppGps"
			case graphmodels.CountryLookupMethodType(2):
				method = "unknownFutureValue"
			default:
				method = "unknown"
			}

			state.Country.CountryLookupMethod = types.StringValue(method)
		}

		countryList := countryNamedLocation.GetCountriesAndRegions()
		var countriesAndRegions []types.String
		for _, c := range countryList {
			countriesAndRegions = append(countriesAndRegions, types.StringValue(c))
		}
		state.Country.CountriesAndRegions, diags = types.ListValueFrom(ctx, types.StringType, countriesAndRegions)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		state.IP = nil

	case "#microsoft.graph.ipNamedLocation":
		ipNamedLocation, ok := result.(graphmodels.IpNamedLocationable)
		if !ok {
			resp.Diagnostics.AddError(
				"Type Assertion Failed",
				"Could not convert result to IpNamedLocationable.",
			)
			return
		}

		if val := ipNamedLocation.GetDisplayName(); val != nil {
			state.DisplayName = types.StringValue(*val)
		}
		if val := ipNamedLocation.GetIsTrusted(); val != nil {
			state.IP.Trusted = types.BoolValue(*val)
		}

		ipList := ipNamedLocation.GetIpRanges()
		var ipStrings []types.String

		for _, ip := range ipList {
			switch v := ip.(type) {
			case graphmodels.IPv4CidrRangeable:
				if cidr := v.GetCidrAddress(); cidr != nil {
					ipStrings = append(ipStrings, types.StringValue(*cidr))
				}
			case graphmodels.IPv6CidrRangeable:
				if cidr := v.GetCidrAddress(); cidr != nil {
					ipStrings = append(ipStrings, types.StringValue(*cidr))
				}
			default:
				resp.Diagnostics.AddError(
					"Unsupported IP Range Type",
					fmt.Sprintf("Unknown IP range type: %T", v),
				)
				return
			}
		}

		state.IP.IPRanges, diags = types.ListValueFrom(ctx, types.StringType, ipStrings)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		state.Country = nil

	default:
		resp.Diagnostics.AddError(
			"Unsupported Named Location Type",
			fmt.Sprintf("Unexpected @odata.type '%s' for named location.", odataType),
		)
		return
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *namedLocationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan namedLocationResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state namedLocationResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.IP != nil {
		if err := r.updateIpNamedLocation(ctx, &plan, &state); err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to update named location.",
				err.Error(),
			)
			return
		}
	} else if plan.Country != nil {
		if err := r.updateCountryNamedLocation(ctx, &plan, &state, (*resource.CreateResponse)(resp)); err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to update named location.",
				err.Error(),
			)
			return
		}
	}

	plan.ID = state.ID
	resp.State.Set(ctx, &plan)
}

func (r *namedLocationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state namedLocationResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	fullID := state.ID.ValueString()
	parts := strings.Split(fullID, "/")
	namedLocationID := parts[len(parts)-1]

	// If it's an IP-based named location and it's trusted, set IsTrusted to false before deletion
	if state.IP != nil && state.IP.Trusted.ValueBool() {
		isTrusted := false
		updateIpNamedLocationRequest := graphmodels.NewIpNamedLocation()
		updateIpNamedLocationRequest.SetIsTrusted(&isTrusted)
		displayName := state.DisplayName.ValueString()
		updateIpNamedLocationRequest.SetDisplayName(&displayName)

		var ipRanges []graphmodels.IpRangeable

		for _, ipVal := range state.IP.IPRanges.Elements() {
			cidrStrVal, ok := ipVal.(types.String)
			if !ok {
				resp.Diagnostics.AddError(
					"Invalid IP Range Type",
					"Expected a string for IP range, but got a different type",
				)
				return
			}

			cidr := cidrStrVal.ValueString()
			ip, _, err := net.ParseCIDR(cidr)
			if err != nil {
				resp.Diagnostics.AddError(
					"Invalid CIDR Format",
					fmt.Sprintf("Failed to parse CIDR: %s", cidr),
				)
				return
			}

			if ip.To4() != nil {
				ipv4Range := graphmodels.NewIPv4CidrRange()
				ipv4Range.SetCidrAddress(&cidr)
				ipv4Type := "#microsoft.graph.iPv4CidrRange"
				ipv4Range.SetOdataType(&ipv4Type)
				ipRanges = append(ipRanges, ipv4Range)
			} else {
				ipv6Range := graphmodels.NewIPv6CidrRange()
				ipv6Range.SetCidrAddress(&cidr)
				ipv6Type := "#microsoft.graph.iPv6CidrRange"
				ipv6Range.SetOdataType(&ipv6Type)
				ipRanges = append(ipRanges, ipv6Range)
			}
		}

		log.Printf("Updating named location with CIDRs: %v", ipRanges)
		updateIpNamedLocationRequest.SetIpRanges(ipRanges)
		odataType := "#microsoft.graph.ipNamedLocation"
		updateIpNamedLocationRequest.SetOdataType(&odataType)

		// Backoff retry for the PATCH request
		var patchErr error
		backoff := 2 * time.Second
		for i := 0; i < 5; i++ {
			_, patchErr = r.client.Identity().
				ConditionalAccess().
				NamedLocations().
				ByNamedLocationId(namedLocationID).
				Patch(ctx, updateIpNamedLocationRequest, nil)

			if patchErr == nil {
				break
			}

			log.Printf("Patch attempt %d failed: %s. Retrying in %s...", i+1, patchErr.Error(), backoff)
			time.Sleep(backoff)
			backoff *= 2
		}

		if patchErr != nil {
			resp.Diagnostics.AddError(
				"Unable to update Named Location",
				fmt.Sprintf("Failed to update Named Location with ID %q after %d retries: %s", namedLocationID, 5, patchErr.Error()),
			)
			return
		}

		time.Sleep(1 * time.Minute)
	}

	err := r.client.Identity().
		ConditionalAccess().
		NamedLocations().
		ByNamedLocationId(namedLocationID).
		Delete(ctx, nil)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Named Location",
			fmt.Sprintf("Error deleting Named Location with ID %q: %s", namedLocationID, err.Error()),
		)
		return
	}
}

func (d *namedLocationResource) createIPNamedLocation(plan *namedLocationResourceModel) error {
	createIPNamedLocationRequest := graphmodels.NewIpNamedLocation()
	odataType := "#microsoft.graph.ipNamedLocation"
	createIPNamedLocationRequest.SetOdataType(&odataType)
	createIPNamedLocationRequest.SetDisplayName(plan.DisplayName.ValueStringPointer())

	if plan.IP.Trusted.IsNull() || plan.IP.Trusted.IsUnknown() {
		trusted := false
		createIPNamedLocationRequest.SetIsTrusted(&trusted)
	} else {
		createIPNamedLocationRequest.SetIsTrusted(plan.IP.Trusted.ValueBoolPointer())
	}

	var ipRanges []graphmodels.IpRangeable

	for _, attrVal := range plan.IP.IPRanges.Elements() {
		cidrStrVal, ok := attrVal.(types.String)
		if !ok {
			return fmt.Errorf("invalid IP range type, expected string")
		}

		cidr := cidrStrVal.ValueString()

		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("invalid CIDR format: %s", cidr)
		}

		if ip.To4() != nil {
			ipv4Range := graphmodels.NewIPv4CidrRange()
			ipv4Range.SetCidrAddress(&cidr)

			ipv4Type := "#microsoft.graph.iPv4CidrRange"
			ipv4Range.SetOdataType(&ipv4Type)

			ipRanges = append(ipRanges, ipv4Range)
		} else {
			ipv6Range := graphmodels.NewIPv6CidrRange()
			ipv6Range.SetCidrAddress(&cidr)

			ipv6Type := "#microsoft.graph.iPv6CidrRange"
			ipv6Range.SetOdataType(&ipv6Type)

			ipRanges = append(ipRanges, ipv6Range)
		}
	}

	createIPNamedLocationRequest.SetIpRanges(ipRanges)

	createIPNamedLocation := func() error {
		// Check if a named location with the same DisplayName already exists
		existingLocations, err := d.client.Identity().
			ConditionalAccess().
			NamedLocations().
			Get(context.TODO(), nil)
		if err != nil {
			return err
		}

		for _, location := range existingLocations.GetValue() {
			if location.GetDisplayName() != nil && *location.GetDisplayName() == *plan.DisplayName.ValueStringPointer() {
				plan.ID = types.StringValue("/identity/conditionalAccess/namedLocations/" + *location.GetId())
				return nil
			}
		}

		// If not found, create a new named location
		createdNamedLocation, err := d.client.Identity().
			ConditionalAccess().
			NamedLocations().
			Post(
				context.TODO(),
				createIPNamedLocationRequest,
				nil,
			)
		if err != nil {
			if t, ok := err.(*errors.TencentCloudSDKError); ok {
				codeStr := t.GetCode()
				codeInt, convErr := strconv.Atoi(codeStr)
				if convErr != nil {
					return backoff.Permanent(fmt.Errorf("failed to convert error code to int: %v", convErr))
				}
				if isAbleToRetry(codeInt) {
					return err
				}
				return backoff.Permanent(err)
			}
			return err
		}

		if createdNamedLocation.GetId() == nil || *createdNamedLocation.GetId() == "" {
			return backoff.Permanent(fmt.Errorf("created IPNamedLocation returned no ID"))
		}

		plan.ID = types.StringValue("/identity/conditionalAccess/namedLocations/" + *createdNamedLocation.GetId())

		// Wait for 1 minutes to ensure the resource has been created before displaying 'Apply Successfully!' to prevent errors.
		time.Sleep(1 * time.Minute)

		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	return backoff.Retry(createIPNamedLocation, reconnectBackoff)
}

func (d *namedLocationResource) createCountryNamedLocation(plan *namedLocationResourceModel, resp *resource.CreateResponse) error {
	createCountryNamedLocationRequest := graphmodels.NewCountryNamedLocation()

	odataType := "#microsoft.graph.countryNamedLocation"
	createCountryNamedLocationRequest.SetOdataType(&odataType)

	// Set DisplayName and IsTrusted
	createCountryNamedLocationRequest.SetDisplayName(plan.DisplayName.ValueStringPointer())

	if plan.Country.IncludeUnknownCountriesAndRegions.IsNull() || plan.Country.IncludeUnknownCountriesAndRegions.IsUnknown() {
		trusted := false
		createCountryNamedLocationRequest.SetIncludeUnknownCountriesAndRegions(&trusted)
	} else {
		createCountryNamedLocationRequest.SetIncludeUnknownCountriesAndRegions(plan.Country.IncludeUnknownCountriesAndRegions.ValueBoolPointer())
	}

	var countriesAndRegions []string

	for _, countryAndRegion := range plan.Country.CountriesAndRegions.Elements() {
		strVal, ok := countryAndRegion.(types.String)
		if !ok {
			return fmt.Errorf("expected types.String but got %T", countryAndRegion)
		}
		countriesAndRegions = append(countriesAndRegions, strVal.ValueString())
	}

	createCountryNamedLocationRequest.SetCountriesAndRegions(countriesAndRegions)

	var diags diag.Diagnostics
	var method graphmodels.CountryLookupMethodType
	lookupMethod := plan.Country.CountryLookupMethod

	if lookupMethod.IsNull() || lookupMethod.IsUnknown() {
		method = graphmodels.CountryLookupMethodType(0)
	} else {
		switch lookupMethod.ValueString() {
		case "clientIpAddress":
			method = graphmodels.CountryLookupMethodType(0)
		case "authenticatorAppGps":
			method = graphmodels.CountryLookupMethodType(1)
		case "unknownFutureValue":
			method = graphmodels.CountryLookupMethodType(2)
		default:
			diags.AddError(
				"Invalid Country Lookup Method",
				fmt.Sprintf("Unknown lookup method: %s", lookupMethod.ValueString()),
			)
			resp.Diagnostics.Append(diags...)
			return fmt.Errorf("invalid country lookup method: %s", lookupMethod.ValueString())
		}
	}

	createCountryNamedLocationRequest.SetCountryLookupMethod(&method)

	createCountryNamedLocation := func() error {
		// Check if a named location with the same DisplayName already exists
		existingLocations, err := d.client.Identity().
			ConditionalAccess().
			NamedLocations().
			Get(context.TODO(), nil)
		if err != nil {
			return err
		}

		for _, location := range existingLocations.GetValue() {
			if location.GetDisplayName() != nil && *location.GetDisplayName() == *plan.DisplayName.ValueStringPointer() {
				plan.ID = types.StringValue("/identity/conditionalAccess/namedLocations/" + *location.GetId())
				return nil
			}
		}

		createdNamedLocation, err := d.client.Identity().
			ConditionalAccess().
			NamedLocations().
			Post(
				context.TODO(),
				createCountryNamedLocationRequest,
				nil,
			)
		if err != nil {
			if t, ok := err.(*errors.TencentCloudSDKError); ok {
				codeStr := t.GetCode()
				codeInt, convErr := strconv.Atoi(codeStr)
				if convErr != nil {
					return backoff.Permanent(fmt.Errorf("failed to convert error code to int: %v", convErr))
				}
				if isAbleToRetry(codeInt) {
					return err
				}
				return backoff.Permanent(err)
			}
			return err
		}

		if createdNamedLocation.GetId() == nil || *createdNamedLocation.GetId() == "" {
			return backoff.Permanent(fmt.Errorf("created CountryNamedLocation returned no ID"))
		}
		plan.ID = types.StringValue("/identity/conditionalAccess/namedLocations/" + *createdNamedLocation.GetId())

		// Wait for 1 minutes to ensure the resource has been created before displaying 'Apply Successfully!' to prevent errors.
		time.Sleep(1 * time.Minute)

		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	return backoff.Retry(createCountryNamedLocation, reconnectBackoff)
}

func (d *namedLocationResource) updateIpNamedLocation(ctx context.Context, plan, state *namedLocationResourceModel) error {
	updateIpNamedLocationRequest := graphmodels.NewIpNamedLocation()

	// Extract the ID from previous state
	fullID := state.ID.ValueString()
	parts := strings.Split(fullID, "/")
	namedLocationId := parts[len(parts)-1]

	odataType := "#microsoft.graph.ipNamedLocation"
	updateIpNamedLocationRequest.SetOdataType(&odataType)

	displayName := plan.DisplayName.ValueString()
	updateIpNamedLocationRequest.SetDisplayName(&displayName)

	updateIpNamedLocationRequest.SetIsTrusted(
		plan.IP.Trusted.ValueBoolPointer(),
	)

	var ipRanges []graphmodels.IpRangeable

	for _, ipVal := range plan.IP.IPRanges.Elements() {
		cidrStrVal, ok := ipVal.(types.String)
		if !ok {
			return fmt.Errorf("invalid IP range type, expected string")
		}

		cidr := cidrStrVal.ValueString()

		// Parse the CIDR to determine whether it's IPv4 or IPv6
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("invalid CIDR format: %s", cidr)
		}

		if ip.To4() != nil {
			// IPv4
			ipv4Range := graphmodels.NewIPv4CidrRange()
			ipv4Range.SetCidrAddress(&cidr)

			ipv4Type := "#microsoft.graph.iPv4CidrRange"
			ipv4Range.SetOdataType(&ipv4Type)

			ipRanges = append(ipRanges, ipv4Range)
		} else {
			// IPv6
			ipv6Range := graphmodels.NewIPv6CidrRange()
			ipv6Range.SetCidrAddress(&cidr)

			ipv6Type := "#microsoft.graph.iPv6CidrRange"
			ipv6Range.SetOdataType(&ipv6Type)

			ipRanges = append(ipRanges, ipv6Range)
		}
	}

	updateIpNamedLocationRequest.SetIpRanges(ipRanges)

	updateIpNamedLocation := func() error {
		_, err := d.client.Identity().
			ConditionalAccess().
			NamedLocations().
			ByNamedLocationId(namedLocationId).
			Patch(ctx, updateIpNamedLocationRequest, nil)

		if err != nil {
			log.Printf("Error during update: %v", err)
			return err
		}
		log.Println("Named location updated successfully.")
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	return backoff.Retry(updateIpNamedLocation, reconnectBackoff)
}

func (d *namedLocationResource) updateCountryNamedLocation(ctx context.Context, plan, state *namedLocationResourceModel, resp *resource.CreateResponse) error {
	updateCountryNamedLocationRequest := graphmodels.NewCountryNamedLocation()

	// Extract the ID from previous state
	fullID := state.ID.ValueString()
	parts := strings.Split(fullID, "/")
	namedLocationId := parts[len(parts)-1]

	odataType := "#microsoft.graph.countryNamedLocation"
	updateCountryNamedLocationRequest.SetOdataType(&odataType)

	displayName := plan.DisplayName.ValueString()
	updateCountryNamedLocationRequest.SetDisplayName(&displayName)

	if plan.Country.IncludeUnknownCountriesAndRegions.IsNull() || plan.Country.IncludeUnknownCountriesAndRegions.IsUnknown() {
		trusted := false
		updateCountryNamedLocationRequest.SetIncludeUnknownCountriesAndRegions(&trusted)
	} else {
		updateCountryNamedLocationRequest.SetIncludeUnknownCountriesAndRegions(plan.Country.IncludeUnknownCountriesAndRegions.ValueBoolPointer())
	}

	var diags diag.Diagnostics
	var method graphmodels.CountryLookupMethodType
	lookupMethod := plan.Country.CountryLookupMethod

	if lookupMethod.IsNull() || lookupMethod.IsUnknown() {
		method = graphmodels.CountryLookupMethodType(0)
	} else {
		switch lookupMethod.ValueString() {
		case "clientIpAddress":
			method = graphmodels.CountryLookupMethodType(0)
		case "authenticatorAppGps":
			method = graphmodels.CountryLookupMethodType(1)
		case "unknownFutureValue":
			method = graphmodels.CountryLookupMethodType(2)
		default:
			diags.AddError(
				"Invalid Country Lookup Method",
				fmt.Sprintf("Unknown lookup method: %s", lookupMethod.ValueString()),
			)
			resp.Diagnostics.Append(diags...)
			return fmt.Errorf("invalid country lookup method: %s", lookupMethod.ValueString())
		}
	}

	updateCountryNamedLocationRequest.SetCountryLookupMethod(&method)

	var countries []string
	for _, countryVal := range plan.Country.CountriesAndRegions.Elements() {
		strVal, ok := countryVal.(types.String)
		if !ok {
			return fmt.Errorf("expected types.String but got %T", countryVal)
		}
		countries = append(countries, strVal.ValueString())
	}
	updateCountryNamedLocationRequest.SetCountriesAndRegions(countries)

	updateCountryNamedLocation := func() error {
		_, err := d.client.Identity().
			ConditionalAccess().
			NamedLocations().
			ByNamedLocationId(namedLocationId).
			Patch(ctx, updateCountryNamedLocationRequest, nil)

		if err != nil {
			log.Printf("Error during update: %v", err)
			return err
		}
		log.Println("Named location updated successfully.")
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	return backoff.Retry(updateCountryNamedLocation, reconnectBackoff)
}
