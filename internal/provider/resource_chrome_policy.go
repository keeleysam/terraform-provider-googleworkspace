package googleworkspace

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"google.golang.org/api/chromepolicy/v1"
)

func resourceChromePolicy() *schema.Resource {
	return &schema.Resource{
		Description: "Chrome Policy resource in the Terraform Googleworkspace provider. " +
			"Chrome Policy Schema resides under the `https://www.googleapis.com/auth/chrome.management.policy` client scope.",

		CreateContext: resourceChromePolicyCreate,
		UpdateContext: resourceChromePolicyUpdate,
		ReadContext:   resourceChromePolicyRead,
		DeleteContext: resourceChromePolicyDelete,

		Importer: &schema.ResourceImporter{
			StateContext: resourceChromePolicyImport,
		},

		Schema: map[string]*schema.Schema{
			"org_unit_id": {
				Description:      "The target org unit on which this policy is applied.",
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				DiffSuppressFunc: diffSuppressOrgUnitId,
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

func resourceChromePolicyCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePoliciesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	targetID := strings.TrimPrefix(d.Get("org_unit_id").(string), "id:")

	log.Printf("[DEBUG] Creating Chrome Policy for orgunits:%s", targetID)

	policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
		TargetResource: "orgunits/" + targetID,
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
		// No additional_target_keys: batch all policies together
		log.Printf("[DEBUG] No additional_target_keys - batching all policies together")

		policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
			TargetResource: "orgunits/" + targetID,
		}

		var requests []*chromepolicy.GoogleChromePolicyVersionsV1ModifyOrgUnitPolicyRequest
		for _, p := range policies {
			var keys []string
			var schemaValues map[string]interface{}
			if err := json.Unmarshal(p.Value, &schemaValues); err != nil {
				return diag.FromErr(err)
			}
			for key := range schemaValues {
				keys = append(keys, key)
			}
			requests = append(requests, &chromepolicy.GoogleChromePolicyVersionsV1ModifyOrgUnitPolicyRequest{
				PolicyTargetKey: policyTargetKey,
				PolicyValue:     p,
				UpdateMask:      strings.Join(keys, ","),
			})
		}

		err := retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
			_, retryErr := chromePoliciesService.Orgunits.BatchModify(fmt.Sprintf("customers/%s", client.Customer), &chromepolicy.GoogleChromePolicyVersionsV1BatchModifyOrgUnitPoliciesRequest{Requests: requests}).Do()
			return retryErr
		})
		if err != nil {
			return diag.FromErr(err)
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
					TargetResource: "orgunits/" + targetID,
					AdditionalTargetKeys: map[string]string{
						keyValuePair["key"]: keyValuePair["value"],
					},
				}

				var requests []*chromepolicy.GoogleChromePolicyVersionsV1ModifyOrgUnitPolicyRequest
				for _, p := range policies {
					var keys []string
					var schemaValues map[string]interface{}
					if err := json.Unmarshal(p.Value, &schemaValues); err != nil {
						return diag.FromErr(err)
					}
					for key := range schemaValues {
						keys = append(keys, key)
					}

					req := &chromepolicy.GoogleChromePolicyVersionsV1ModifyOrgUnitPolicyRequest{
						PolicyTargetKey: policyTargetKey,
						PolicyValue:     p,
						UpdateMask:      strings.Join(keys, ","),
					}
					requests = append(requests, req)
				}

				// Batch all policies for this specific additional_target_key value
				batchReq := &chromepolicy.GoogleChromePolicyVersionsV1BatchModifyOrgUnitPoliciesRequest{
					Requests: requests,
				}

				log.Printf("[DEBUG] Batching %d policies for %s=%s", len(requests), keyValuePair["key"], keyValuePair["value"])

				err := retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
					_, retryErr := chromePoliciesService.Orgunits.BatchModify(fmt.Sprintf("customers/%s", client.Customer), batchReq).Do()
					return retryErr
				})

				if err != nil {
					return diag.FromErr(err)
				}
			}
		}
	}

	log.Printf("[DEBUG] Finished creating Chrome Policy for orgunits:%s", targetID)
	d.SetId(targetID)

	return resourceChromePolicyRead(ctx, d, meta)
}

func resourceChromePolicyUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePoliciesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	log.Printf("[DEBUG] Updating Chrome Policy for orgunits:%s", d.Id())

	old, _ := d.GetChange("policies")

	// For org units, we use inherit-then-create pattern
	// Check if we have additional_target_keys
	additionalTargetKeysRaw, hasAdditionalKeys := d.GetOk("additional_target_keys")

	if !hasAdditionalKeys {
		// No additional target keys - batch all policies together
		policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
			TargetResource: "orgunits/" + d.Id(),
		}

		var requests []*chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest
		for _, p := range old.([]interface{}) {
			policy := p.(map[string]interface{})
			schemaName := policy["schema_name"].(string)

			requests = append(requests, &chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest{
				PolicyTargetKey: policyTargetKey,
				PolicySchema:    schemaName,
			})
		}

		err := retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
			_, retryErr := chromePoliciesService.Orgunits.BatchInherit(fmt.Sprintf("customers/%s", client.Customer), &chromepolicy.GoogleChromePolicyVersionsV1BatchInheritOrgUnitPoliciesRequest{Requests: requests}).Do()
			return retryErr
		})
		if err != nil {
			// Ignore 400 errors about apps not being installed - this happens when deleting policies for apps
			// that were uninstalled from the domain
			if isApiErrorWithCode(err, 400) && strings.Contains(err.Error(), "apps are not installed") {
				log.Printf("[DEBUG] Ignoring error about apps not being installed during policy inheritance: %v", err)
			} else {
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

		log.Printf("[DEBUG] Grouped additional_target_keys into %d groups", len(keyGroups))

		// For each unique target_key, batch all policies for each unique target_value
		for targetKey, keyValuePairs := range keyGroups {
			log.Printf("[DEBUG] Processing target_key: %s with %d target_values", targetKey, len(keyValuePairs))

			for _, keyValuePair := range keyValuePairs {
				log.Printf("[DEBUG] Batching policies for target_key=%s, target_value=%s", keyValuePair["key"], keyValuePair["value"])

				policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
					TargetResource: "orgunits/" + d.Id(),
					AdditionalTargetKeys: map[string]string{
						keyValuePair["key"]: keyValuePair["value"],
					},
				}

				var requests []*chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest
				for _, p := range old.([]interface{}) {
					policy := p.(map[string]interface{})
					schemaName := policy["schema_name"].(string)

					requests = append(requests, &chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest{
						PolicyTargetKey: policyTargetKey,
						PolicySchema:    schemaName,
					})
				}

				log.Printf("[DEBUG] Making BatchInherit call for target_key=%s, target_value=%s with %d policies", keyValuePair["key"], keyValuePair["value"], len(requests))

				err := retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
					_, retryErr := chromePoliciesService.Orgunits.BatchInherit(fmt.Sprintf("customers/%s", client.Customer), &chromepolicy.GoogleChromePolicyVersionsV1BatchInheritOrgUnitPoliciesRequest{Requests: requests}).Do()
					return retryErr
				})
				if err != nil {
					// Ignore 400 errors about apps not being installed - this happens when deleting policies for apps
					// that were uninstalled from the domain
					if isApiErrorWithCode(err, 400) && strings.Contains(err.Error(), "apps are not installed") {
						log.Printf("[DEBUG] Ignoring error about apps not being installed during policy inheritance for %s=%s: %v", keyValuePair["key"], keyValuePair["value"], err)
					} else {
						return diag.FromErr(err)
					}
				}
			}
		}
	}

	// Re-run create logic to apply the new set
	diags = resourceChromePolicyCreate(ctx, d, meta)
	if diags.HasError() {
		return diags
	}

	log.Printf("[DEBUG] Finished updating Chrome Policy for orgunits:%s", d.Id())
	return diags
}

func resourceChromePolicyRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePoliciesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	log.Printf("[DEBUG] Getting Chrome Policy for orgunits:%s", d.Id())

	policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
		TargetResource: "orgunits/" + d.Id(),
	}

	if _, ok := d.GetOk("additional_target_keys"); ok {
		policyTargetKey.AdditionalTargetKeys = expandChromePoliciesAdditionalTargetKeys(d.Get("additional_target_keys").([]interface{}))
	}

	policiesObj := []*chromepolicy.GoogleChromePolicyVersionsV1PolicyValue{}
	for _, p := range d.Get("policies").([]interface{}) {
		policy := p.(map[string]interface{})
		schemaName := policy["schema_name"].(string)

		var resp *chromepolicy.GoogleChromePolicyVersionsV1ResolveResponse
		err := retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
			var retryErr error
			resp, retryErr = chromePoliciesService.Resolve(fmt.Sprintf("customers/%s", client.Customer), &chromepolicy.GoogleChromePolicyVersionsV1ResolveRequest{
				PolicySchemaFilter: schemaName,
				PolicyTargetKey:    policyTargetKey,
			}).Do()
			return retryErr
		})
		if err != nil {
			// Check if it's a 404 error - the orgunit or policy was deleted outside of Terraform
			return handleNotFoundError(err, d, fmt.Sprintf("Chrome Policy %s", d.Id()))
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

	log.Printf("[DEBUG] Finished getting Chrome Policy for orgunits:%s", d.Id())
	return nil
}

func resourceChromePolicyDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePoliciesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	log.Printf("[DEBUG] Deleting Chrome Policy for orgunits:%s", d.Id())

	// Check if we have additional_target_keys
	additionalTargetKeysRaw, hasAdditionalKeys := d.GetOk("additional_target_keys")

	if !hasAdditionalKeys {
		// No additional target keys - batch all policies together
		policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
			TargetResource: "orgunits/" + d.Id(),
		}

		var requests []*chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest
		for _, p := range d.Get("policies").([]interface{}) {
			policy := p.(map[string]interface{})
			schemaName := policy["schema_name"].(string)
			requests = append(requests, &chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest{
				PolicyTargetKey: policyTargetKey,
				PolicySchema:    schemaName,
			})
		}

		err := retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
			_, retryErr := chromePoliciesService.Orgunits.BatchInherit(fmt.Sprintf("customers/%s", client.Customer), &chromepolicy.GoogleChromePolicyVersionsV1BatchInheritOrgUnitPoliciesRequest{Requests: requests}).Do()
			return retryErr
		})
		if err != nil {
			if isApiErrorWithCode(err, 400) && isNonFatalDeleteError(err) {
				log.Printf("[DEBUG] Ignoring non-fatal 400 error during OU policy deletion: %v", err)
			} else {
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

		log.Printf("[DEBUG] Grouped additional_target_keys into %d groups", len(keyGroups))

		// For each unique target_key, batch all policies for each unique target_value
		for targetKey, keyValuePairs := range keyGroups {
			log.Printf("[DEBUG] Processing target_key: %s with %d target_values", targetKey, len(keyValuePairs))

			for _, keyValuePair := range keyValuePairs {
				log.Printf("[DEBUG] Batching policies for deletion: target_key=%s, target_value=%s", keyValuePair["key"], keyValuePair["value"])

				policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
					TargetResource: "orgunits/" + d.Id(),
					AdditionalTargetKeys: map[string]string{
						keyValuePair["key"]: keyValuePair["value"],
					},
				}

				var requests []*chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest
				for _, p := range d.Get("policies").([]interface{}) {
					policy := p.(map[string]interface{})
					schemaName := policy["schema_name"].(string)
					requests = append(requests, &chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest{
						PolicyTargetKey: policyTargetKey,
						PolicySchema:    schemaName,
					})
				}

				log.Printf("[DEBUG] Making BatchInherit call for deletion: target_key=%s, target_value=%s with %d policies", keyValuePair["key"], keyValuePair["value"], len(requests))

				err := retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
					_, retryErr := chromePoliciesService.Orgunits.BatchInherit(fmt.Sprintf("customers/%s", client.Customer), &chromepolicy.GoogleChromePolicyVersionsV1BatchInheritOrgUnitPoliciesRequest{Requests: requests}).Do()
					return retryErr
				})
				if err != nil {
					if isApiErrorWithCode(err, 400) && isNonFatalDeleteError(err) {
						log.Printf("[DEBUG] Ignoring non-fatal 400 error during OU policy deletion for %s=%s: %v", keyValuePair["key"], keyValuePair["value"], err)
					} else {
						return diag.FromErr(err)
					}
				}
			}
		}
	}

	log.Printf("[DEBUG] Finished deleting Chrome Policy for orgunits:%s", d.Id())
	return nil
}

func resourceChromePolicyImport(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	parts := strings.Split(d.Id(), "/")
	if len(parts) < 2 || len(parts) > 3 {
		return nil, fmt.Errorf("invalid import ID format, expected '<org_unit_id>/<schemas>' or '<org_unit_id>/<additional_keys>/<schemas>', got: %s", d.Id())
	}

	orgUnitId := strings.TrimPrefix(parts[0], "id:")
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

	expectedTargetResource := "orgunits/" + orgUnitId
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
		err := retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
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
		// Check if the policy is explicitly set on THIS target or inherited from a parent OU.
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

	d.SetId(orgUnitId)
	d.Set("org_unit_id", orgUnitId)

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

	log.Printf("[DEBUG] Import Chrome Policy for %s with %d schemas", expectedTargetResource, len(schemaNames))

	return []*schema.ResourceData{d}, nil
}

// Chrome Policies

func validateChromePolicies(ctx context.Context, d *schema.ResourceData, client *apiClient) diag.Diagnostics {
	var diags diag.Diagnostics

	new := d.Get("policies")

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePolicySchemasService, diags := GetChromePolicySchemasService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	// Validate config against schemas
	for _, policy := range new.([]interface{}) {
		schemaName := policy.(map[string]interface{})["schema_name"].(string)

		var schemaDef *chromepolicy.GoogleChromePolicyVersionsV1PolicySchema
		err := retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
			var retryErr error

			schemaDef, retryErr = chromePolicySchemasService.Get(fmt.Sprintf("customers/%s/policySchemas/%s", client.Customer, schemaName)).Do()
			return retryErr
		})
		if err != nil {
			return diag.FromErr(err)
		}

		if schemaDef == nil || schemaDef.Definition == nil || schemaDef.Definition.MessageType == nil {
			return append(diags, diag.Diagnostic{
				Summary:  fmt.Sprintf("schema definition (%s) is empty", schemaName),
				Severity: diag.Error,
			})
		}

		schemaFieldMap := map[string]*chromepolicy.Proto2FieldDescriptorProto{}
		for _, schemaField := range schemaDef.Definition.MessageType {
			for i, schemaNestedField := range schemaField.Field {
				schemaFieldMap[schemaNestedField.Name] = schemaField.Field[i]
			}
		}

		policyDef := policy.(map[string]interface{})["schema_values"].(map[string]interface{})

		for polKey, polJsonVal := range policyDef {
			if _, ok := schemaFieldMap[polKey]; !ok {
				return append(diags, diag.Diagnostic{
					Summary:  fmt.Sprintf("field name (%s) is not found in this schema definition (%s)", polKey, schemaName),
					Severity: diag.Error,
				})
			}

			var polVal interface{}
			err := json.Unmarshal([]byte(polJsonVal.(string)), &polVal)
			if err != nil {
				return diag.FromErr(err)
			}

			schemaField := schemaFieldMap[polKey]
			if schemaField == nil {
				return append(diags, diag.Diagnostic{
					Summary:  fmt.Sprintf("field type is not defined for field name (%s)", polKey),
					Severity: diag.Warning,
				})
			}

			if schemaField.Label == "LABEL_REPEATED" {
				polValType := reflect.ValueOf(polVal).Kind()
				if !((polValType == reflect.Array) || (polValType == reflect.Slice)) {
					return append(diags, diag.Diagnostic{
						Summary:  fmt.Sprintf("value provided for %s is of incorrect type %v (expected type: []%v)", schemaField.Name, polValType, schemaField.Type),
						Severity: diag.Error,
					})
				} else {
					if polValArray, ok := polVal.([]interface{}); ok {
						for _, polValItem := range polValArray {
							if !validatePolicyFieldValueType(schemaField.Type, polValItem) {
								return append(diags, diag.Diagnostic{
									Summary:  fmt.Sprintf("array value %v provided for %s is of incorrect type (expected type: %s)", polValItem, schemaField.Name, schemaField.Type),
									Severity: diag.Error,
								})
							}
						}
					}
				}
			} else {
				if !validatePolicyFieldValueType(schemaField.Type, polVal) {
					return append(diags, diag.Diagnostic{
						Summary:  fmt.Sprintf("value %v provided for %s is of incorrect type (expected type: %s)", polVal, schemaField.Name, schemaField.Type),
						Severity: diag.Error,
					})
				}
			}
		}

		if _, ok := d.GetOk("additional_target_keys"); ok {
			if schemaDef.AdditionalTargetKeyNames == nil {
				return append(diags, diag.Diagnostic{
					Summary:  fmt.Sprintf("schema defintion (%s) does not support additional target key names", schemaName),
					Severity: diag.Error,
				})
			}

			additionalTargetKeyNames := map[string]string{}
			for _, targetKeyName := range schemaDef.AdditionalTargetKeyNames {
				additionalTargetKeyNames[targetKeyName.Key] = targetKeyName.KeyDescription
			}

			additionalTargetKeys := expandChromePoliciesAdditionalTargetKeys(d.Get("additional_target_keys").([]interface{}))
			for additionalTargetKeyName := range additionalTargetKeys {
				if _, ok := additionalTargetKeyNames[additionalTargetKeyName]; !ok {
					return append(diags, diag.Diagnostic{
						Summary:  fmt.Sprintf("additional target key name (%s) is not found in this schema definition (%s)", additionalTargetKeyName, schemaName),
						Severity: diag.Error,
					})
				}
			}
		} else if schemaDef.AdditionalTargetKeyNames != nil {
			return append(diags, diag.Diagnostic{
				Summary:  fmt.Sprintf("additional target key names are required by this schema definition (%s)", schemaName),
				Severity: diag.Error,
			})
		}
	}

	return nil
}

// This will take a value and validate whether the type is correct
func validatePolicyFieldValueType(fieldType string, fieldValue interface{}) bool {
	valid := false

	switch fieldType {
	case "TYPE_BOOL":
		valid = reflect.ValueOf(fieldValue).Kind() == reflect.Bool
	case "TYPE_FLOAT":
		fallthrough
	case "TYPE_DOUBLE":
		valid = reflect.ValueOf(fieldValue).Kind() == reflect.Float64
	case "TYPE_INT64":
		fallthrough
	case "TYPE_FIXED64":
		fallthrough
	case "TYPE_SFIXED64":
		fallthrough
	case "TYPE_SINT64":
		fallthrough
	case "TYPE_UINT64":
		// this is unmarshalled as a float, check that it's an int
		if reflect.ValueOf(fieldValue).Kind() == reflect.Float64 &&
			fieldValue == float64(int(fieldValue.(float64))) {
			valid = true
		}
	case "TYPE_INT32":
		fallthrough
	case "TYPE_FIXED32":
		fallthrough
	case "TYPE_SFIXED32":
		fallthrough
	case "TYPE_SINT32":
		fallthrough
	case "TYPE_UINT32":
		// this is unmarshalled as a float, check that it's an int
		if reflect.ValueOf(fieldValue).Kind() == reflect.Float32 &&
			fieldValue == float32(int(fieldValue.(float32))) {
			valid = true
		}
	case "TYPE_MESSAGE":
		valid = reflect.ValueOf(fieldValue).Kind() == reflect.Map
		// TODO we should probably recursively ensure the type is correct
	case "TYPE_ENUM":
		fallthrough
	case "TYPE_STRING":
		fallthrough
	default:
		valid = reflect.ValueOf(fieldValue).Kind() == reflect.String
	}

	return valid
}

// The API returns numeric values as strings. This will convert it to the appropriate type
func convertPolicyFieldValueType(fieldType string, fieldValue interface{}) (interface{}, error) {
	// If it's not of type string, then we'll assume it's the right type
	if reflect.ValueOf(fieldValue).Kind() != reflect.String {
		return fieldValue, nil
	}

	var err error
	var value interface{}

	switch fieldType {
	case "TYPE_BOOL":
		value, err = strconv.ParseBool(fieldValue.(string))
	case "TYPE_FLOAT":
		fallthrough
	case "TYPE_DOUBLE":
		value, err = strconv.ParseFloat(fieldValue.(string), 64)
	case "TYPE_INT64":
		fallthrough
	case "TYPE_FIXED64":
		fallthrough
	case "TYPE_SFIXED64":
		fallthrough
	case "TYPE_SINT64":
		fallthrough
	case "TYPE_UINT64":
		value, err = strconv.ParseInt(fieldValue.(string), 10, 64)
	case "TYPE_INT32":
		fallthrough
	case "TYPE_FIXED32":
		fallthrough
	case "TYPE_SFIXED32":
		fallthrough
	case "TYPE_SINT32":
		fallthrough
	case "TYPE_UINT32":
		value, err = strconv.ParseInt(fieldValue.(string), 10, 32)
	case "TYPE_ENUM":
		fallthrough
	case "TYPE_MESSAGE":
		fallthrough
	case "TYPE_STRING":
		fallthrough
	default:
		value = fieldValue
	}

	return value, err
}

func expandChromePoliciesValues(policies []interface{}) ([]*chromepolicy.GoogleChromePolicyVersionsV1PolicyValue, diag.Diagnostics) {
	var diags diag.Diagnostics
	result := []*chromepolicy.GoogleChromePolicyVersionsV1PolicyValue{}

	for _, p := range policies {
		policy := p.(map[string]interface{})

		schemaName := policy["schema_name"].(string)
		schemaValues := policy["schema_values"].(map[string]interface{})

		policyValuesObj := make(map[string]interface{}, len(schemaValues))

		for k, v := range schemaValues {
			// Try to parse as JSON first
			if strVal, ok := v.(string); ok {
				var jsonVal interface{}
				if err := json.Unmarshal([]byte(strVal), &jsonVal); err == nil {
					// Successfully parsed as JSON
					policyValuesObj[k] = jsonVal
					continue
				}
				// If it's not valid JSON, use the string value directly
				policyValuesObj[k] = strVal
			} else {
				// For non-string values, use them as-is
				policyValuesObj[k] = v
			}
		}

		// Marshal the entire policy value object
		schemaValuesJson, err := json.Marshal(policyValuesObj)
		if err != nil {
			return nil, diag.FromErr(fmt.Errorf("failed to marshal policy values for schema %s: %v", schemaName, err))
		}

		policyValue := &chromepolicy.GoogleChromePolicyVersionsV1PolicyValue{
			PolicySchema: schemaName,
			Value:        schemaValuesJson,
		}

		result = append(result, policyValue)
	}

	return result, diags
}

func expandChromePoliciesAdditionalTargetKeys(keys []interface{}) map[string]string {
	result := map[string]string{}

	for _, k := range keys {
		targetKeyDef := k.(map[string]interface{})
		targetKeyName := targetKeyDef["target_key"].(string)
		targetKeyValue := targetKeyDef["target_value"].(string)
		result[targetKeyName] = targetKeyValue
	}

	return result
}

func flattenChromePolicies(ctx context.Context, policiesObj []*chromepolicy.GoogleChromePolicyVersionsV1PolicyValue, client *apiClient) ([]map[string]interface{}, diag.Diagnostics) {
	var policies []map[string]interface{}

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return nil, diags
	}

	schemaService, diags := GetChromePolicySchemasService(chromePolicyService)
	if diags.HasError() {
		return nil, diags
	}

	for _, polObj := range policiesObj {
		var schemaDef *chromepolicy.GoogleChromePolicyVersionsV1PolicySchema
		err := retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
			var retryErr error

			schemaDef, retryErr = schemaService.Get(fmt.Sprintf("customers/%s/policySchemas/%s", client.Customer, polObj.PolicySchema)).Do()
			return retryErr
		})
		if err != nil {
			return nil, diag.FromErr(err)
		}

		if schemaDef == nil || schemaDef.Definition == nil || schemaDef.Definition.MessageType == nil {
			return nil, append(diags, diag.Diagnostic{
				Summary:  fmt.Sprintf("schema definition (%s) is not defined", polObj.PolicySchema),
				Severity: diag.Warning,
			})
		}

		schemaFieldMap := map[string]*chromepolicy.Proto2FieldDescriptorProto{}
		for _, schemaField := range schemaDef.Definition.MessageType {
			for i, schemaNestedField := range schemaField.Field {
				schemaFieldMap[schemaNestedField.Name] = schemaField.Field[i]
			}
		}

		var schemaValuesObj map[string]interface{}

		err = json.Unmarshal(polObj.Value, &schemaValuesObj)
		if err != nil {
			return nil, diag.FromErr(err)
		}

		schemaValues := map[string]interface{}{}
		for k, v := range schemaValuesObj {
			if _, ok := schemaFieldMap[k]; !ok {
				return nil, append(diags, diag.Diagnostic{
					Summary:  fmt.Sprintf("field name (%s) is not found in this schema definition (%s)", k, polObj.PolicySchema),
					Severity: diag.Warning,
				})
			}

			schemaField := schemaFieldMap[k]
			if schemaField == nil {
				return nil, append(diags, diag.Diagnostic{
					Summary:  fmt.Sprintf("field type is not defined for field name (%s)", k),
					Severity: diag.Warning,
				})
			}

			val, err := convertPolicyFieldValueType(schemaField.Type, v)
			if err != nil {
				return nil, diag.FromErr(err)
			}

			jsonVal, err := json.Marshal(val)
			if err != nil {
				return nil, diag.FromErr(err)
			}
			schemaValues[k] = string(jsonVal)
		}

		policies = append(policies, map[string]interface{}{
			"schema_name":   polObj.PolicySchema,
			"schema_values": schemaValues,
		})
	}

	return policies, nil
}
