package googleworkspace

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"google.golang.org/api/chromepolicy/v1"
)

func resourceChromePolicyGroupPriorityOrdering() *schema.Resource {
	return &schema.Resource{
		Description: "Manages the group priority ordering for a Chrome policy. " +
			"When multiple groups have the same policy configured, the priority " +
			"ordering determines which group's policy takes precedence. This " +
			"resource is authoritative over the complete ordering.",

		CreateContext: resourceChromePolicyGroupPriorityOrderingCreate,
		ReadContext:   resourceChromePolicyGroupPriorityOrderingRead,
		UpdateContext: resourceChromePolicyGroupPriorityOrderingUpdate,
		DeleteContext: resourceChromePolicyGroupPriorityOrderingDelete,

		Importer: &schema.ResourceImporter{
			StateContext: resourceChromePolicyGroupPriorityOrderingImport,
		},

		Schema: map[string]*schema.Schema{
			"policy_schema": {
				Description: "The full qualified name of the policy schema, e.g. `chrome.users.MaxConnectionsPerProxy`.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"additional_target_keys": {
				Description: "Additional target keys for app-scoped policies, " +
					"e.g. `{app_id = \"chrome:aabbcc\"}`. Omit for non-app-scoped policies.",
				Type:     schema.TypeMap,
				Optional: true,
				ForceNew: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"group_ids": {
				Description: "The group IDs in desired priority order (highest priority first).",
				Type:        schema.TypeList,
				Required:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func resourceChromePolicyGroupPriorityOrderingCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)
	policySchema := d.Get("policy_schema").(string)
	groupIds := expandStringList(d.Get("group_ids").([]interface{}))

	log.Printf("[DEBUG] Creating Chrome Policy Group Priority Ordering for %s", policySchema)

	if len(groupIds) > 0 {
		if diags := updateGroupPriorityOrdering(ctx, client, policySchema, groupIds, d); diags.HasError() {
			return diags
		}
	}

	d.SetId(groupPriorityOrderingID(policySchema, d))
	return resourceChromePolicyGroupPriorityOrderingRead(ctx, d, meta)
}

func resourceChromePolicyGroupPriorityOrderingRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
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

	log.Printf("[DEBUG] Reading Chrome Policy Group Priority Ordering for %s", policySchema)

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
			if err := d.Set("group_ids", []string{}); err != nil {
				return diag.FromErr(err)
			}
			return nil
		}
		return diag.FromErr(err)
	}

	if err := d.Set("group_ids", resp.GroupIds); err != nil {
		return diag.FromErr(err)
	}

	log.Printf("[DEBUG] Finished reading Chrome Policy Group Priority Ordering for %s: %d groups", policySchema, len(resp.GroupIds))
	return nil
}

func resourceChromePolicyGroupPriorityOrderingUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)
	policySchema := d.Get("policy_schema").(string)
	groupIds := expandStringList(d.Get("group_ids").([]interface{}))

	log.Printf("[DEBUG] Updating Chrome Policy Group Priority Ordering for %s", policySchema)

	if len(groupIds) > 0 {
		if diags := updateGroupPriorityOrdering(ctx, client, policySchema, groupIds, d); diags.HasError() {
			return diags
		}
	}

	return resourceChromePolicyGroupPriorityOrderingRead(ctx, d, meta)
}

func resourceChromePolicyGroupPriorityOrderingDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG] Removing Chrome Policy Group Priority Ordering from state (no-op)")
	return nil
}

func resourceChromePolicyGroupPriorityOrderingImport(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	parts := strings.SplitN(d.Id(), "/", 2)
	if len(parts) < 1 || parts[0] == "" {
		return nil, fmt.Errorf("expected import ID format: {policy_schema} or {policy_schema}/{key=value,...}, got: %s", d.Id())
	}

	if err := d.Set("policy_schema", parts[0]); err != nil {
		return nil, err
	}

	if len(parts) == 2 && parts[1] != "" {
		atk, err := parseAdditionalTargetKeys(parts[1])
		if err != nil {
			return nil, err
		}
		if err := d.Set("additional_target_keys", atk); err != nil {
			return nil, err
		}
	}

	return []*schema.ResourceData{d}, nil
}

func updateGroupPriorityOrdering(
	ctx context.Context,
	client *apiClient,
	policySchema string,
	groupIds []string,
	d *schema.ResourceData,
) diag.Diagnostics {
	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	groupsService, diags := GetChromePoliciesGroupsService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	req := &chromepolicy.GoogleChromePolicyVersionsV1UpdateGroupPriorityOrderingRequest{
		PolicySchema:    policySchema,
		GroupIds:        groupIds,
		PolicyTargetKey: buildGroupPriorityTargetKey(d),
	}

	err := retryTimeDuration(ctx, time.Minute, func() error {
		_, retryErr := groupsService.UpdateGroupPriorityOrdering(
			fmt.Sprintf("customers/%s", client.Customer), req,
		).Do()
		return retryErr
	})
	if err != nil {
		return diag.FromErr(err)
	}

	return nil
}

// parseAdditionalTargetKeys parses "key1=val1,key2=val2" into a map.
func parseAdditionalTargetKeys(raw string) (map[string]string, error) {
	result := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid additional_target_keys format %q; expected key=value pairs separated by commas", raw)
		}
		result[kv[0]] = kv[1]
	}
	return result, nil
}

func expandStringList(v []interface{}) []string {
	result := make([]string, len(v))
	for i, item := range v {
		result[i] = item.(string)
	}
	return result
}

// canonicalAdditionalTargetKeysFromMap produces a deterministic string for
// a map[string]string, for use in resource IDs.
func canonicalAdditionalTargetKeysFromMap(keys map[string]string) string {
	if len(keys) == 0 {
		return ""
	}
	sorted := make([]string, 0, len(keys))
	for k, v := range keys {
		sorted = append(sorted, k+"="+v)
	}
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}
