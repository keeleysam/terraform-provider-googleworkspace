data "googleworkspace_org_units" "all" {
  type = "ALL"
}

output "num_org_units" {
  value = length(data.googleworkspace_org_units.all.org_units)
}
