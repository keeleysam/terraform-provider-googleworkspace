---
page_title: "googleworkspace_chrome_policy_group_priority_ordering Data Source - terraform-provider-googleworkspace"
subcategory: ""
description: |-
  Reads the group priority ordering for a Chrome policy. When multiple groups have the same policy configured, the priority ordering determines which group's policy takes precedence.
---

# googleworkspace_chrome_policy_group_priority_ordering (Data Source)

Reads the group priority ordering for a Chrome policy. When multiple groups have the same policy configured, the priority ordering determines which group's policy takes precedence.

If the policy is not configured on any groups, `group_ids` will be empty and a warning will be emitted.

## Example Usage

```terraform
data "googleworkspace_chrome_policy_group_priority_ordering" "example" {
  policy_schema = "chrome.users.MaxConnectionsPerProxy"
}

output "group_priority_order" {
  value = data.googleworkspace_chrome_policy_group_priority_ordering.example.group_ids
}

# For app-scoped policies, specify additional_target_keys:
data "googleworkspace_chrome_policy_group_priority_ordering" "app_example" {
  policy_schema = "chrome.users.apps.InstallType"

  additional_target_keys = {
    app_id = "chrome:aabbccdd"
  }
}
```

## Schema

### Required

- `policy_schema` (String) The full qualified name of the policy schema, e.g. `chrome.users.MaxConnectionsPerProxy`.

### Optional

- `additional_target_keys` (Map of String) Additional target keys for app-scoped policies, e.g. `{app_id = "chrome:aabbcc"}`. Omit for non-app-scoped policies.

### Read-Only

- `group_ids` (List of String) The group IDs in priority order (highest priority first).
- `id` (String) The ID of this resource.
