package googleworkspace

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"google.golang.org/api/chromepolicy/v1"
)

func resourceChromeGroupPolicy() *schema.Resource {
	return &schema.Resource{
		Description: "Chrome Policy resource in the Terraform Googleworkspace provider. " +
			"Chrome Policy Schema resides under the `https://www.googleapis.com/auth/chrome.management.policy` client scope.",

		CreateContext: resourceChromeGroupPolicyCreate,
		UpdateContext: resourceChromeGroupPolicyUpdate,
		ReadContext:   resourceChromeGroupPolicyRead,
		DeleteContext: resourceChromeGroupPolicyDelete,

		Importer: &schema.ResourceImporter{
			StateContext: resourceChromeGroupPolicyImport,
		},

		Schema: map[string]*schema.Schema{
			"group_id": {
				Description: "The target group on which this policy is applied.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"additional_target_keys": {
				Description: "Additional target keys for policies.",
				Type:        schema.TypeList,
				Optional:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"target_key": {
							Description: "The target key name.",
							Type:        schema.TypeString,
							Required:    true,
						},
						"target_value": {
							Description: "The target key value.",
							Type:        schema.TypeString,
							Required:    true,
						},
					},
				},
			},
			"policies": {
				Description: "Policies to set for the org unit",
				Type:        schema.TypeList,
				Required:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"schema_name": {
							Description: "The full qualified name of the policy schema.",
							Type:        schema.TypeString,
							Required:    true,
						},
						"schema_values": {
							Description: "JSON encoded map that represents key/value pairs that " +
								"correspond to the given schema. ",
							Type:     schema.TypeMap,
							Required: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
								ValidateDiagFunc: validation.ToDiagFunc(
									validation.StringIsJSON,
								),
							},
						},
					},
				},
			},
		},
	}
}

func resourceChromeGroupPolicyCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return chromePolicyCreateCommon(ctx, d, meta, targetGroup, groupBatchModify, resourceChromeGroupPolicyRead)
}

// groupBatchModify builds ModifyGroupPolicyRequests and calls Groups.BatchModify.
// Without additional_target_keys, policies are sent one at a time (API workaround).
// With additional_target_keys, all policies for a given key are batched together.
func groupBatchModify(
	ctx context.Context,
	chromePoliciesService *chromepolicy.CustomersPoliciesService,
	customer string,
	targetKey *chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey,
	policies []*chromepolicy.GoogleChromePolicyVersionsV1PolicyValue,
	updateMasks []string,
) error {
	// When additional_target_keys are present (indicated by non-nil map), batch all
	// together. Otherwise send one policy per call as a workaround for the Groups API.
	if targetKey.AdditionalTargetKeys != nil {
		var requests []*chromepolicy.GoogleChromePolicyVersionsV1ModifyGroupPolicyRequest
		for i, p := range policies {
			requests = append(requests, &chromepolicy.GoogleChromePolicyVersionsV1ModifyGroupPolicyRequest{
				PolicyTargetKey: targetKey,
				PolicyValue:     p,
				UpdateMask:      updateMasks[i],
			})
		}
		return retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
			_, retryErr := chromePoliciesService.Groups.BatchModify(
				fmt.Sprintf("customers/%s", customer),
				&chromepolicy.GoogleChromePolicyVersionsV1BatchModifyGroupPoliciesRequest{Requests: requests},
			).Do()
			return retryErr
		})
	}

	// No additional keys: send one policy per call.
	for i, p := range policies {
		req := &chromepolicy.GoogleChromePolicyVersionsV1ModifyGroupPolicyRequest{
			PolicyTargetKey: targetKey,
			PolicyValue:     p,
			UpdateMask:      updateMasks[i],
		}
		err := retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
			_, retryErr := chromePoliciesService.Groups.BatchModify(
				fmt.Sprintf("customers/%s", customer),
				&chromepolicy.GoogleChromePolicyVersionsV1BatchModifyGroupPoliciesRequest{
					Requests: []*chromepolicy.GoogleChromePolicyVersionsV1ModifyGroupPolicyRequest{req},
				},
			).Do()
			return retryErr
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func resourceChromeGroupPolicyUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePoliciesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	if d.HasChange("group_id") {
		oldGroupIDRaw, _ := d.GetChange("group_id")
		oldPoliciesRaw, _ := d.GetChange("policies")
		oldAdditionalKeysRaw, _ := d.GetChange("additional_target_keys")
		oldAdditionalKeysList := oldAdditionalKeysRaw.([]interface{})
		oldHasAdditionalKeys := len(oldAdditionalKeysList) > 0

		// Create on new group FIRST so users are never left without a policy.
		diags = resourceChromeGroupPolicyCreate(ctx, d, meta)
		if diags.HasError() {
			return diags
		}

		// Delete from old group AFTER new group is active.
		log.Printf("[DEBUG] Deleting Chrome Policy from old group:%s after moving to new group", oldGroupIDRaw.(string))
		return deleteChromePoliciesFromGroup(
			ctx, client, chromePoliciesService,
			oldGroupIDRaw.(string),
			oldPoliciesRaw.([]interface{}),
			oldAdditionalKeysRaw,
			oldHasAdditionalKeys,
		)

	} else if d.HasChange("policies") {
		oldPoliciesRaw, newPoliciesRaw := d.GetChange("policies")

		// Apply new/changed schemas FIRST (BatchModify is additive/overwrites).
		diags = resourceChromeGroupPolicyCreate(ctx, d, meta)
		if diags.HasError() {
			return diags
		}

		// Delete only the schemas that were removed AFTER the new set is active.
		removedPolicies := computeRemovedPolicies(oldPoliciesRaw, newPoliciesRaw)
		if len(removedPolicies) > 0 {
			additionalKeysRaw, hasAdditionalKeys := d.GetOk("additional_target_keys")
			return deleteChromePoliciesFromGroup(
				ctx, client, chromePoliciesService,
				d.Id(),
				removedPolicies,
				additionalKeysRaw,
				hasAdditionalKeys,
			)
		}
		return diags

	} else {
		return resourceChromeGroupPolicyCreate(ctx, d, meta)
	}
}

func resourceChromeGroupPolicyRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return chromePolicyReadCommon(ctx, d, meta, targetGroup)
}

func resourceChromeGroupPolicyDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePoliciesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	log.Printf("[DEBUG] Deleting Chrome Policy for groups:%s", d.Id())

	additionalTargetKeysRaw, hasAdditionalKeys := d.GetOk("additional_target_keys")
	diags = deleteChromePoliciesFromGroup(
		ctx, client, chromePoliciesService,
		d.Id(),
		d.Get("policies").([]interface{}),
		additionalTargetKeysRaw,
		hasAdditionalKeys,
	)
	if !diags.HasError() {
		log.Printf("[DEBUG] Finished deleting Chrome Policy for groups:%s", d.Id())
	}
	return diags
}

func deleteChromePoliciesFromGroup(
	ctx context.Context,
	client *apiClient,
	chromePoliciesService *chromepolicy.CustomersPoliciesService,
	groupID string,
	policies []interface{},
	additionalTargetKeysRaw interface{},
	hasAdditionalKeys bool,
) diag.Diagnostics {
	if !hasAdditionalKeys {
		// No additional target keys - delete policies individually.
		// Workaround: send only one policy per batch delete call.
		policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
			TargetResource: "groups/" + groupID,
		}
		for _, p := range policies {
			policy := p.(map[string]interface{})
			schemaName := policy["schema_name"].(string)
			deleteReq := &chromepolicy.GoogleChromePolicyVersionsV1DeleteGroupPolicyRequest{
				PolicyTargetKey: policyTargetKey,
				PolicySchema:    schemaName,
			}
			batchReq := &chromepolicy.GoogleChromePolicyVersionsV1BatchDeleteGroupPoliciesRequest{
				Requests: []*chromepolicy.GoogleChromePolicyVersionsV1DeleteGroupPolicyRequest{deleteReq},
			}
			err := retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
				_, retryErr := chromePoliciesService.Groups.BatchDelete(fmt.Sprintf("customers/%s", client.Customer), batchReq).Do()
				return retryErr
			})
			if err != nil {
				// Ignore errors about apps not being installed.
				if isApiErrorWithCode(err, 400) && strings.Contains(err.Error(), "apps are not installed") {
					log.Printf("[DEBUG] Skipping delete for policy %s - app not installed: %v", schemaName, err)
					continue
				}
				return diag.FromErr(err)
			}
		}
	} else {
		// Have additional_target_keys: group by target_key.
		keyGroups := groupAdditionalTargetKeys(additionalTargetKeysRaw.([]interface{}))

		log.Printf("[DEBUG] Grouped additional_target_keys into %d groups", len(keyGroups))

		for targetKey, keyValuePairs := range keyGroups {
			log.Printf("[DEBUG] Processing target_key: %s with %d target_values", targetKey, len(keyValuePairs))

			for _, keyValuePair := range keyValuePairs {
				log.Printf("[DEBUG] Batching policies for deletion: target_key=%s, target_value=%s", keyValuePair["key"], keyValuePair["value"])

				policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
					TargetResource: "groups/" + groupID,
					AdditionalTargetKeys: map[string]string{
						keyValuePair["key"]: keyValuePair["value"],
					},
				}

				var deleteRequests []*chromepolicy.GoogleChromePolicyVersionsV1DeleteGroupPolicyRequest
				for _, p := range policies {
					policy := p.(map[string]interface{})
					schemaName := policy["schema_name"].(string)
					deleteRequests = append(deleteRequests, &chromepolicy.GoogleChromePolicyVersionsV1DeleteGroupPolicyRequest{
						PolicyTargetKey: policyTargetKey,
						PolicySchema:    schemaName,
					})
				}

				batchReq := &chromepolicy.GoogleChromePolicyVersionsV1BatchDeleteGroupPoliciesRequest{
					Requests: deleteRequests,
				}

				log.Printf("[DEBUG] Making BatchDelete call for target_key=%s, target_value=%s with %d policies", keyValuePair["key"], keyValuePair["value"], len(deleteRequests))

				err := retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
					_, retryErr := chromePoliciesService.Groups.BatchDelete(fmt.Sprintf("customers/%s", client.Customer), batchReq).Do()
					return retryErr
				})
				if err != nil {
					if isApiErrorWithCode(err, 400) && strings.Contains(err.Error(), "apps are not installed") {
						log.Printf("[DEBUG] Ignoring error about apps not being installed during policy deletion for %s=%s: %v", keyValuePair["key"], keyValuePair["value"], err)
					} else {
						return diag.FromErr(err)
					}
				}
			}
		}
	}
	return nil
}

// computeRemovedPolicies returns entries from oldPolicies whose schema_name
// is absent from newPolicies. Used during updates to clean up dropped schemas.
func computeRemovedPolicies(oldPolicies, newPolicies interface{}) []interface{} {
	newSchemaNames := make(map[string]bool)
	if newList, ok := newPolicies.([]interface{}); ok {
		for _, p := range newList {
			if policy, ok := p.(map[string]interface{}); ok {
				if name, ok := policy["schema_name"].(string); ok {
					newSchemaNames[name] = true
				}
			}
		}
	}
	var removed []interface{}
	if oldList, ok := oldPolicies.([]interface{}); ok {
		for _, p := range oldList {
			if policy, ok := p.(map[string]interface{}); ok {
				if name, ok := policy["schema_name"].(string); ok {
					if !newSchemaNames[name] {
						removed = append(removed, p)
					}
				}
			}
		}
	}
	return removed
}

func resourceChromeGroupPolicyImport(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	return chromePolicyImportCommon(ctx, d, meta, targetGroup, "group_id")
}
