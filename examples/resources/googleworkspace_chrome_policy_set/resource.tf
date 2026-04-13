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
