---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "st-azuread_auth_method_policy Resource - st-azuread"
subcategory: ""
description: |-
  Manages an authentication method policy on Microsoft Entra ID. Currently,  QR code and Hardware OATH Tokens are not supported in Microsoft Graph API
---

# st-azuread_auth_method_policy (Resource)

Manages an authentication method policy on Microsoft Entra ID. Currently,  QR code and Hardware OATH Tokens are not supported in Microsoft Graph API

## Example Usage

```terraform
resource "st-azuread_auth_method_policy" "example" {
  state              = "enabled"
  type               = "Voice"
  excluded_group_ids = ["xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `state` (String) Whether the authentication method policy is enabled in the tenant. Possible values are `enabled` or `disabled`.
- `type` (String) The type of the authentication method policy. Possible values are `Email`, `Fido2`, `MicrosoftAuthenticator`, `Voice`, `Sms`, `SoftwareOath``TemporaryAccessPass`, `X509Certificate`

### Optional

- `excluded_group_ids` (List of String) A list of group IDs to exclude from the authentication method policy.
