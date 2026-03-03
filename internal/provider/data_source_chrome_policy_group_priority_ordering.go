package googleworkspace

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"google.golang.org/api/chromepolicy/v1"
	"google.golang.org/api/googleapi"
)

func dataSourceChromePolicyGroupPriorityOrdering() *schema.Resource {
	return &schema.Resource{
		Description: "Reads the group priority ordering for a Chrome policy. " +
			"When multiple groups have the same policy configured, the priority " +
			"ordering determines which group's policy takes precedence.",

		ReadContext: dataSourceChromePolicyGroupPriorityOrderingRead,

		Schema: map[string]*schema.Schema{
			"policy_schema": {
				Description: "The full qualified name of the policy schema, e.g. `chrome.users.MaxConnectionsPerProxy`.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"additional_target_keys": {
				Description: "Additional target keys for app-scoped policies, " +
					"e.g. `{app_id = \"chrome:aabbcc\"}`. Omit for non-app-scoped policies.",
				Type:     schema.TypeMap,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"group_ids": {
				Description: "The group IDs in priority order (highest priority first).",
				Type:        schema.TypeList,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func dataSourceChromePolicyGroupPriorityOrderingRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	groupsService, diags := GetChromePoliciesGroupsService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	policySchema := d.Get("policy_schema").(string)

	req := &chromepolicy.GoogleChromePolicyVersionsV1ListGroupPriorityOrderingRequest{
		PolicySchema:    policySchema,
		PolicyTargetKey: buildGroupPriorityTargetKey(d),
	}

	var resp *chromepolicy.GoogleChromePolicyVersionsV1ListGroupPriorityOrderingResponse
	err := retryTimeDuration(ctx, time.Minute, func() error {
		var retryErr error
		resp, retryErr = groupsService.ListGroupPriorityOrdering(
			fmt.Sprintf("customers/%s", client.Customer), req,
		).Do()
		return retryErr
	})
	if err != nil {
		if isGroupPriorityNotConfiguredError(err) {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Warning,
				Summary:  "The specified policy is not configured on any groups; group_ids is empty.",
			})
			d.SetId(groupPriorityOrderingID(policySchema, d))
			if err := d.Set("group_ids", []string{}); err != nil {
				return diag.FromErr(err)
			}
			return diags
		}
		return diag.FromErr(err)
	}

	d.SetId(groupPriorityOrderingID(policySchema, d))

	if err := d.Set("group_ids", resp.GroupIds); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

// isGroupPriorityNotConfiguredError checks whether the error is the specific
// 400 INVALID_ARGUMENT indicating the policy is not configured on any groups.
func isGroupPriorityNotConfiguredError(err error) bool {
	if gerr, ok := err.(*googleapi.Error); ok {
		return gerr.Code == 400 &&
			strings.Contains(gerr.Message, "not configured on any Groups")
	}
	return false
}

func buildGroupPriorityTargetKey(d *schema.ResourceData) *chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey {
	v, ok := d.GetOk("additional_target_keys")
	if !ok {
		return nil
	}
	raw := v.(map[string]interface{})
	if len(raw) == 0 {
		return nil
	}
	key := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
		AdditionalTargetKeys: make(map[string]string, len(raw)),
	}
	for k, val := range raw {
		key.AdditionalTargetKeys[k] = val.(string)
	}
	return key
}

func groupPriorityOrderingID(policySchema string, d *schema.ResourceData) string {
	id := policySchema
	if v, ok := d.GetOk("additional_target_keys"); ok {
		raw := v.(map[string]interface{})
		if len(raw) > 0 {
			id += "/" + canonicalAdditionalTargetKeys(raw)
		}
	}
	return id
}
