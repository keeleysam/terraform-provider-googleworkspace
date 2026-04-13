package googleworkspace

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	directory "google.golang.org/api/admin/directory/v1"
)

func dataSourceOrgUnits() *schema.Resource {
	dsOrgUnitSchema := datasourceSchemaFromResourceSchema(resourceOrgUnit().Schema)

	return &schema.Resource{
		Description: "Org Units data source in the Terraform Googleworkspace provider. Org Units resides " +
			"under the `https://www.googleapis.com/auth/admin.directory.orgunit` client scope.",

		ReadContext: dataSourceOrgUnitsRead,

		Schema: map[string]*schema.Schema{
			"org_unit_path": {
				Description: "The full path to the organizational unit or its unique ID. " +
					"Returns the children of the specified organizational unit.",
				Type:     schema.TypeString,
				Optional: true,
			},
			"type": {
				Description: "Whether to return all sub-organizations or just immediate children. " +
					"Valid values are `ALL`, `CHILDREN`, and `ALL_INCLUDING_PARENT`. " +
					"Defaults to `CHILDREN`.",
				Type:     schema.TypeString,
				Optional: true,
				ValidateFunc: validation.StringInSlice([]string{
					"ALL", "CHILDREN", "ALL_INCLUDING_PARENT",
				}, false),
			},
			"org_units": {
				Description: "A list of Org Unit resources.",
				Type:        schema.TypeList,
				Computed:    true,
				Elem: &schema.Resource{
					Schema: dsOrgUnitSchema,
				},
			},
		},
	}
}

func dataSourceOrgUnitsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)

	directoryService, diags := client.NewDirectoryService()
	if diags.HasError() {
		return diags
	}

	orgUnitsService, diags := GetOrgUnitsService(directoryService)
	if diags.HasError() {
		return diags
	}

	listCall := orgUnitsService.List(client.Customer)

	orgUnitPath := d.Get("org_unit_path").(string)
	if orgUnitPath != "" {
		listCall = listCall.OrgUnitPath(orgUnitPath)
	}

	listType := d.Get("type").(string)
	if listType != "" {
		listCall = listCall.Type(listType)
	}

	resp, err := listCall.Do()
	if err != nil {
		return handleNotFoundError(err, d, "org_units")
	}

	if err := d.Set("org_units", flattenOrgUnits(resp.OrganizationUnits)); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(fmt.Sprintf("org_units/%s/%s", orgUnitPath, listType))

	return diags
}

func flattenOrgUnits(orgUnits []*directory.OrgUnit) interface{} {
	var result []interface{}

	for _, orgUnit := range orgUnits {
		result = append(result, flattenOrgUnit(orgUnit))
	}

	return result
}

func flattenOrgUnit(orgUnit *directory.OrgUnit) interface{} {
	result := map[string]interface{}{}
	result["name"] = orgUnit.Name
	result["description"] = orgUnit.Description
	result["etag"] = orgUnit.Etag
	result["block_inheritance"] = orgUnit.BlockInheritance
	result["org_unit_id"] = orgUnit.OrgUnitId
	result["org_unit_path"] = orgUnit.OrgUnitPath
	result["parent_org_unit_id"] = orgUnit.ParentOrgUnitId
	result["parent_org_unit_path"] = orgUnit.ParentOrgUnitPath

	return result
}
