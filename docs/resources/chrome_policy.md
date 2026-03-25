---
page_title: "googleworkspace_chrome_policy Resource - terraform-provider-googleworkspace"
subcategory: ""
description: |-
  Chrome Policy resource in the Terraform Googleworkspace provider. Chrome Policy Schema resides under the https://www.googleapis.com/auth/chrome.management.policy client scope.
---

# googleworkspace_chrome_policy (Resource)

Chrome Policy resource in the Terraform Googleworkspace provider. Applies Chrome policies to a target org unit. Chrome Policy Schema resides under the `https://www.googleapis.com/auth/chrome.management.policy` client scope.

## Example Usage

```terraform
resource "googleworkspace_org_unit" "example" {
  name                 = "example"
  parent_org_unit_path = "/"
}

resource "googleworkspace_chrome_policy" "example" {
  org_unit_id = googleworkspace_org_unit.example.id

  policies {
    schema_name = "chrome.users.MaxConnectionsPerProxy"
    schema_values = {
      maxConnectionsPerProxy = jsonencode(34)
    }
  }
}

# With additional target keys (e.g. app-scoped policies)
resource "googleworkspace_chrome_policy" "app_policy" {
  org_unit_id = googleworkspace_org_unit.example.id

  additional_target_keys {
    target_key   = "app_id"
    target_value = "chrome:aabbccddee"
  }

  policies {
    schema_name = "chrome.users.apps.InstallType"
    schema_values = {
      appInstallType = jsonencode("FORCED")
    }
  }
}
```

## Schema

### Required

- `org_unit_id` (String) The target org unit on which this policy is applied. Forces a new resource if changed.
- `policies` (Block List, Min: 1) Policies to set for the org unit (see [below for nested schema](#nestedblock--policies)).

### Optional

- `additional_target_keys` (Block List) Additional target keys for policies (see [below for nested schema](#nestedblock--additional_target_keys)).

### Read-Only

- `id` (String) The ID of this resource.

<a id="nestedblock--policies"></a>
### Nested Schema for `policies`

#### Required

- `schema_name` (String) The full qualified name of the policy schema.
- `schema_values` (Map of String) JSON encoded map that represents key/value pairs that correspond to the given schema.

<a id="nestedblock--additional_target_keys"></a>
### Nested Schema for `additional_target_keys`

#### Required

- `target_key` (String) The target key name.
- `target_value` (String) The target key value.


