package googleworkspace

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"google.golang.org/api/chromepolicy/v1"
)

func resourceChromePolicyGroupPriorityOrdering() *schema.Resource {
	return &schema.Resource{
		Description: "Chrome Policy Group Priority Ordering resource in the Terraform Googleworkspace provider. " +
			"This resource manages the priority ordering of groups for a specific Chrome policy. " +
			"Groups with higher priority (earlier in the list) override policies from groups with lower priority.",

		CreateContext: resourceChromePolicyGroupPriorityOrderingCreate,
		UpdateContext: resourceChromePolicyGroupPriorityOrderingUpdate,
		ReadContext:   resourceChromePolicyGroupPriorityOrderingRead,
		DeleteContext: resourceChromePolicyGroupPriorityOrderingDelete,

		Importer: &schema.ResourceImporter{
			StateContext: resourceChromePolicyGroupPriorityOrderingImport,
		},

		Schema: map[string]*schema.Schema{
			"policy_schema": {
				Description: "The full qualified name of the policy schema.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"policy_namespace": {
				Description: "The namespace of the policy type for the request.",
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
			},
			"policy_target_key": {
				Description: "The target resource for which the group priority ordering applies. " +
					"Required only for policies that use policyTargetKey. " +
					"When provided, the target app must be supplied in additional_target_key_names.",
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"target_resource": {
							Description: "The target resource on which this policy is applied (e.g., 'orgunits/123').",
							Type:        schema.TypeString,
							Required:    true,
						},
						"additional_target_key_names": {
							Description: "Map of additional target keys. The target app must be supplied here.",
							Type:        schema.TypeMap,
							Required:    true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
					},
				},
			},
			"group_ids": {
				Description: "Ordered list of group IDs. Groups earlier in the list have higher priority " +
					"and their policies will override those of groups later in the list.",
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},
	}
}

func resourceChromePolicyGroupPriorityOrderingCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePolicyGroupsService, diags := GetChromePolicyGroupsService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	policySchema := d.Get("policy_schema").(string)
	policyNamespace := d.Get("policy_namespace").(string)

	policyTargetKey := expandPolicyTargetKey(d.Get("policy_target_key").([]interface{}))
	groupIds := expandStringList(d.Get("group_ids").([]interface{}))

	log.Printf("[DEBUG] Creating Chrome Policy Group Priority Ordering for schema: %s", policySchema)

	req := &chromepolicy.GoogleChromePolicyVersionsV1UpdateGroupPriorityOrderingRequest{
		PolicyTargetKey: policyTargetKey,
		PolicyNamespace: policyNamespace,
		PolicySchema:    policySchema,
		GroupIds:        groupIds,
	}

	err := retryTimeDuration(ctx, time.Minute, func() error {
		_, retryErr := chromePolicyGroupsService.UpdateGroupPriorityOrdering(fmt.Sprintf("customers/%s", client.Customer), req).Do()
		return retryErr
	})

	if err != nil {
		return diag.FromErr(err)
	}

	// Create an ID based on the policy schema and optionally target resource
	targetResource := ""
	if policyTargetKey != nil {
		targetResource = policyTargetKey.TargetResource
	}
	id := generateChromePolicyGroupPriorityOrderingID(policySchema, targetResource)
	d.SetId(id)

	log.Printf("[DEBUG] Finished creating Chrome Policy Group Priority Ordering: %s", id)

	return resourceChromePolicyGroupPriorityOrderingRead(ctx, d, meta)
}

func resourceChromePolicyGroupPriorityOrderingUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePolicyGroupsService, diags := GetChromePolicyGroupsService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	policySchema := d.Get("policy_schema").(string)
	policyNamespace := d.Get("policy_namespace").(string)

	policyTargetKey := expandPolicyTargetKey(d.Get("policy_target_key").([]interface{}))
	groupIds := expandStringList(d.Get("group_ids").([]interface{}))

	log.Printf("[DEBUG] Updating Chrome Policy Group Priority Ordering: %s", d.Id())

	req := &chromepolicy.GoogleChromePolicyVersionsV1UpdateGroupPriorityOrderingRequest{
		PolicyTargetKey: policyTargetKey,
		PolicyNamespace: policyNamespace,
		PolicySchema:    policySchema,
		GroupIds:        groupIds,
	}

	err := retryTimeDuration(ctx, time.Minute, func() error {
		_, retryErr := chromePolicyGroupsService.UpdateGroupPriorityOrdering(fmt.Sprintf("customers/%s", client.Customer), req).Do()
		return retryErr
	})

	if err != nil {
		return diag.FromErr(err)
	}

	log.Printf("[DEBUG] Finished updating Chrome Policy Group Priority Ordering: %s", d.Id())

	return resourceChromePolicyGroupPriorityOrderingRead(ctx, d, meta)
}

func resourceChromePolicyGroupPriorityOrderingRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePolicyGroupsService, diags := GetChromePolicyGroupsService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	policySchema := d.Get("policy_schema").(string)
	policyNamespace := d.Get("policy_namespace").(string)
	policyTargetKey := expandPolicyTargetKey(d.Get("policy_target_key").([]interface{}))

	log.Printf("[DEBUG] Reading Chrome Policy Group Priority Ordering: %s", d.Id())

	req := &chromepolicy.GoogleChromePolicyVersionsV1ListGroupPriorityOrderingRequest{
		PolicyTargetKey: policyTargetKey,
		PolicyNamespace: policyNamespace,
		PolicySchema:    policySchema,
	}

	var resp *chromepolicy.GoogleChromePolicyVersionsV1ListGroupPriorityOrderingResponse
	err := retryTimeDuration(ctx, time.Minute, func() error {
		var retryErr error
		resp, retryErr = chromePolicyGroupsService.ListGroupPriorityOrdering(fmt.Sprintf("customers/%s", client.Customer), req).Do()
		return retryErr
	})

	if err != nil {
		// The API returns 400 "not configured on any Groups" when the policy
		// has no group assignments (e.g., groups were removed or policy was
		// deleted outside Terraform). Treat this as "resource no longer exists".
		if isApiErrorWithCode(err, 400) {
			log.Printf("[WARN] Removing Chrome Policy Group Priority Ordering %s from state: %v", d.Id(), err)
			d.SetId("")
			return nil
		}
		return handleNotFoundError(err, d, fmt.Sprintf("Chrome Policy Group Priority Ordering %s", d.Id()))
	}

	if err := d.Set("group_ids", resp.GroupIds); err != nil {
		return diag.FromErr(err)
	}

	log.Printf("[DEBUG] Finished reading Chrome Policy Group Priority Ordering: %s", d.Id())

	return nil
}

func resourceChromePolicyGroupPriorityOrderingDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// The Chrome Policy API does not support deleting a group priority ordering
	// (sending an empty group list returns "Request must have at least one Group ID").
	// Group priority orderings are inherent to the policy — they exist as long as
	// multiple groups have the same policy applied. We simply remove the resource
	// from Terraform state and leave the ordering as-is in Google Admin.
	log.Printf("[DEBUG] Removing Chrome Policy Group Priority Ordering from state: %s (API does not support deletion)", d.Id())
	d.SetId("")
	return nil
}

func resourceChromePolicyGroupPriorityOrderingImport(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	// ID format: policySchema:targetResource
	parts := strings.SplitN(d.Id(), ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid ID format, expected 'policySchema:targetResource', got: %s", d.Id())
	}

	policySchema := parts[0]
	targetResource := parts[1]

	if err := d.Set("policy_schema", policySchema); err != nil {
		return nil, err
	}

	policyTargetKey := []interface{}{
		map[string]interface{}{
			"target_resource":             targetResource,
			"additional_target_key_names": map[string]interface{}{},
		},
	}

	if err := d.Set("policy_target_key", policyTargetKey); err != nil {
		return nil, err
	}

	return []*schema.ResourceData{d}, nil
}

// Helper functions

func expandPolicyTargetKey(targetKeys []interface{}) *chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey {
	if len(targetKeys) == 0 {
		return nil
	}

	targetKey := targetKeys[0].(map[string]interface{})
	policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
		TargetResource: targetKey["target_resource"].(string),
	}

	if additionalKeys, ok := targetKey["additional_target_key_names"].(map[string]interface{}); ok {
		policyTargetKey.AdditionalTargetKeys = make(map[string]string)
		for k, v := range additionalKeys {
			policyTargetKey.AdditionalTargetKeys[k] = v.(string)
		}
	}

	return policyTargetKey
}

func expandStringList(list []interface{}) []string {
	result := make([]string, len(list))
	for i, v := range list {
		result[i] = v.(string)
	}
	return result
}

func generateChromePolicyGroupPriorityOrderingID(policySchema, targetResource string) string {
	return fmt.Sprintf("%s:%s", policySchema, targetResource)
}
