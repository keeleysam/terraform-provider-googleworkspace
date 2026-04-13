package googleworkspace

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"google.golang.org/api/chromepolicy/v1"
)

// chromePolicyTargetKind distinguishes between org-unit and group policy targets.
type chromePolicyTargetKind string

const (
	targetOrgUnit chromePolicyTargetKind = "orgunits"
	targetGroup   chromePolicyTargetKind = "groups"
)

// chromePolicyTargetResource returns the API target resource string for the
// given kind and ID, e.g. "orgunits/abc123" or "groups/def456".
func chromePolicyTargetResource(kind chromePolicyTargetKind, id string) string {
	return string(kind) + "/" + id
}

// chromePolicyTargetID extracts the target ID from ResourceData for the given
// kind. For org units, the "id:" prefix is stripped.
func chromePolicyTargetID(d *schema.ResourceData, kind chromePolicyTargetKind) string {
	switch kind {
	case targetOrgUnit:
		return strings.TrimPrefix(d.Get("org_unit_id").(string), "id:")
	case targetGroup:
		return d.Get("group_id").(string)
	default:
		return d.Id()
	}
}

// idAttrForKind returns the schema attribute name for the target ID.
func idAttrForKind(kind chromePolicyTargetKind) string {
	switch kind {
	case targetOrgUnit:
		return "org_unit_id"
	case targetGroup:
		return "group_id"
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// Read (shared)
// ---------------------------------------------------------------------------

func chromePolicyReadCommon(ctx context.Context, d *schema.ResourceData, meta interface{}, kind chromePolicyTargetKind) diag.Diagnostics {
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePoliciesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	targetResource := chromePolicyTargetResource(kind, d.Id())
	log.Printf("[DEBUG] Getting Chrome Policy for %s", targetResource)

	policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
		TargetResource: targetResource,
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
			return handleNotFoundError(err, d, fmt.Sprintf("Chrome Policy %s", d.Id()))
		}

		if len(resp.ResolvedPolicies) == 0 {
			log.Printf("[DEBUG] No resolved policies found for schema %s - policy may have been deleted", schemaName)
			continue
		}

		if len(resp.ResolvedPolicies) != 1 {
			log.Printf("[WARN] Expected 1 resolved policy for schema %s, got %d", schemaName, len(resp.ResolvedPolicies))
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

	log.Printf("[DEBUG] Finished getting Chrome Policy for %s", targetResource)
	return nil
}

// ---------------------------------------------------------------------------
// Import (shared)
// ---------------------------------------------------------------------------

func chromePolicyImportCommon(ctx context.Context, d *schema.ResourceData, meta interface{}, kind chromePolicyTargetKind, idAttr string) ([]*schema.ResourceData, error) {
	parts := strings.Split(d.Id(), "/")
	if len(parts) < 2 || len(parts) > 3 {
		return nil, fmt.Errorf("invalid import ID format, expected '<%s>/<schemas>' or '<%s>/<additional_keys>/<schemas>', got: %s", idAttr, idAttr, d.Id())
	}

	targetID := parts[0]
	if kind == targetOrgUnit {
		targetID = strings.TrimPrefix(targetID, "id:")
	}

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

	// Strict existence validation: verify each policy is set on this target.
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return nil, fmt.Errorf("failed to create Chrome Policy service: %s", diags[0].Summary)
	}

	chromePoliciesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return nil, fmt.Errorf("failed to get Chrome Policies service: %s", diags[0].Summary)
	}

	expectedTargetResource := chromePolicyTargetResource(kind, targetID)
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
		// Check if the policy is explicitly set on THIS target or inherited.
		// Inherited policies are allowed but logged as warnings.
		sourceTarget := resp.ResolvedPolicies[0].SourceKey.TargetResource
		if sourceTarget != expectedTargetResource {
			log.Printf("[WARN] Import: policy %s on %s is inherited from %s (not explicitly set). "+
				"Terraform will manage this policy going forward.",
				schemaName, expectedTargetResource, sourceTarget,
			)
		}
	}

	d.SetId(targetID)
	d.Set(idAttr, targetID)

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
