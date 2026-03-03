package googleworkspace

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

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
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePoliciesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	targetID := d.Get("group_id").(string)

	log.Printf("[DEBUG] Creating Chrome Policy for groups:%s", targetID)

	policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
		TargetResource: "groups/" + targetID,
	}

	if _, ok := d.GetOk("additional_target_keys"); ok {
		policyTargetKey.AdditionalTargetKeys = expandChromePoliciesAdditionalTargetKeys(d.Get("additional_target_keys").([]interface{}))
	}

	diags = validateChromePolicies(ctx, d, client)
	if diags.HasError() {
		return diags
	}

	policies, diags := expandChromePoliciesValues(d.Get("policies").([]interface{}))
	if diags.HasError() {
		return diags
	}

	log.Printf("[DEBUG] Expanded policies: %+v", policies)

	// Check if we have additional_target_keys
	additionalTargetKeysRaw, hasAdditionalKeys := d.GetOk("additional_target_keys")

	if !hasAdditionalKeys {
		// No additional_target_keys: make individual call for each policy
		log.Printf("[DEBUG] No additional_target_keys - processing policies individually")
		for _, p := range policies {
			policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
				TargetResource: "groups/" + targetID,
			}

			var keys []string
			var schemaValues map[string]interface{}
			if err := json.Unmarshal(p.Value, &schemaValues); err != nil {
				return diag.FromErr(err)
			}
			for key := range schemaValues {
				keys = append(keys, key)
			}

			req := &chromepolicy.GoogleChromePolicyVersionsV1ModifyGroupPolicyRequest{
				PolicyTargetKey: policyTargetKey,
				PolicyValue:     p,
				UpdateMask:      strings.Join(keys, ","),
			}

			batchReq := &chromepolicy.GoogleChromePolicyVersionsV1BatchModifyGroupPoliciesRequest{
				Requests: []*chromepolicy.GoogleChromePolicyVersionsV1ModifyGroupPolicyRequest{req},
			}

			err := retryTimeDuration(ctx, time.Minute, func() error {
				_, retryErr := chromePoliciesService.Groups.BatchModify(fmt.Sprintf("customers/%s", client.Customer), batchReq).Do()
				return retryErr
			})

			if err != nil {
				return diag.FromErr(err)
			}
		}
	} else {
		// Have additional_target_keys: group by target_key
		additionalTargetKeysList := additionalTargetKeysRaw.([]interface{})

		// Group additional_target_keys by their target_key
		keyGroups := make(map[string][]map[string]string)
		for _, k := range additionalTargetKeysList {
			targetKeyDef := k.(map[string]interface{})
			targetKeyName := targetKeyDef["target_key"].(string)
			targetKeyValue := targetKeyDef["target_value"].(string)

			keyGroups[targetKeyName] = append(keyGroups[targetKeyName], map[string]string{
				"key":   targetKeyName,
				"value": targetKeyValue,
			})
		}

		log.Printf("[DEBUG] Grouped additional_target_keys by target_key: %d groups", len(keyGroups))

		// Process each group of target_keys
		for targetKeyName, keyValuePairs := range keyGroups {
			log.Printf("[DEBUG] Processing target_key group: %s with %d values", targetKeyName, len(keyValuePairs))

			// For each value in this target_key group, create requests for all policies
			for _, keyValuePair := range keyValuePairs {
				policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
					TargetResource: "groups/" + targetID,
					AdditionalTargetKeys: map[string]string{
						keyValuePair["key"]: keyValuePair["value"],
					},
				}

				var requests []*chromepolicy.GoogleChromePolicyVersionsV1ModifyGroupPolicyRequest
				for _, p := range policies {
					var keys []string
					var schemaValues map[string]interface{}
					if err := json.Unmarshal(p.Value, &schemaValues); err != nil {
						return diag.FromErr(err)
					}
					for key := range schemaValues {
						keys = append(keys, key)
					}

					req := &chromepolicy.GoogleChromePolicyVersionsV1ModifyGroupPolicyRequest{
						PolicyTargetKey: policyTargetKey,
						PolicyValue:     p,
						UpdateMask:      strings.Join(keys, ","),
					}
					requests = append(requests, req)
				}

				// Batch all policies for this specific additional_target_key value
				batchReq := &chromepolicy.GoogleChromePolicyVersionsV1BatchModifyGroupPoliciesRequest{
					Requests: requests,
				}

				log.Printf("[DEBUG] Batching %d policies for %s=%s", len(requests), keyValuePair["key"], keyValuePair["value"])

				err := retryTimeDuration(ctx, time.Minute, func() error {
					_, retryErr := chromePoliciesService.Groups.BatchModify(fmt.Sprintf("customers/%s", client.Customer), batchReq).Do()
					return retryErr
				})

				if err != nil {
					return diag.FromErr(err)
				}
			}
		}
	}

	log.Printf("[DEBUG] Finished creating Chrome Policy for groups:%s", targetID)
	d.SetId(targetID)

	return resourceChromeGroupPolicyRead(ctx, d, meta)
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
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePoliciesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	log.Printf("[DEBUG] Getting Chrome Policy for groups:%s", d.Id())

	policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
		TargetResource: "groups/" + d.Id(),
	}

	if _, ok := d.GetOk("additional_target_keys"); ok {
		policyTargetKey.AdditionalTargetKeys = expandChromePoliciesAdditionalTargetKeys(d.Get("additional_target_keys").([]interface{}))
	}

	policiesObj := []*chromepolicy.GoogleChromePolicyVersionsV1PolicyValue{}
	for _, p := range d.Get("policies").([]interface{}) {
		policy := p.(map[string]interface{})
		schemaName := policy["schema_name"].(string)

		var resp *chromepolicy.GoogleChromePolicyVersionsV1ResolveResponse
		err := retryTimeDuration(ctx, time.Minute, func() error {
			var retryErr error
			resp, retryErr = chromePoliciesService.Resolve(fmt.Sprintf("customers/%s", client.Customer), &chromepolicy.GoogleChromePolicyVersionsV1ResolveRequest{
				PolicySchemaFilter: schemaName,
				PolicyTargetKey:    policyTargetKey,
			}).Do()
			return retryErr
		})
		if err != nil {
			// Check if it's a 404 error - the group or policy was deleted outside of Terraform
			return handleNotFoundError(err, d, fmt.Sprintf("Chrome Group Policy %s", d.Id()))
		}

		// Handle cases where policy might not exist or has been deleted
		// This can happen when removing a resource from terraform config
		if len(resp.ResolvedPolicies) == 0 {
			log.Printf("[DEBUG] No resolved policies found for schema %s - policy may have been deleted", schemaName)
			// Skip this policy - it doesn't exist in Google anymore
			continue
		}

		if len(resp.ResolvedPolicies) != 1 {
			log.Printf("[WARN] Expected 1 resolved policy for schema %s, got %d", schemaName, len(resp.ResolvedPolicies))
			// Use the first policy if multiple are returned
		}

		value := resp.ResolvedPolicies[0].Value
		policiesObj = append(policiesObj, value)
	}

	policies, diags := flattenChromePolicies(ctx, policiesObj, client)
	if diags.HasError() {
		return diags
	}

	if err := d.Set("policies", policies); err != nil {
		return diag.FromErr(err)
	}

	log.Printf("[DEBUG] Finished getting Chrome Policy for groups:%s", d.Id())
	return nil
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
			err := retryTimeDuration(ctx, time.Minute, func() error {
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
		additionalTargetKeysList := additionalTargetKeysRaw.([]interface{})

		keyGroups := make(map[string][]map[string]string)
		for _, k := range additionalTargetKeysList {
			targetKeyDef := k.(map[string]interface{})
			targetKeyName := targetKeyDef["target_key"].(string)
			targetKeyValue := targetKeyDef["target_value"].(string)
			keyGroups[targetKeyName] = append(keyGroups[targetKeyName], map[string]string{
				"key":   targetKeyName,
				"value": targetKeyValue,
			})
		}

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

				err := retryTimeDuration(ctx, time.Minute, func() error {
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
	parts := strings.Split(d.Id(), "/")
	if len(parts) < 2 || len(parts) > 3 {
		return nil, fmt.Errorf("invalid import ID format, expected '<group_id>/<schemas>' or '<group_id>/<additional_keys>/<schemas>', got: %s", d.Id())
	}

	groupId := parts[0]
	var schemasStr string
	var additionalTargetKeys []interface{}

	if len(parts) == 3 {
		// Parse additional_target_keys: key=value+key=value
		for _, pair := range strings.Split(parts[1], "+") {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) != 2 {
				return nil, fmt.Errorf("invalid additional_target_key format '%s', expected 'key=value'", pair)
			}
			additionalTargetKeys = append(additionalTargetKeys, map[string]interface{}{
				"target_key":   kv[0],
				"target_value": kv[1],
			})
		}
		schemasStr = parts[2]
	} else {
		schemasStr = parts[1]
	}

	schemaNames := strings.Split(schemasStr, ",")
	for i := range schemaNames {
		schemaNames[i] = strings.TrimSpace(schemaNames[i])
	}

	// Strict existence validation: verify each policy is EXPLICITLY set on this target.
	// Uses sourceKey.targetResource from Resolve() response to check where the value
	// was actually set. If it doesn't match our target, the policy is inherited.
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return nil, fmt.Errorf("failed to create Chrome Policy service: %s", diags[0].Summary)
	}

	chromePoliciesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return nil, fmt.Errorf("failed to get Chrome Policies service: %s", diags[0].Summary)
	}

	expectedTargetResource := "groups/" + groupId
	policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
		TargetResource: expectedTargetResource,
	}
	if len(additionalTargetKeys) > 0 {
		atk := make(map[string]string)
		for _, k := range additionalTargetKeys {
			kv := k.(map[string]interface{})
			atk[kv["target_key"].(string)] = kv["target_value"].(string)
		}
		policyTargetKey.AdditionalTargetKeys = atk
	}

	for _, schemaName := range schemaNames {
		var resp *chromepolicy.GoogleChromePolicyVersionsV1ResolveResponse
		err := retryTimeDuration(ctx, time.Minute, func() error {
			var retryErr error
			resp, retryErr = chromePoliciesService.Resolve(
				fmt.Sprintf("customers/%s", client.Customer),
				&chromepolicy.GoogleChromePolicyVersionsV1ResolveRequest{
					PolicySchemaFilter: schemaName,
					PolicyTargetKey:    policyTargetKey,
				},
			).Do()
			return retryErr
		})
		if err != nil {
			return nil, fmt.Errorf("import failed: could not resolve policy %s for %s: %v", schemaName, expectedTargetResource, err)
		}
		if len(resp.ResolvedPolicies) == 0 {
			return nil, fmt.Errorf("import failed: policy %s does not exist on %s", schemaName, expectedTargetResource)
		}
		// Check if the policy is explicitly set on THIS target or inherited from a parent.
		// Inherited policies are allowed (they have valid values) but logged as warnings.
		// After import, Terraform will manage them: if the config matches the inherited
		// value there's no change; if it differs, the next apply will set it explicitly.
		sourceTarget := resp.ResolvedPolicies[0].SourceKey.TargetResource
		if sourceTarget != expectedTargetResource {
			log.Printf("[WARN] Import: policy %s on %s is inherited from %s (not explicitly set). "+
				"Terraform will manage this policy going forward.",
				schemaName, expectedTargetResource, sourceTarget,
			)
		}
	}

	d.SetId(groupId)
	d.Set("group_id", groupId)

	if len(additionalTargetKeys) > 0 {
		d.Set("additional_target_keys", additionalTargetKeys)
	}

	// Pre-populate policies with schema names so Read can call Resolve()
	var policies []interface{}
	for _, schemaName := range schemaNames {
		policies = append(policies, map[string]interface{}{
			"schema_name":   schemaName,
			"schema_values": map[string]interface{}{},
		})
	}
	d.Set("policies", policies)

	log.Printf("[DEBUG] Import Chrome Group Policy for %s with %d schemas", expectedTargetResource, len(schemaNames))

	return []*schema.ResourceData{d}, nil
}
