data "st-azuread_auth_strength_policy" "example" {
  names = ["Passwordless MFA", "Phishing-resistant MFA"]
}
