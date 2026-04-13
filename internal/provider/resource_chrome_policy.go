package googleworkspace

import (
	"context"
	"fmt"
	"log"

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
	return chromePolicyCreateCommon(ctx, d, meta, targetOrgUnit, orgUnitBatchModify, resourceChromePolicyRead)
}

// orgUnitBatchModify builds ModifyOrgUnitPolicyRequests and calls Orgunits.BatchModify
// with all policies in a single batch.
func orgUnitBatchModify(
	ctx context.Context,
	chromePoliciesService *chromepolicy.CustomersPoliciesService,
	customer string,
	targetKey *chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey,
	policies []*chromepolicy.GoogleChromePolicyVersionsV1PolicyValue,
	updateMasks []string,
) error {
	var requests []*chromepolicy.GoogleChromePolicyVersionsV1ModifyOrgUnitPolicyRequest
	for i, p := range policies {
		requests = append(requests, &chromepolicy.GoogleChromePolicyVersionsV1ModifyOrgUnitPolicyRequest{
			PolicyTargetKey: targetKey,
			PolicyValue:     p,
			UpdateMask:      updateMasks[i],
		})
	}

	return retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
		_, retryErr := chromePoliciesService.Orgunits.BatchModify(
			fmt.Sprintf("customers/%s", customer),
			&chromepolicy.GoogleChromePolicyVersionsV1BatchModifyOrgUnitPoliciesRequest{Requests: requests},
		).Do()
		return retryErr
	})
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

	targetResource := chromePolicyTargetResource(targetOrgUnit, d.Id())
	log.Printf("[DEBUG] Updating Chrome Policy for %s", targetResource)

	old, _ := d.GetChange("policies")

	// Org units use inherit-then-create: reset old policies, then apply new ones.
	if diags := orgUnitBatchInherit(ctx, chromePoliciesService, client.Customer, d, old.([]interface{})); diags.HasError() {
		return diags
	}

	diags = resourceChromePolicyCreate(ctx, d, meta)
	if diags.HasError() {
		return diags
	}

	log.Printf("[DEBUG] Finished updating Chrome Policy for %s", targetResource)
	return diags
}

func resourceChromePolicyRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return chromePolicyReadCommon(ctx, d, meta, targetOrgUnit)
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

	targetResource := chromePolicyTargetResource(targetOrgUnit, d.Id())
	log.Printf("[DEBUG] Deleting Chrome Policy for %s", targetResource)

	if diags := orgUnitBatchInherit(ctx, chromePoliciesService, client.Customer, d, d.Get("policies").([]interface{})); diags.HasError() {
		return diags
	}

	log.Printf("[DEBUG] Finished deleting Chrome Policy for %s", targetResource)
	return nil
}

// orgUnitBatchInherit resets a set of policies to inherit from the parent OU.
// Used by both Update (inherit old, then re-create new) and Delete (inherit all).
// Handles additional_target_keys grouping and non-fatal error suppression.
func orgUnitBatchInherit(
	ctx context.Context,
	chromePoliciesService *chromepolicy.CustomersPoliciesService,
	customer string,
	d *schema.ResourceData,
	policies []interface{},
) diag.Diagnostics {
	targetResource := chromePolicyTargetResource(targetOrgUnit, d.Id())
	additionalTargetKeysRaw, hasAdditionalKeys := d.GetOk("additional_target_keys")

	// buildInheritRequests creates BatchInherit requests for the given target key.
	buildInheritRequests := func(targetKey *chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey) []*chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest {
		var requests []*chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest
		for _, p := range policies {
			policy := p.(map[string]interface{})
			requests = append(requests, &chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest{
				PolicyTargetKey: targetKey,
				PolicySchema:    policy["schema_name"].(string),
			})
		}
		return requests
	}

	// doInherit executes a BatchInherit call, suppressing non-fatal errors.
	doInherit := func(requests []*chromepolicy.GoogleChromePolicyVersionsV1InheritOrgUnitPolicyRequest, label string) diag.Diagnostics {
		if len(requests) == 0 {
			log.Printf("[DEBUG] Skipping BatchInherit for %s %s — no policies", targetResource, label)
			return nil
		}
		err := retryTimeDuration(ctx, chromePolicyRetryDuration, func() error {
			_, retryErr := chromePoliciesService.Orgunits.BatchInherit(
				fmt.Sprintf("customers/%s", customer),
				&chromepolicy.GoogleChromePolicyVersionsV1BatchInheritOrgUnitPoliciesRequest{Requests: requests},
			).Do()
			return retryErr
		})
		if err != nil {
			if isNonFatalDeleteError(err) {
				log.Printf("[DEBUG] Ignoring non-fatal error during OU policy inheritance %s: %v", label, err)
			} else {
				return diag.FromErr(err)
			}
		}
		return nil
	}

	if !hasAdditionalKeys {
		targetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
			TargetResource: targetResource,
		}
		return doInherit(buildInheritRequests(targetKey), "")
	}

	keyGroups := groupAdditionalTargetKeys(additionalTargetKeysRaw.([]interface{}))
	log.Printf("[DEBUG] Grouped additional_target_keys into %d groups", len(keyGroups))

	for targetKeyName, keyValuePairs := range keyGroups {
		log.Printf("[DEBUG] Processing target_key: %s with %d target_values", targetKeyName, len(keyValuePairs))
		for _, kvp := range keyValuePairs {
			targetKey := &chromepolicy.GoogleChromePolicyVersionsV1PolicyTargetKey{
				TargetResource:       targetResource,
				AdditionalTargetKeys: map[string]string{kvp["key"]: kvp["value"]},
			}
			label := fmt.Sprintf("(%s=%s)", kvp["key"], kvp["value"])
			if diags := doInherit(buildInheritRequests(targetKey), label); diags.HasError() {
				return diags
			}
		}
	}

	return nil
}

func resourceChromePolicyImport(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	return chromePolicyImportCommon(ctx, d, meta, targetOrgUnit, "org_unit_id")
}
