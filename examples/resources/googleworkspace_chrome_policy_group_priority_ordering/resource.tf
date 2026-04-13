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
