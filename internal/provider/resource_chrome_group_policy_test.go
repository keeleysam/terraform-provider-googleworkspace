package googleworkspace

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"google.golang.org/api/chromepolicy/v1"
)

// (idAttributeForKind tests moved to chrome_policy_common_test.go)

func TestAccResourceChromeGroupPolicy_basic(t *testing.T) {
	t.Parallel()

	groupName := fmt.Sprintf("tf-test-group-%s", acctest.RandString(6))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceChromeGroupPolicy_basic(groupName, 7),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.#", "1"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_name", "chrome.users.MaxConnectionsPerProxy"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_values.maxConnectionsPerProxy", "7"),
				),
			},
		},
	})
}

func TestAccResourceChromeGroupPolicy_typeMessage(t *testing.T) {
	t.Parallel()

	groupName := fmt.Sprintf("tf-test-group-%s", acctest.RandString(6))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceChromeGroupPolicy_typeMessage(groupName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.#", "1"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_name", "chrome.users.ManagedBookmarksSetting"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_values.managedBookmarks", "{\"toplevelName\":\"Stuff\"}"),
				),
			},
		},
	})
}

func TestAccResourceChromeGroupPolicy_additionalTargetKey(t *testing.T) {
	t.Parallel()

	groupName := fmt.Sprintf("tf-test-group-%s", acctest.RandString(6))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceChromeGroupPolicy_additionalTargetKey(groupName, "chrome:glnpjglilkicbckjpbgcfkogebgllemb", "ALLOWED"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.#", "1"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_name", "chrome.users.apps.InstallType"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_values.appInstallType", encode("ALLOWED")),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "additional_target_keys.#", "1"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "additional_target_keys.0.target_key", "app_id"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "additional_target_keys.0.target_value", "chrome:glnpjglilkicbckjpbgcfkogebgllemb"),
				),
			},
		},
	})
}

func TestAccResourceChromeGroupPolicy_update(t *testing.T) {
	t.Parallel()

	groupName := fmt.Sprintf("tf-test-group-%s", acctest.RandString(6))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceChromeGroupPolicy_basic(groupName, 5),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.#", "1"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_name", "chrome.users.MaxConnectionsPerProxy"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_values.maxConnectionsPerProxy", "5"),
				),
			},
			{
				Config: testAccResourceChromeGroupPolicy_basic(groupName, 9),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.#", "1"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_name", "chrome.users.MaxConnectionsPerProxy"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_values.maxConnectionsPerProxy", "9"),
				),
			},
		},
	})
}

func TestAccResourceChromeGroupPolicy_multiple(t *testing.T) {
	t.Parallel()

	groupName := fmt.Sprintf("tf-test-group-%s", acctest.RandString(6))

	// ensures previously set field was reset/removed
	testCheck := func(s *terraform.State) error {
		client, err := googleworkspaceTestClient()
		if err != nil {
			return err
		}

		rs, ok := s.RootModule().Resources["googleworkspace_group.test"]
		if !ok {
			return fmt.Errorf("Can't find group resource: googleworkspace_group.test")
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("group ID not set")
		}

		chromePolicyService, diags := client.NewChromePolicyService()
		if diags.HasError() {
			return errors.New(diags[0].Summary)
		}

		chromePoliciesService, diags := GetChromePoliciesService(chromePolicyService)
		if diags.HasError() {
			return errors.New(diags[0].Summary)
		}

		policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
			TargetResource: "groups/" + strings.TrimPrefix(rs.Primary.ID, "id:"),
		}

		resp, err := chromePoliciesService.Resolve(fmt.Sprintf("customers/%s", client.Customer), &chromepolicy.GoogleChromePolicyVersionsV1ResolveRequest{
			PolicySchemaFilter: "chrome.users.MaxConnectionsPerProxy",
			PolicyTargetKey:    policyTargetKey,
		}).Do()
		if err != nil {
			return err
		}
		if len(resp.ResolvedPolicies) > 0 {
			return fmt.Errorf("Expected policy to be reset")
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceChromeGroupPolicy_multiple(groupName, 3, ".*@example"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.#", "2"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_name", "chrome.users.RestrictSigninToPattern"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_values.restrictSigninToPattern", encode(".*@example")),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.1.schema_name", "chrome.users.MaxConnectionsPerProxy"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.1.schema_values.maxConnectionsPerProxy", "3"),
				),
			},
			{
				Config: testAccResourceChromeGroupPolicy_multipleRearranged(groupName, 4, ".*@example.com"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.#", "2"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_name", "chrome.users.MaxConnectionsPerProxy"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_values.maxConnectionsPerProxy", "4"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.1.schema_name", "chrome.users.RestrictSigninToPattern"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.1.schema_values.restrictSigninToPattern", encode(".*@example.com")),
				),
			},
			{
				Config: testAccResourceChromeGroupPolicy_multipleDifferent(groupName, true, ".*@example.com"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.#", "2"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_name", "chrome.users.OnlineRevocationChecks"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_values.enableOnlineRevocationChecks", "true"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.1.schema_name", "chrome.users.RestrictSigninToPattern"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.1.schema_values.restrictSigninToPattern", encode(".*@example.com")),
					testCheck,
				),
			},
			{
				Config: testAccResourceChromeGroupPolicy_multipleValueTypes(groupName, true, "POLICY_MODE_RECOMMENDED"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.#", "1"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_name", "chrome.users.DomainReliabilityAllowed"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_values.domainReliabilityAllowed", "true"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_values.domainReliabilityAllowedSettingGroupPolicyMode", encode("POLICY_MODE_RECOMMENDED")),
					testCheck,
				),
			},
		},
	})
}

func testAccResourceChromeGroupPolicy_basic(groupName string, conns int) string {
	return fmt.Sprintf(`
resource "googleworkspace_group" "test" {
  email       = "%s@example.com"
  name        = "%s"
  description = "Test group"
}

resource "googleworkspace_chrome_group_policy" "test" {
  group_id = googleworkspace_group.test.id
  policies {
    schema_name = "chrome.users.MaxConnectionsPerProxy"
    schema_values = {
      maxConnectionsPerProxy = jsonencode(%d)
    }
  }
}
`, groupName, groupName, conns)
}

func testAccResourceChromeGroupPolicy_typeMessage(groupName string) string {
	return fmt.Sprintf(`
resource "googleworkspace_group" "test" {
  email       = "%s@example.com"
  name        = "%s"
  description = "Test group"
}

resource "googleworkspace_chrome_group_policy" "test" {
  group_id = googleworkspace_group.test.id
  policies {
    schema_name = "chrome.users.ManagedBookmarksSetting"
    schema_values = {
      managedBookmarks = "{\"toplevelName\":\"Stuff\"}"
    }
  }
}
`, groupName, groupName)
}

func testAccResourceChromeGroupPolicy_additionalTargetKey(groupName, appID, installType string) string {
	return fmt.Sprintf(`
resource "googleworkspace_group" "test" {
  email       = "%s@example.com"
  name        = "%s"
  description = "Test group"
}

resource "googleworkspace_chrome_group_policy" "test" {
  group_id = googleworkspace_group.test.id
  additional_target_keys {
    target_key   = "app_id"
    target_value = "%s"
  }
  policies {
    schema_name = "chrome.users.apps.InstallType"
    schema_values = {
      appInstallType = jsonencode("%s")
    }
  }
}
`, groupName, groupName, appID, installType)
}

func testAccResourceChromeGroupPolicy_multiple(groupName string, conns int, pattern string) string {
	return fmt.Sprintf(`
resource "googleworkspace_group" "test" {
  email       = "%s@example.com"
  name        = "%s"
  description = "Test group"
}

resource "googleworkspace_chrome_group_policy" "test" {
  group_id = googleworkspace_group.test.id
  policies {
    schema_name = "chrome.users.RestrictSigninToPattern"
    schema_values = {
      restrictSigninToPattern = jsonencode("%s")
    }
  }
  policies {
    schema_name = "chrome.users.MaxConnectionsPerProxy"
    schema_values = {
      maxConnectionsPerProxy = jsonencode(%d)
    }
  }
}
`, groupName, groupName, pattern, conns)
}

func testAccResourceChromeGroupPolicy_multipleRearranged(groupName string, conns int, pattern string) string {
	return fmt.Sprintf(`
resource "googleworkspace_group" "test" {
  email       = "%s@example.com"
  name        = "%s"
  description = "Test group"
}

resource "googleworkspace_chrome_group_policy" "test" {
  group_id = googleworkspace_group.test.id
  policies {
    schema_name = "chrome.users.MaxConnectionsPerProxy"
    schema_values = {
      maxConnectionsPerProxy = jsonencode(%d)
    }
  }
  policies {
    schema_name = "chrome.users.RestrictSigninToPattern"
    schema_values = {
      restrictSigninToPattern = jsonencode("%s")
    }
  }
}
`, groupName, groupName, conns, pattern)
}

func testAccResourceChromeGroupPolicy_multipleDifferent(groupName string, enabled bool, pattern string) string {
	return fmt.Sprintf(`
resource "googleworkspace_group" "test" {
  email       = "%s@example.com"
  name        = "%s"
  description = "Test group"
}

resource "googleworkspace_chrome_group_policy" "test" {
  group_id = googleworkspace_group.test.id
  policies {
    schema_name = "chrome.users.OnlineRevocationChecks"
    schema_values = {
      enableOnlineRevocationChecks = jsonencode(%t)
    }
  }
  policies {
    schema_name = "chrome.users.RestrictSigninToPattern"
    schema_values = {
      restrictSigninToPattern = jsonencode("%s")
    }
  }
}
`, groupName, groupName, enabled, pattern)
}

func testAccResourceChromeGroupPolicy_multipleValueTypes(groupName string, enabled bool, policyMode string) string {
	return fmt.Sprintf(`
resource "googleworkspace_group" "test" {
  email       = "%s@example.com"
  name        = "%s"
  description = "Test group"
}

resource "googleworkspace_chrome_group_policy" "test" {
  group_id = googleworkspace_group.test.id
  policies {
    schema_name = "chrome.users.DomainReliabilityAllowed"
    schema_values = {
      domainReliabilityAllowed                       = jsonencode(%t)
      domainReliabilityAllowedSettingGroupPolicyMode = jsonencode("%s")
    }
  }
}
`, groupName, groupName, enabled, policyMode)
}
