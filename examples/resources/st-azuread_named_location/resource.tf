resource "st-azuread_named_location" "ip" {
  display_name = "named-location-ip"

  ip {
    ip_ranges     = ["1.1.1.1/32"]
    trusted       = false
    force_destroy = false
  }
}

resource "st-azuread_named_location" "country" {
  display_name = "named-location-country"

  country {
    countries_and_regions                 = ["MY"]
    include_unknown_countries_and_regions = false
    country_lookup_method                 = "authenticatorAppGps"
  }
}
