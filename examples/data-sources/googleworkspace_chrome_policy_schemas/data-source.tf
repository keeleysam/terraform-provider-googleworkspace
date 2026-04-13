data "googleworkspace_chrome_policy_schemas" "example" {
  filter = "chrome.printers"
}

output "num_schemas" {
  value = length(data.googleworkspace_chrome_policy_schemas.example.policy_schemas)
}
