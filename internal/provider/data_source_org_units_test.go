package googleworkspace

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccDataSourceOrgUnits(t *testing.T) {
	t.Parallel()

	ouName := fmt.Sprintf("tf-test-%s", acctest.RandString(10))

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceOrgUnits(ouName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.googleworkspace_org_units.all",
						"org_units.#"),
				),
			},
		},
	})
}

func testAccDataSourceOrgUnits(ouName string) string {
	return fmt.Sprintf(`
resource "googleworkspace_org_unit" "my-org-unit" {
  name = "%s"
  parent_org_unit_path = "/"
}

data "googleworkspace_org_units" "all" {
  depends_on = [googleworkspace_org_unit.my-org-unit]
}
`, ouName)
}
