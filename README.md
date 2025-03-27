Terraform Custom Provider for AzureAD
======================================

This Terraform custom provider is designed for own use case scenario.

Supported Versions
------------------

| Terraform version | minimum provider version |maxmimum provider version
| ---- | ---- | ----|
| >= 1.8.x	| 0.1.1	| latest |

Requirements
------------

-	[Terraform](https://www.terraform.io/downloads.html) 1.8.x
-	[Go](https://golang.org/doc/install) 1.19 (to build the provider plugin)

Local Installation
------------------

1. Run make file `make install-local-custom-provider` to install the provider under ~/.terraform.d/plugins.

2. The provider source should be change to the path that configured in the *Makefile*:

    ```
    terraform {
      required_providers {
        st-byteplus = {
          source = "example.local/myklst/st-azuread"
        }
      }
    }

    provider "st-byteplus" {}
    ```

Why Custom Provider
-------------------

This custom provider exists due to some of the resources and data sources in the
official AzureAD Terraform provider may not fulfill the requirements of some
scenario. The reason behind every resources and data sources are stated as below:

### Resources

- **st-azuread_auth_method_policy**

  - Official AzureAD Terraform provider does not have the ability to manage the
    authentication method policies.

### Data Sources

- **st-azuread_auth_strength_policy**

  - Official AzureAD Terraform provider does not have the ability to obtain the
    id or name of the authentication strength policy on Microsoft Entra ID via
    id or name.

References
----------

- Website: https://www.terraform.io
- Terraform Plugin Framework: https://developer.hashicorp.com/terraform/tutorials/providers-plugin-framework
- Byteplus official Terraform provider: https://github.com/hashicorp/terraform-provider-azuread
