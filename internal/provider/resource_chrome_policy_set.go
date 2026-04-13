package googleworkspace

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"google.golang.org/api/chromepolicy/v1"
)

type chromePolicyTargetKind string

const (
	targetOrgUnit chromePolicyTargetKind = "orgunits"
	targetGroup   chromePolicyTargetKind = "groups"
)

func resourceChromePolicySet() *schema.Resource {
	return &schema.Resource{
		Description: "Authoritative Chrome Policy Set resource that manages all Chrome policies " +
			"matching a schema filter for a given target (org unit or group). Any directly-set " +
			"policies in the scope that are not declared in config will be removed (inherited).",

		CreateContext: resourceChromePolicySetCreate,
		ReadContext:   resourceChromePolicySetRead,
		UpdateContext: resourceChromePolicySetUpdate,
		DeleteContext: resourceChromePolicySetDelete,

		CustomizeDiff: chromePolicySetCustomizeDiff,

		Importer: &schema.ResourceImporter{
			StateContext: resourceChromePolicySetImport,
		},

		Schema: map[string]*schema.Schema{
			"org_unit_id": {
				Description:   "The target org unit on which policies are applied.",
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"group_id"},
				DiffSuppressFunc: diffSuppressOrgUnitId,
			},
			"group_id": {
				Description:   "The target group on which policies are applied.",
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"org_unit_id"},
			},
			"policy_schema_filter": {
				Description: "The schema filter defining the authoritative scope, e.g. " +
					"`chrome.users.*` or `chrome.users.apps.*`. Supports wildcards at the leaf " +
					"level. If omitted, inferred from the policy blocks (all must share the same " +
					"`schema_name`).",
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},
			"policy": {
				Description: "Set of policies to enforce. If empty, all directly-set policies " +
					"in the scope are removed (inherited).",
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"schema_name": {
							Description: "The full qualified name of the policy schema.",
							Type:        schema.TypeString,
							Required:    true,
						},
						"schema_values": {
							Description: "JSON encoded map of key/value pairs for the policy.",
							Type:        schema.TypeMap,
							Required:    true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
								ValidateDiagFunc: validation.ToDiagFunc(
									validation.StringIsJSON,
								),
							},
						},
						"additional_target_keys": {
							Description: "Additional target keys for this policy, e.g. " +
								"`{app_id = \"chrome:...\"}` for app-scoped policies.",
							Type:     schema.TypeMap,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
					},
				},
				Set: chromePolicySetHash,
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Hash function for TypeSet: identity is (schema_name, additional_target_keys)
// ---------------------------------------------------------------------------

func chromePolicySetHash(v interface{}) int {
	m := v.(map[string]interface{})
	schemaName := m["schema_name"].(string)

	var additionalKeys string
	if atk, ok := m["additional_target_keys"]; ok && atk != nil {
		additionalKeys = canonicalAdditionalTargetKeys(atk.(map[string]interface{}))
	}

	raw := schemaName + "\x00" + additionalKeys
	return int(crc32.ChecksumIEEE([]byte(raw)))
}

// canonicalAdditionalTargetKeys produces a deterministic string representation
// of additional target keys for use in hashing and identity comparison.
func canonicalAdditionalTargetKeys(keys map[string]interface{}) string {
	if len(keys) == 0 {
		return ""
	}
	sorted := make([]string, 0, len(keys))
	for k, v := range keys {
		sorted = append(sorted, k+"="+v.(string))
	}
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

// ---------------------------------------------------------------------------
// policyIdentity is the unique key for a policy within a resource.
// ---------------------------------------------------------------------------

type policyIdentity struct {
	SchemaName           string
	AdditionalTargetKeys map[string]string
}

func (p policyIdentity) key() string {
	if len(p.AdditionalTargetKeys) == 0 {
		return p.SchemaName
	}
	sorted := make([]string, 0, len(p.AdditionalTargetKeys))
	for k, v := range p.AdditionalTargetKeys {
		sorted = append(sorted, k+"="+v)
	}
	sort.Strings(sorted)
	return p.SchemaName + "\x00" + strings.Join(sorted, ",")
}

func identityFromPolicy(p map[string]interface{}) policyIdentity {
	id := policyIdentity{
		SchemaName: p["schema_name"].(string),
	}
	if atk, ok := p["additional_target_keys"]; ok && atk != nil {
		raw := atk.(map[string]interface{})
		if len(raw) > 0 {
			id.AdditionalTargetKeys = make(map[string]string, len(raw))
			for k, v := range raw {
				id.AdditionalTargetKeys[k] = v.(string)
			}
		}
	}
	return id
}

func identityFromResolved(rp *chromepolicy.GoogleChromePolicyVersionsV1ResolvedPolicy) policyIdentity {
	id := policyIdentity{
		SchemaName: rp.Value.PolicySchema,
	}
	if rp.TargetKey != nil && len(rp.TargetKey.AdditionalTargetKeys) > 0 {
		id.AdditionalTargetKeys = rp.TargetKey.AdditionalTargetKeys
	}
	return id
}

// ---------------------------------------------------------------------------
// CustomizeDiff
// ---------------------------------------------------------------------------

func chromePolicySetCustomizeDiff(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
	hasOU := d.Get("org_unit_id").(string) != ""
	hasGroup := d.Get("group_id").(string) != ""
	if !hasOU && !hasGroup {
		return fmt.Errorf("one of `org_unit_id` or `group_id` must be set")
	}

	filter := d.Get("policy_schema_filter").(string)
	rawPolicies := d.Get("policy").(*schema.Set).List()

	// TypeSet diff computation can produce phantom entries with zero-value
	// fields; filter them out before validation.
	var policies []interface{}
	for _, p := range rawPolicies {
		if p.(map[string]interface{})["schema_name"].(string) != "" {
			policies = append(policies, p)
		}
	}

	if filter == "" {
		if len(policies) == 0 {
			return fmt.Errorf("`policy_schema_filter` is required when no policy blocks are defined")
		}
		firstName := policies[0].(map[string]interface{})["schema_name"].(string)
		for _, p := range policies[1:] {
			if p.(map[string]interface{})["schema_name"].(string) != firstName {
				return fmt.Errorf("`policy_schema_filter` is required when policy blocks have different schema_name values")
			}
		}
		if err := d.SetNew("policy_schema_filter", firstName); err != nil {
			return err
		}
		filter = firstName
	}

	seen := make(map[string]bool)
	for _, p := range policies {
		pol := p.(map[string]interface{})
		schemaName := pol["schema_name"].(string)

		if !schemaNameMatchesFilter(schemaName, filter) {
			return fmt.Errorf("schema_name %q does not match policy_schema_filter %q", schemaName, filter)
		}

		id := identityFromPolicy(pol)
		k := id.key()
		if seen[k] {
			return fmt.Errorf("duplicate policy: %s", k)
		}
		seen[k] = true
	}

	return nil
}

// schemaNameMatchesFilter checks if a fully-qualified schema name matches a
// policy schema filter. The filter may be an exact name or end with ".*" to
// match any single leaf segment (no dots).
func schemaNameMatchesFilter(name, filter string) bool {
	if !strings.HasSuffix(filter, ".*") {
		return name == filter
	}
	prefix := strings.TrimSuffix(filter, "*")
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	leaf := name[len(prefix):]
	return len(leaf) > 0 && !strings.Contains(leaf, ".")
}

// ---------------------------------------------------------------------------
// Import
// ---------------------------------------------------------------------------

func resourceChromePolicySetImport(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	idParts := strings.SplitN(d.Id(), "/", 3)
	if len(idParts) != 3 {
		return nil, fmt.Errorf("expected import ID format: {orgunits|groups}/{targetId}/{schemaFilter}, got: %s", d.Id())
	}

	kind := chromePolicyTargetKind(idParts[0])
	targetID := idParts[1]
	filter := idParts[2]

	switch kind {
	case targetOrgUnit:
		if err := d.Set("org_unit_id", targetID); err != nil {
			return nil, err
		}
	case targetGroup:
		if err := d.Set("group_id", targetID); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown target kind %q; expected \"orgunits\" or \"groups\"", kind)
	}

	if err := d.Set("policy_schema_filter", filter); err != nil {
		return nil, err
	}

	d.SetId(d.Id())
	return []*schema.ResourceData{d}, nil
}

// ---------------------------------------------------------------------------
// Resolve helper: paginated wildcard resolve, filtered to directly-set policies
// ---------------------------------------------------------------------------

type resolvedPolicyEntry struct {
	Identity policyIdentity
	Value    *chromepolicy.GoogleChromePolicyVersionsV1PolicyValue
}

func resolveDirectlySetPolicies(
	ctx context.Context,
	policiesService *chromepolicy.CustomersPoliciesService,
	customer string,
	filter string,
	targetResource string,
) ([]resolvedPolicyEntry, diag.Diagnostics) {
	var result []resolvedPolicyEntry

	req := &chromepolicy.GoogleChromePolicyVersionsV1ResolveRequest{
		PolicySchemaFilter: filter,
		PolicyTargetKey: &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
			TargetResource: targetResource,
		},
		PageSize: 1000,
	}

	for {
		var resp *chromepolicy.GoogleChromePolicyVersionsV1ResolveResponse
		err := retryTimeDuration(ctx, time.Minute, func() error {
			var retryErr error
			resp, retryErr = policiesService.Resolve(
				fmt.Sprintf("customers/%s", customer), req,
			).Do()
			return retryErr
		})
		if err != nil {
			return nil, diag.FromErr(err)
		}

		for _, rp := range resp.ResolvedPolicies {
			if rp.SourceKey == nil || rp.SourceKey.TargetResource != targetResource {
				continue
			}
			result = append(result, resolvedPolicyEntry{
				Identity: identityFromResolved(rp),
				Value:    rp.Value,
			})
		}

		if resp.NextPageToken == "" {
			break
		}
		req.PageToken = resp.NextPageToken
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Schema definition caching for flatten
// ---------------------------------------------------------------------------

type schemaCache struct {
	service  *chromepolicy.CustomersPolicySchemasService
	customer string
	cache    map[string]*chromepolicy.GoogleChromePolicyVersionsV1PolicySchema
}

func newSchemaCache(service *chromepolicy.CustomersPolicySchemasService, customer string) *schemaCache {
	return &schemaCache{
		service:  service,
		customer: customer,
		cache:    make(map[string]*chromepolicy.GoogleChromePolicyVersionsV1PolicySchema),
	}
}

func (sc *schemaCache) get(ctx context.Context, schemaName string) (*chromepolicy.GoogleChromePolicyVersionsV1PolicySchema, error) {
	if cached, ok := sc.cache[schemaName]; ok {
		return cached, nil
	}

	var schemaDef *chromepolicy.GoogleChromePolicyVersionsV1PolicySchema
	err := retryTimeDuration(ctx, time.Minute, func() error {
		var retryErr error
		schemaDef, retryErr = sc.service.Get(
			fmt.Sprintf("customers/%s/policySchemas/%s", sc.customer, schemaName),
		).Do()
		return retryErr
	})
	if err != nil {
		return nil, err
	}

	sc.cache[schemaName] = schemaDef
	return schemaDef, nil
}

// ---------------------------------------------------------------------------
// Flatten: convert resolved policies to Terraform state
// ---------------------------------------------------------------------------

func flattenResolvedPolicies(entries []resolvedPolicyEntry) ([]map[string]interface{}, diag.Diagnostics) {
	var result []map[string]interface{}

	for _, entry := range entries {
		var rawValues map[string]interface{}
		if err := json.Unmarshal(entry.Value.Value, &rawValues); err != nil {
			return nil, diag.FromErr(err)
		}

		schemaValues := make(map[string]interface{}, len(rawValues))
		for k, v := range rawValues {
			jsonVal, err := json.Marshal(v)
			if err != nil {
				return nil, diag.FromErr(err)
			}
			schemaValues[k] = string(jsonVal)
		}

		flat := map[string]interface{}{
			"schema_name":   entry.Value.PolicySchema,
			"schema_values": schemaValues,
		}
		if len(entry.Identity.AdditionalTargetKeys) > 0 {
			flat["additional_target_keys"] = flattenAdditionalTargetKeys(entry.Identity.AdditionalTargetKeys)
		}
		result = append(result, flat)
	}

	return result, nil
}

func flattenAdditionalTargetKeys(keys map[string]string) map[string]interface{} {
	if len(keys) == 0 {
		return map[string]interface{}{}
	}
	result := make(map[string]interface{}, len(keys))
	for k, v := range keys {
		result[k] = v
	}
	return result
}

func buildSchemaFieldMap(schemaDef *chromepolicy.GoogleChromePolicyVersionsV1PolicySchema) map[string]*chromepolicy.Proto2FieldDescriptorProto {
	fieldMap := make(map[string]*chromepolicy.Proto2FieldDescriptorProto)
	for _, mt := range schemaDef.Definition.MessageType {
		for i, f := range mt.Field {
			fieldMap[f.Name] = mt.Field[i]
		}
	}
	return fieldMap
}

// ---------------------------------------------------------------------------
// Validation: check policy values against schema definitions
// ---------------------------------------------------------------------------

func validatePolicySetPolicies(
	ctx context.Context,
	policies []interface{},
	sc *schemaCache,
) diag.Diagnostics {
	for _, p := range policies {
		pol := p.(map[string]interface{})
		schemaName := pol["schema_name"].(string)
		schemaValues := pol["schema_values"].(map[string]interface{})

		schemaDef, err := sc.get(ctx, schemaName)
		if err != nil {
			return diag.FromErr(err)
		}

		if schemaDef == nil || schemaDef.Definition == nil || schemaDef.Definition.MessageType == nil {
			return diag.Errorf("schema definition (%s) is empty", schemaName)
		}

		fieldMap := buildSchemaFieldMap(schemaDef)

		for fieldName, jsonVal := range schemaValues {
			field, ok := fieldMap[fieldName]
			if !ok {
				return diag.Errorf("field %q is not found in schema %s", fieldName, schemaName)
			}

			var val interface{}
			if err := json.Unmarshal([]byte(jsonVal.(string)), &val); err != nil {
				return diag.FromErr(err)
			}

			if field.Label == "LABEL_REPEATED" {
				arr, ok := val.([]interface{})
				if !ok {
					return diag.Errorf("value for %s.%s must be an array (got %T)", schemaName, fieldName, val)
				}
				for _, item := range arr {
					if !validatePolicyFieldValueType(field.Type, item) {
						return diag.Errorf("array element in %s.%s has incorrect type (expected %s)", schemaName, fieldName, field.Type)
					}
				}
			} else if !validatePolicyFieldValueType(field.Type, val) {
				return diag.Errorf("value for %s.%s has incorrect type (expected %s)", schemaName, fieldName, field.Type)
			}
		}

		additionalKeys := identityFromPolicy(pol).AdditionalTargetKeys
		if schemaDef.AdditionalTargetKeyNames != nil && len(additionalKeys) == 0 {
			return diag.Errorf("schema %s requires additional_target_keys", schemaName)
		}
		if schemaDef.AdditionalTargetKeyNames == nil && len(additionalKeys) > 0 {
			return diag.Errorf("schema %s does not support additional_target_keys", schemaName)
		}

		if len(additionalKeys) > 0 {
			allowed := make(map[string]bool)
			for _, tkn := range schemaDef.AdditionalTargetKeyNames {
				allowed[tkn.Key] = true
			}
			for k := range additionalKeys {
				if !allowed[k] {
					return diag.Errorf("additional_target_key %q is not valid for schema %s", k, schemaName)
				}
			}
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Build PolicyTargetKey for a policy block
// ---------------------------------------------------------------------------

func buildPolicyTargetKey(targetResource string, pol map[string]interface{}) *chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey {
	key := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
		TargetResource: targetResource,
	}
	if atk, ok := pol["additional_target_keys"]; ok && atk != nil {
		raw := atk.(map[string]interface{})
		if len(raw) > 0 {
			key.AdditionalTargetKeys = make(map[string]string, len(raw))
			for k, v := range raw {
				key.AdditionalTargetKeys[k] = v.(string)
			}
		}
	}
	return key
}

// ---------------------------------------------------------------------------
// Determine target kind and resource string from ResourceData
// ---------------------------------------------------------------------------

func chromePolicySetTarget(d *schema.ResourceData) (chromePolicyTargetKind, string) {
	if v, ok := d.GetOk("org_unit_id"); ok {
		targetID := strings.TrimPrefix(v.(string), "id:")
		return targetOrgUnit, string(targetOrgUnit) + "/" + targetID
	}
	if v, ok := d.GetOk("group_id"); ok {
		return targetGroup, string(targetGroup) + "/" + v.(string)
	}
	return "", ""
}

func chromePolicySetResourceID(kind chromePolicyTargetKind, targetID, filter string) string {
	return string(kind) + "/" + targetID + "/" + filter
}

// ---------------------------------------------------------------------------
// CRUD: Create
// ---------------------------------------------------------------------------

func resourceChromePolicySetCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)
	kind, targetResource := chromePolicySetTarget(d)
	filter := d.Get("policy_schema_filter").(string)
	targetID := strings.TrimPrefix(targetResource, string(kind)+"/")

	log.Printf("[DEBUG] Creating Chrome Policy Set for %s (filter: %s)", targetResource, filter)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	policiesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	schemasService, diags := GetChromePolicySchemasService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	sc := newSchemaCache(schemasService, client.Customer)

	policies := d.Get("policy").(*schema.Set).List()

	if len(policies) > 0 {
		diags = validatePolicySetPolicies(ctx, policies, sc)
		if diags.HasError() {
			return diags
		}

		if diags := batchModifyPolicies(ctx, policiesService, client.Customer, targetResource, policies); diags.HasError() {
			return diags
		}
	}

	d.SetId(chromePolicySetResourceID(kind, targetID, filter))

	return resourceChromePolicySetRead(ctx, d, meta)
}

// ---------------------------------------------------------------------------
// CRUD: Read
// ---------------------------------------------------------------------------

func resourceChromePolicySetRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)
	_, targetResource := chromePolicySetTarget(d)
	filter := d.Get("policy_schema_filter").(string)

	log.Printf("[DEBUG] Reading Chrome Policy Set for %s (filter: %s)", targetResource, filter)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	policiesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	entries, diags := resolveDirectlySetPolicies(ctx, policiesService, client.Customer, filter, targetResource)
	if diags.HasError() {
		return diags
	}

	flatPolicies, diags := flattenResolvedPolicies(entries)
	if diags.HasError() {
		return diags
	}

	if err := d.Set("policy", flatPolicies); err != nil {
		return diag.FromErr(err)
	}

	log.Printf("[DEBUG] Finished reading Chrome Policy Set for %s: %d directly-set policies", targetResource, len(flatPolicies))
	return nil
}

// ---------------------------------------------------------------------------
// CRUD: Update
// ---------------------------------------------------------------------------

func resourceChromePolicySetUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)
	_, targetResource := chromePolicySetTarget(d)
	filter := d.Get("policy_schema_filter").(string)

	log.Printf("[DEBUG] Updating Chrome Policy Set for %s (filter: %s)", targetResource, filter)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	policiesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	schemasService, diags := GetChromePolicySchemasService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	sc := newSchemaCache(schemasService, client.Customer)

	oldRaw, newRaw := d.GetChange("policy")
	oldPolicies := oldRaw.(*schema.Set).List()
	newPolicies := newRaw.(*schema.Set).List()

	if len(newPolicies) > 0 {
		diags = validatePolicySetPolicies(ctx, newPolicies, sc)
		if diags.HasError() {
			return diags
		}
	}

	// Build lookup of new policies by identity
	newByKey := make(map[string]bool, len(newPolicies))
	for _, p := range newPolicies {
		id := identityFromPolicy(p.(map[string]interface{}))
		newByKey[id.key()] = true
	}

	// Inherit policies that are in old state but not in new config
	var inheritRequests []*chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest
	for _, p := range oldPolicies {
		pol := p.(map[string]interface{})
		id := identityFromPolicy(pol)
		if !newByKey[id.key()] {
			inheritRequests = append(inheritRequests, &chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest{
				PolicyTargetKey: buildPolicyTargetKey(targetResource, pol),
				PolicySchema:    id.SchemaName,
			})
		}
	}

	if len(inheritRequests) > 0 {
		err := retryTimeDuration(ctx, time.Minute, func() error {
			_, retryErr := policiesService.Orgunits.BatchInherit(
				fmt.Sprintf("customers/%s", client.Customer),
				&chromepolicy.GoogleChromePolicyVersionsV1BatchInheritOrgUnitPoliciesRequest{Requests: inheritRequests},
			).Do()
			return retryErr
		})
		if err != nil {
			return diag.FromErr(err)
		}
	}

	// Modify all policies in new config
	if len(newPolicies) > 0 {
		if diags := batchModifyPolicies(ctx, policiesService, client.Customer, targetResource, newPolicies); diags.HasError() {
			return diags
		}
	}

	log.Printf("[DEBUG] Finished updating Chrome Policy Set for %s", targetResource)
	return resourceChromePolicySetRead(ctx, d, meta)
}

// ---------------------------------------------------------------------------
// CRUD: Delete
// ---------------------------------------------------------------------------

func resourceChromePolicySetDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)
	_, targetResource := chromePolicySetTarget(d)
	filter := d.Get("policy_schema_filter").(string)

	log.Printf("[DEBUG] Deleting Chrome Policy Set for %s (filter: %s)", targetResource, filter)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	policiesService, diags := GetChromePoliciesService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	// Fresh resolve to catch any policies set since last read
	entries, diags := resolveDirectlySetPolicies(ctx, policiesService, client.Customer, filter, targetResource)
	if diags.HasError() {
		return diags
	}

	if len(entries) == 0 {
		return nil
	}

	var requests []*chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest
	for _, entry := range entries {
		targetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
			TargetResource:     targetResource,
			AdditionalTargetKeys: entry.Identity.AdditionalTargetKeys,
		}
		requests = append(requests, &chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest{
			PolicyTargetKey: targetKey,
			PolicySchema:    entry.Identity.SchemaName,
		})
	}

	err := retryTimeDuration(ctx, time.Minute, func() error {
		_, retryErr := policiesService.Orgunits.BatchInherit(
			fmt.Sprintf("customers/%s", client.Customer),
			&chromepolicy.GoogleChromePolicyVersionsV1BatchInheritOrgUnitPoliciesRequest{Requests: requests},
		).Do()
		return retryErr
	})
	if err != nil {
		return diag.FromErr(err)
	}

	log.Printf("[DEBUG] Finished deleting Chrome Policy Set for %s: inherited %d policies", targetResource, len(requests))
	return nil
}

// ---------------------------------------------------------------------------
// Shared: BatchModify a list of policy blocks
// ---------------------------------------------------------------------------

func batchModifyPolicies(
	ctx context.Context,
	policiesService *chromepolicy.CustomersPoliciesService,
	customer string,
	targetResource string,
	policies []interface{},
) diag.Diagnostics {
	var requests []*chromepolicy.GoogleChromePolicyVersionsV1ModifyOrgUnitPolicyRequest

	for _, p := range policies {
		pol := p.(map[string]interface{})
		schemaName := pol["schema_name"].(string)
		schemaValues := pol["schema_values"].(map[string]interface{})

		valuesObj := make(map[string]interface{}, len(schemaValues))
		var updateKeys []string
		for k, v := range schemaValues {
			var parsed interface{}
			if err := json.Unmarshal([]byte(v.(string)), &parsed); err != nil {
				return diag.FromErr(err)
			}
			valuesObj[k] = parsed
			updateKeys = append(updateKeys, k)
		}

		valueJSON, err := json.Marshal(valuesObj)
		if err != nil {
			return diag.FromErr(err)
		}

		requests = append(requests, &chromepolicy.GoogleChromePolicyVersionsV1ModifyOrgUnitPolicyRequest{
			PolicyTargetKey: buildPolicyTargetKey(targetResource, pol),
			PolicyValue: &chromepolicy.GoogleChromePolicyVersionsV1PolicyValue{
				PolicySchema: schemaName,
				Value:        valueJSON,
			},
			UpdateMask: strings.Join(updateKeys, ","),
		})
	}

	err := retryTimeDuration(ctx, time.Minute, func() error {
		_, retryErr := policiesService.Orgunits.BatchModify(
			fmt.Sprintf("customers/%s", customer),
			&chromepolicy.GoogleChromePolicyVersionsV1BatchModifyOrgUnitPoliciesRequest{Requests: requests},
		).Do()
		return retryErr
	})
	if err != nil {
		return diag.FromErr(err)
	}

	return nil
}
