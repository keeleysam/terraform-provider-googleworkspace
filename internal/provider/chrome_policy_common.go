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
	"google.golang.org/api/chromepolicy/v1"
)

// ---------------------------------------------------------------------------
// Target kind
// ---------------------------------------------------------------------------

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
// Additional target keys helper (was duplicated 5 times)
// ---------------------------------------------------------------------------

// groupAdditionalTargetKeys parses the additional_target_keys list attribute
// and groups entries by their target_key name. Each entry in the returned map
// value is a {key, value} pair.
func groupAdditionalTargetKeys(raw []interface{}) map[string][]map[string]string {
	keyGroups := make(map[string][]map[string]string)
	for _, k := range raw {
		def := k.(map[string]interface{})
		name := def["target_key"].(string)
		value := def["target_value"].(string)
		keyGroups[name] = append(keyGroups[name], map[string]string{
			"key":   name,
			"value": value,
		})
	}
	return keyGroups
}

// ---------------------------------------------------------------------------
// Create (shared)
// ---------------------------------------------------------------------------

// chromePolicyBatchModifyFunc executes a batch-modify API call. The callback
// receives the policies service, customer ID, target key, and for each policy
// its value and update mask. The implementation constructs the correct
// request type (OrgUnit vs Group) and calls the appropriate endpoint.
type chromePolicyBatchModifyFunc func(
	ctx context.Context,
	chromePoliciesService *chromepolicy.CustomersPoliciesService,
	customer string,
	targetKey *chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey,
	policies []*chromepolicy.GoogleChromePolicyVersionsV1PolicyValue,
	updateMasks []string,
) error

// chromePolicyCreateCommon implements the shared Create flow for both org-unit
// and group chrome policy resources. It validates, expands, groups by
// additional target keys, then calls the provided batchModify callback for
// each batch. After writing, it sets the resource ID and calls readFunc.
func chromePolicyCreateCommon(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
	kind chromePolicyTargetKind,
	batchModify chromePolicyBatchModifyFunc,
	readFunc func(context.Context, *schema.ResourceData, interface{}) diag.Diagnostics,
) diag.Diagnostics {
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePoliciesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	targetID := chromePolicyTargetID(d, kind)
	targetResource := chromePolicyTargetResource(kind, targetID)

	log.Printf("[DEBUG] Creating Chrome Policy for %s", targetResource)

	diags = validateChromePolicies(ctx, d, client)
	if diags.HasError() {
		return diags
	}

	policies, diags := expandChromePoliciesValues(d.Get("policies").([]interface{}))
	if diags.HasError() {
		return diags
	}

	// Build update masks for each policy (the set of field names being modified).
	updateMasks := make([]string, len(policies))
	for i, p := range policies {
		var schemaValues map[string]interface{}
		if err := json.Unmarshal(p.Value, &schemaValues); err != nil {
			return diag.FromErr(err)
		}
		var keys []string
		for key := range schemaValues {
			keys = append(keys, key)
		}
		updateMasks[i] = strings.Join(keys, ",")
	}

	additionalTargetKeysRaw, hasAdditionalKeys := d.GetOk("additional_target_keys")

	if !hasAdditionalKeys {
		// No additional_target_keys: single batch for this target.
		policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
			TargetResource: targetResource,
		}

		if err := batchModify(ctx, chromePoliciesService, client.Customer, policyTargetKey, policies, updateMasks); err != nil {
			return diag.FromErr(err)
		}
	} else {
		// Have additional_target_keys: group by target_key.
		keyGroups := groupAdditionalTargetKeys(additionalTargetKeysRaw.([]interface{}))

		log.Printf("[DEBUG] Grouped additional_target_keys by target_key: %d groups", len(keyGroups))

		for targetKeyName, keyValuePairs := range keyGroups {
			log.Printf("[DEBUG] Processing target_key group: %s with %d values", targetKeyName, len(keyValuePairs))

			for _, keyValuePair := range keyValuePairs {
				policyTargetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
					TargetResource: targetResource,
					AdditionalTargetKeys: map[string]string{
						keyValuePair["key"]: keyValuePair["value"],
					},
				}

				log.Printf("[DEBUG] Batching %d policies for %s=%s", len(policies), keyValuePair["key"], keyValuePair["value"])

				if err := batchModify(ctx, chromePoliciesService, client.Customer, policyTargetKey, policies, updateMasks); err != nil {
					return diag.FromErr(err)
				}
			}
		}
	}

	log.Printf("[DEBUG] Finished creating Chrome Policy for %s", targetResource)
	d.SetId(targetID)

	return readFunc(ctx, d, meta)
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

// ---------------------------------------------------------------------------
// Shared helpers (validation, expansion, flattening)
// ---------------------------------------------------------------------------

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
		if reflect.ValueOf(fieldValue).Kind() == reflect.Float32 &&
			fieldValue == float32(int(fieldValue.(float32))) {
			valid = true
		}
	case "TYPE_MESSAGE":
		valid = reflect.ValueOf(fieldValue).Kind() == reflect.Map
	case "TYPE_ENUM":
		fallthrough
	case "TYPE_STRING":
		fallthrough
	default:
		valid = reflect.ValueOf(fieldValue).Kind() == reflect.String
	}

	return valid
}

func convertPolicyFieldValueType(fieldType string, fieldValue interface{}) (interface{}, error) {
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
			if strVal, ok := v.(string); ok {
				var jsonVal interface{}
				if err := json.Unmarshal([]byte(strVal), &jsonVal); err == nil {
					policyValuesObj[k] = jsonVal
					continue
				}
				policyValuesObj[k] = strVal
			} else {
				policyValuesObj[k] = v
			}
		}

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
