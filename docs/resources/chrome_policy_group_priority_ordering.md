---
page_title: "googleworkspace_chrome_policy_group_priority_ordering Resource - terraform-provider-googleworkspace"
subcategory: ""
description: |-
  Manages the group priority ordering for a Chrome policy. When multiple groups have the same policy configured, the priority ordering determines which group's policy takes precedence. This resource is authoritative over the complete ordering.
---

# googleworkspace_chrome_policy_group_priority_ordering (Resource)

Manages the group priority ordering for a Chrome policy. When multiple groups have the same policy configured, the priority ordering determines which group's policy takes precedence. This resource is authoritative over the complete ordering.

## Example Usage

```terraform
resource "googleworkspace_chrome_policy_group_priority_ordering" "example" {
  policy_schema = "chrome.users.MaxConnectionsPerProxy"

  group_ids = [
    "group-id-highest-priority",
    "group-id-second-priority",
    "group-id-lowest-priority",
  ]
}

# For app-scoped policies, specify additional_target_keys:
resource "googleworkspace_chrome_policy_group_priority_ordering" "app_example" {
  policy_schema = "chrome.users.apps.InstallType"

  additional_target_keys = {
    app_id = "chrome:aabbccdd"
  }

  group_ids = [
    "group-id-highest-priority",
    "group-id-lowest-priority",
  ]
}
```

## Schema

### Required

- `group_ids` (List of String) The group IDs in desired priority order (highest priority first).
- `policy_schema` (String) The full qualified name of the policy schema, e.g. `chrome.users.MaxConnectionsPerProxy`.

### Optional

- `additional_target_keys` (Map of String) Additional target keys for app-scoped policies, e.g. `{app_id = "chrome:aabbcc"}`. Omit for non-app-scoped policies.

### Read-Only

- `id` (String) The ID of this resource.

## Import

Import is supported using the following syntax:

```shell
# Normal policy:
terraform import googleworkspace_chrome_policy_group_priority_ordering.example chrome.users.MaxConnectionsPerProxy

# App-scoped policy:
terraform import googleworkspace_chrome_policy_group_priority_ordering.app_example chrome.users.apps.InstallType/app_id=chrome:aabbccdd
```
