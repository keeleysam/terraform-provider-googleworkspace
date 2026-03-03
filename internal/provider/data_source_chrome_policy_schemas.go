package googleworkspace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"google.golang.org/api/chromepolicy/v1"
)

func dataSourceChromePolicySchemas() *schema.Resource {
	return &schema.Resource{
		Description: "Chrome Policy Schemas data source in the Terraform Googleworkspace provider. Chrome Policy Schemas " +
			"resides under the `https://www.googleapis.com/auth/chrome.management.policy` client scope.",

		ReadContext: dataSourceChromePolicySchemasRead,

		Schema: map[string]*schema.Schema{
			"filter": {
				Description: "The schema filter used to find a particular schema based on fields like its resource name, description and `additionalTargetKeyNames`.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"policy_schemas": {
				Description: "A list of Chrome Policy Schema resources.",
				Type:        schema.TypeList,
				Computed:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"schema_name": {
							Description: "The full qualified name of the policy schema.",
							Type:        schema.TypeString,
							Computed:    true,
						},
						"policy_description": {
							Description: "Description about the policy schema for user consumption.",
							Type:        schema.TypeString,
							Computed:    true,
						},
						"additional_target_key_names": {
							Description: "Additional key names that will be used to identify the target of the policy value.",
							Type:        schema.TypeList,
							Computed:    true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"key": {
										Description: "Key name.",
										Type:        schema.TypeString,
										Computed:    true,
									},
									"key_description": {
										Description: "Key description.",
										Type:        schema.TypeString,
										Computed:    true,
									},
								},
							},
						},
						"definition": {
							Description: "Schema definition using proto descriptor.",
							Type:        schema.TypeList,
							Computed:    true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"name": {
										Description: "File name, relative to root of source tree.",
										Type:        schema.TypeString,
										Computed:    true,
									},
									"package": {
										Description: "e.g. 'foo', 'foo.bar', etc.",
										Type:        schema.TypeString,
										Computed:    true,
									},
									"message_type": {
										Description: "All top-level definitions in this file, represented as a JSON string.",
										Type:        schema.TypeString,
										Computed:    true,
									},
									"enum_type": {
										Type:     schema.TypeList,
										Computed: true,
										Elem: &schema.Resource{
											Schema: map[string]*schema.Schema{
												"name": {
													Type:     schema.TypeString,
													Computed: true,
												},
												"value": {
													Type:     schema.TypeList,
													Computed: true,
													Elem: &schema.Resource{
														Schema: map[string]*schema.Schema{
															"name": {
																Type:     schema.TypeString,
																Computed: true,
															},
															"number": {
																Type:     schema.TypeInt,
																Computed: true,
															},
														},
													},
												},
											},
										},
									},
									"syntax": {
										Description: "The syntax of the proto file. The supported values are 'proto' and 'proto3'.",
										Type:        schema.TypeString,
										Computed:    true,
									},
								},
							},
						},
						"field_descriptions": {
							Description: "Detailed description of each field that is part of the schema, represented as a JSON string.",
							Type:        schema.TypeString,
							Computed:    true,
						},
						"access_restrictions": {
							Description: "Specific access restrictions related to this policy.",
							Type:        schema.TypeList,
							Computed:    true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
						"notices": {
							Description: "Special notice messages related to setting certain values in certain fields in the schema.",
							Type:        schema.TypeList,
							Computed:    true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"field": {
										Description: "The field name associated with the notice.",
										Type:        schema.TypeString,
										Computed:    true,
									},
									"notice_value": {
										Description: "The value of the field that has a notice.",
										Type:        schema.TypeString,
										Computed:    true,
									},
									"notice_message": {
										Description: "The notice message associate with the value of the field.",
										Type:        schema.TypeString,
										Computed:    true,
									},
									"acknowledgement_required": {
										Description: "Whether the user needs to acknowledge the notice message before the value can be set.",
										Type:        schema.TypeBool,
										Computed:    true,
									},
								},
							},
						},
						"support_uri": {
							Description: "URI to related support article for this schema.",
							Type:        schema.TypeString,
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func dataSourceChromePolicySchemasRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	chromePolicySchemasService, diags := GetChromePolicySchemasService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	listCall := chromePolicySchemasService.List(fmt.Sprintf("customers/%s", client.Customer)).PageSize(1000)

	filter := d.Get("filter").(string)
	if filter != "" {
		listCall = listCall.Filter(filter)
	}

	var result []*chromepolicy.GoogleChromePolicyV1PolicySchema
	err := listCall.Pages(ctx, func(resp *chromepolicy.GoogleChromePolicyV1ListPolicySchemasResponse) error {
		result = append(result, resp.PolicySchemas...)
		return nil
	})

	if err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("policy_schemas", flattenChromePolicySchemas(result)); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(fmt.Sprintf("chrome_policy_schemas/%s", filter))

	return diags
}

func flattenChromePolicySchemas(schemas []*chromepolicy.GoogleChromePolicyV1PolicySchema) []interface{} {
	var result []interface{}

	for _, s := range schemas {
		result = append(result, flattenChromePolicySchema(s))
	}

	return result
}

func flattenChromePolicySchema(s *chromepolicy.GoogleChromePolicyV1PolicySchema) interface{} {
	obj := map[string]interface{}{}
	obj["schema_name"] = s.SchemaName
	obj["policy_description"] = s.PolicyDescription
	obj["support_uri"] = s.SupportUri
	obj["additional_target_key_names"] = flattenAdditionalTargetKeyNames(s.AdditionalTargetKeyNames)
	obj["definition"] = flattenDefinition(s.Definition)
	obj["access_restrictions"] = s.AccessRestrictions
	obj["notices"] = flattenNotices(s.Notices)

	fieldDescriptions, _ := json.MarshalIndent(s.FieldDescriptions, "", "  ")
	obj["field_descriptions"] = string(fieldDescriptions)

	return obj
}
