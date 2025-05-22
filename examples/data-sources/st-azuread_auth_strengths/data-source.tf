data "st-azuread_auth_strengths" "ids" {
}

data "st-azuread_auth_strengths" "ids" {
  ids = ["xxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxx4"]
}

data "st-azuread_auth_strengths" "names" {
  names = ["Passwordless MFA", "Phishing-resistant MFA"]
}
