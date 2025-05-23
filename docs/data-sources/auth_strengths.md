---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "st-azuread_auth_strengths Data Source - st-azuread"
subcategory: ""
description: |-
  This data source provides the authentication strength policies based on the list of ids or names of the policies. Will return all policies if input is empty.
---

# st-azuread_auth_strengths (Data Source)

This data source provides the authentication strength policies based on the list of ids or names of the policies. Will return all policies if input is empty.

## Example Usage

```terraform
data "st-azuread_auth_strengths" "example" {
  names = ["Passwordless MFA", "Phishing-resistant MFA"]
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `ids` (List of String) The IDs of the authentication strength policy.
- `names` (List of String) The names of the authentication strength policy.
