---
page_title: "googleworkspace_chrome_policy_set Resource - terraform-provider-googleworkspace"
subcategory: ""
description: |-
  Authoritative Chrome Policy Set resource that manages all Chrome policies matching a schema filter for a given target (org unit or group). Any directly-set policies in the scope that are not declared in config will be removed (inherited).
---

# googleworkspace_chrome_policy_set (Resource)

Authoritative Chrome Policy Set resource that manages all Chrome policies matching a schema filter for a given target (org unit or group). Any directly-set policies in the scope that are not declared in config will be removed (inherited).

Unlike `googleworkspace_chrome_policy`, this resource is fully authoritative over the specified `policy_schema_filter` scope. Policies within the scope that are not explicitly declared in configuration will be reset to inherit from the parent.

Chrome Policy resides under the `https://www.googleapis.com/auth/chrome.management.policy` client scope.

## Example Usage

```terraform
# Manage all chrome.users policies for an org unit
resource "googleworkspace_chrome_policy_set" "users" {
  org_unit_id          = googleworkspace_org_unit.example.id
  policy_schema_filter = "chrome.users.*"

  policy {
    schema_name = "chrome.users.MaxConnectionsPerProxy"
    schema_values = {
      maxConnectionsPerProxy = jsonencode(32)
    }
  }

  policy {
    schema_name = "chrome.users.SafeBrowsingProtectionLevel"
    schema_values = {
      safeBrowsingProtectionLevel = jsonencode("ENHANCED_PROTECTION")
    }
  }
}

# Manage app-scoped policies for a group
resource "googleworkspace_chrome_policy_set" "app_policies" {
  group_id             = "01abcdef"
  policy_schema_filter = "chrome.users.apps.*"

  policy {
    schema_name = "chrome.users.apps.InstallType"
    schema_values = {
      appInstallType = jsonencode("FORCED")
    }
    additional_target_keys = {
      app_id = "chrome:aabbccddee"
    }
  }
}

# Clear all directly-set chrome.users policies for an org unit (inherit all)
resource "googleworkspace_chrome_policy_set" "reset_users" {
  org_unit_id          = googleworkspace_org_unit.example.id
  policy_schema_filter = "chrome.users.*"
}
```

## Schema

### Required

One of `org_unit_id` or `group_id` must be set.

### Optional

- `org_unit_id` (String) The target org unit on which policies are applied. Conflicts with `group_id`. Forces a new resource if changed.
- `group_id` (String) The target group on which policies are applied. Conflicts with `org_unit_id`. Forces a new resource if changed.
- `policy_schema_filter` (String) The schema filter defining the authoritative scope, e.g. `chrome.users.*` or `chrome.users.apps.*`. Supports wildcards at the leaf level. If omitted, inferred from the policy blocks (all must share the same `schema_name`). Forces a new resource if changed.
- `policy` (Set) Set of policies to enforce. If empty, all directly-set policies in the scope are removed (inherited).

### Nested Schema for `policy`

#### Required

- `schema_name` (String) The full qualified name of the policy schema, e.g. `chrome.users.MaxConnectionsPerProxy`.
- `schema_values` (Map of String) JSON-encoded map of key/value pairs for the policy. Each value must be a valid JSON literal (string, number, boolean, array, or object). Example: `{ maxConnectionsPerProxy = jsonencode(32) }`.

#### Optional

- `additional_target_keys` (Map of String) Additional target keys for app-scoped policies, e.g. `{ app_id = "chrome:aabbcc" }`. Required for schemas that declare additional target key names; must be omitted for schemas that do not.

### Read-Only

- `id` (String) The ID of this resource, in the format `{orgunits|groups}/{targetId}/{schemaFilter}`.

## Import

Import is supported using the following syntax:

```shell
# Org unit target:
terraform import googleworkspace_chrome_policy_set.example orgunits/03ph8a2z1kajcan/chrome.users.*

# Group target:
terraform import googleworkspace_chrome_policy_set.app_policies groups/01abcdef/chrome.users.apps.*
```
