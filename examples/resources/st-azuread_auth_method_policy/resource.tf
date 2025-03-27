resource "st-azuread_auth_method_policy" "example" {
  state              = "enabled"
  type               = "Voice"
  excluded_group_ids = ["xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"]
}
