resource "googleworkspace_chrome_policy_file" "wallpaper" {
  policy_field = "chrome.users.Wallpaper.wallpaperImage"
  file_path    = "${path.module}/wallpaper.jpg"
}

resource "googleworkspace_chrome_policy" "wallpaper" {
  org_unit_id = googleworkspace_org_unit.example.id
  policies {
    schema_name = "chrome.users.Wallpaper"
    schema_values = {
      wallpaperImage = jsonencode(googleworkspace_chrome_policy_file.wallpaper.download_uri)
    }
  }
}
