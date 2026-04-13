resource "googleworkspace_chrome_group_policy" "example" {
  group_id = "01abcdef"

  policies {
    schema_name = "chrome.users.MaxConnectionsPerProxy"
    schema_values = {
      maxConnectionsPerProxy = jsonencode(32)
    }
  }
}

# With additional target keys (e.g. app-scoped policies)
resource "googleworkspace_chrome_group_policy" "app_policy" {
  group_id = "01abcdef"

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
