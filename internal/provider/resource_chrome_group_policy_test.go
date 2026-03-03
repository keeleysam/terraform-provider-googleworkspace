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

// TestAccResourceChromeGroupPolicy_groupIDChange verifies create-before-delete ordering
// when group_id is changed: the policy must exist on the new group before being removed
// from the old group, so users experience no gap in enforcement.
func TestAccResourceChromeGroupPolicy_groupIDChange(t *testing.T) {
	t.Parallel()

	groupNameA := fmt.Sprintf("tf-test-group-a-%s", acctest.RandString(6))
	groupNameB := fmt.Sprintf("tf-test-group-b-%s", acctest.RandString(6))

	// Verify the policy was deleted from group A after the group_id change.
	checkRemovedFromGroupA := func(s *terraform.State) error {
		client, err := googleworkspaceTestClient()
		if err != nil {
			return err
		}

		rs, ok := s.RootModule().Resources["googleworkspace_group.test_a"]
		if !ok {
			return fmt.Errorf("can't find resource: googleworkspace_group.test_a")
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("group A ID not set")
		}

		chromePolicyService, diags := client.NewChromePolicyService()
		if diags.HasError() {
			return errors.New(diags[0].Summary)
		}
		chromePoliciesService, diags := GetChromePoliciesService(chromePolicyService)
		if diags.HasError() {
			return errors.New(diags[0].Summary)
		}

		groupID := strings.TrimPrefix(rs.Primary.ID, "id:")
		policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
			TargetResource: "groups/" + groupID,
		}

		resp, err := chromePoliciesService.Resolve(
			fmt.Sprintf("customers/%s", client.Customer),
			&chromepolicy.GoogleChromePolicyVersionsV1ResolveRequest{
				PolicySchemaFilter: "chrome.users.MaxConnectionsPerProxy",
				PolicyTargetKey:    policyTargetKey,
			},
		).Do()
		if err != nil {
			return err
		}

		// The policy must not be explicitly set on group A anymore.
		for _, rp := range resp.ResolvedPolicies {
			if rp.SourceKey != nil && rp.SourceKey.TargetResource == "groups/"+groupID {
				return fmt.Errorf("policy still explicitly set on group A after group_id change")
			}
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				// Step 1: assign policy to group A.
				Config: testAccResourceChromeGroupPolicy_groupIDChangeStep1(groupNameA, groupNameB, 7),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.#", "1"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_name", "chrome.users.MaxConnectionsPerProxy"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_values.maxConnectionsPerProxy", "7"),
				),
			},
			{
				// Step 2: move the same policy to group B; verify it exists on B and is gone from A.
				Config: testAccResourceChromeGroupPolicy_groupIDChangeStep2(groupNameA, groupNameB, 7),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.#", "1"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_name", "chrome.users.MaxConnectionsPerProxy"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_values.maxConnectionsPerProxy", "7"),
					checkRemovedFromGroupA,
				),
			},
		},
	})
}

func testAccResourceChromeGroupPolicy_groupIDChangeStep1(groupNameA, groupNameB string, conns int) string {
	return fmt.Sprintf(`
resource "googleworkspace_group" "test_a" {
  email       = "%s@example.com"
  name        = "%s"
  description = "Test group A"
}

resource "googleworkspace_group" "test_b" {
  email       = "%s@example.com"
  name        = "%s"
  description = "Test group B"
}

resource "googleworkspace_chrome_group_policy" "test" {
  group_id = googleworkspace_group.test_a.id
  policies {
    schema_name = "chrome.users.MaxConnectionsPerProxy"
    schema_values = {
      maxConnectionsPerProxy = jsonencode(%d)
    }
  }
}
`, groupNameA, groupNameA, groupNameB, groupNameB, conns)
}

func testAccResourceChromeGroupPolicy_groupIDChangeStep2(groupNameA, groupNameB string, conns int) string {
	return fmt.Sprintf(`
resource "googleworkspace_group" "test_a" {
  email       = "%s@example.com"
  name        = "%s"
  description = "Test group A"
}

resource "googleworkspace_group" "test_b" {
  email       = "%s@example.com"
  name        = "%s"
  description = "Test group B"
}

resource "googleworkspace_chrome_group_policy" "test" {
  group_id = googleworkspace_group.test_b.id
  policies {
    schema_name = "chrome.users.MaxConnectionsPerProxy"
    schema_values = {
      maxConnectionsPerProxy = jsonencode(%d)
    }
  }
}
`, groupNameA, groupNameA, groupNameB, groupNameB, conns)
}

// Test batching scenario: No additional_target_keys with multiple policies
// Expected: Individual API call per policy
func TestAccResourceChromeGroupPolicy_batchingNoPolicyKeys(t *testing.T) {
	t.Parallel()

	groupName := fmt.Sprintf("tf-test-group-%s", acctest.RandString(6))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceChromeGroupPolicy_multiplePoliciesNoKeys(groupName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.#", "2"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_name", "chrome.users.MaxConnectionsPerProxy"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_values.maxConnectionsPerProxy", "8"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.1.schema_name", "chrome.users.RestrictSigninToPattern"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.1.schema_values.restrictSigninToPattern", encode(".*@example.com")),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "additional_target_keys.#", "0"),
				),
			},
		},
	})
}

// Test batching scenario: Multiple additional_target_keys with same target_key
// Expected: One batched API call per unique target_value, each containing all policies
func TestAccResourceChromeGroupPolicy_batchingSameTargetKey(t *testing.T) {
	t.Parallel()

	groupName := fmt.Sprintf("tf-test-group-%s", acctest.RandString(6))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceChromeGroupPolicy_multipleAppsSameTargetKey(groupName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.#", "1"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_name", "chrome.users.apps.InstallType"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "additional_target_keys.#", "2"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "additional_target_keys.0.target_key", "app_id"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "additional_target_keys.0.target_value", "chrome:glnpjglilkicbckjpbgcfkogebgllemb"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "additional_target_keys.1.target_key", "app_id"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "additional_target_keys.1.target_value", "chrome:aapbdbdomjkkjkaonfhkkikfgjllcleb"),
				),
			},
		},
	})
}

// Test batching scenario: Multiple policies with multiple apps (same target_key)
// Expected: One batched API call per app, each containing all policies
func TestAccResourceChromeGroupPolicy_batchingMultiplePoliciesMultipleApps(t *testing.T) {
	t.Parallel()

	groupName := fmt.Sprintf("tf-test-group-%s", acctest.RandString(6))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceChromeGroupPolicy_multiplePoliciesMultipleApps(groupName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.#", "2"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.0.schema_name", "chrome.users.apps.InstallType"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "policies.1.schema_name", "chrome.users.apps.PinningPolicy"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "additional_target_keys.#", "2"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "additional_target_keys.0.target_key", "app_id"),
					resource.TestCheckResourceAttr("googleworkspace_chrome_group_policy.test", "additional_target_keys.1.target_key", "app_id"),
				),
			},
		},
	})
}

func testAccResourceChromeGroupPolicy_multiplePoliciesNoKeys(groupName string) string {
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
      maxConnectionsPerProxy = jsonencode(8)
    }
  }
  policies {
    schema_name = "chrome.users.RestrictSigninToPattern"
    schema_values = {
      restrictSigninToPattern = jsonencode(".*@example.com")
    }
  }
}
`, groupName, groupName)
}

func testAccResourceChromeGroupPolicy_multipleAppsSameTargetKey(groupName string) string {
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
    target_value = "chrome:glnpjglilkicbckjpbgcfkogebgllemb"
  }
  additional_target_keys {
    target_key   = "app_id"
    target_value = "chrome:aapbdbdomjkkjkaonfhkkikfgjllcleb"
  }
  policies {
    schema_name = "chrome.users.apps.InstallType"
    schema_values = {
      appInstallType = jsonencode("ALLOWED")
    }
  }
}
`, groupName, groupName)
}

func testAccResourceChromeGroupPolicy_multiplePoliciesMultipleApps(groupName string) string {
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
    target_value = "chrome:glnpjglilkicbckjpbgcfkogebgllemb"
  }
  additional_target_keys {
    target_key   = "app_id"
    target_value = "chrome:aapbdbdomjkkjkaonfhkkikfgjllcleb"
  }
  policies {
    schema_name = "chrome.users.apps.InstallType"
    schema_values = {
      appInstallType = jsonencode("FORCED")
    }
  }
  policies {
    schema_name = "chrome.users.apps.PinningPolicy"
    schema_values = {
      extensionPinningPolicy = jsonencode("FORCE_PINNED")
    }
  }
}
`, groupName, groupName)
}
