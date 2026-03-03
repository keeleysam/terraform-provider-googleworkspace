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
