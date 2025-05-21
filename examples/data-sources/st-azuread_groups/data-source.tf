data "azuread_groups" "user_groups" {
  display_names  = ["Example"]
  ignore_missing = true
}
