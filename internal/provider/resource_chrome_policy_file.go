package googleworkspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"google.golang.org/api/chromepolicy/v1"
)

func resourceChromePolicyFile() *schema.Resource {
	return &schema.Resource{
		Description: "Uploads a file (e.g. a wallpaper image) to Google Chrome Policy and " +
			"returns a public download URI that can be referenced in other Chrome policies. " +
			"This is a create-only resource: updates to the file or policy field will force " +
			"recreation, and deletion only removes the resource from Terraform state.",

		CreateContext: resourceChromePolicyFileCreate,
		ReadContext:   resourceChromePolicyFileRead,
		DeleteContext: resourceChromePolicyFileDelete,

		Importer: &schema.ResourceImporter{
			StateContext: resourceChromePolicyFileImport,
		},

		Schema: map[string]*schema.Schema{
			"policy_field": {
				Description: "The fully qualified policy schema and field name this file is " +
					"uploaded for, e.g. `chrome.users.Wallpaper.wallpaperImage`. The API uses " +
					"this to validate the content type of the file.",
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"file_path": {
				Description: "Path to the local file to upload.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"file_sha256": {
				Description: "SHA-256 hash of the uploaded file. Changes to the local file " +
					"will produce a new hash and trigger resource recreation.",
				Type:     schema.TypeString,
				Computed: true,
				ForceNew: true,
			},
			"download_uri": {
				Description: "The public download URI for the uploaded file, returned by the API. " +
					"Use this value to reference the file in Chrome policy configurations.",
				Type:     schema.TypeString,
				Computed: true,
			},
		},

		CustomizeDiff: resourceChromePolicyFileCustomDiff,
	}
}

func resourceChromePolicyFileCustomDiff(ctx context.Context, diff *schema.ResourceDiff, meta interface{}) error {
	if diff.Id() == "" {
		return nil
	}

	filePath := diff.Get("file_path").(string)
	oldHash := diff.Get("file_sha256").(string)

	newHash, err := hashFile(filePath)
	if err != nil {
		// File missing or unreadable — force recreation.
		diff.ForceNew("file_sha256")
		return nil
	}

	if newHash != oldHash {
		if err := diff.SetNew("file_sha256", newHash); err != nil {
			return err
		}
		diff.ForceNew("file_sha256")
	}

	return nil
}

func resourceChromePolicyFileCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*apiClient)

	policyField := d.Get("policy_field").(string)
	filePath := d.Get("file_path").(string)

	log.Printf("[DEBUG] Uploading Chrome Policy file for %s from %s", policyField, filePath)

	fileHash, err := hashFile(filePath)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to read file %s: %w", filePath, err))
	}

	f, err := os.Open(filePath)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to open file %s: %w", filePath, err))
	}
	defer f.Close()

	chromePolicyService, diags := client.NewChromePolicyService()
	if diags.HasError() {
		return diags
	}

	mediaService, diags := GetChromeMediaService(chromePolicyService)
	if diags.HasError() {
		return diags
	}

	req := &chromepolicy.GoogleChromePolicyVersionsV1UploadPolicyFileRequest{
		PolicyField: policyField,
	}

	customer := fmt.Sprintf("customers/%s", client.Customer)
	resp, err := mediaService.Upload(customer, req).Media(f).Context(ctx).Do()
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to upload Chrome Policy file: %w", err))
	}

	if err := d.Set("download_uri", resp.DownloadUri); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("file_sha256", fileHash); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(fmt.Sprintf("%s/%s", policyField, fileHash))

	log.Printf("[DEBUG] Uploaded Chrome Policy file for %s, download_uri=%s", policyField, resp.DownloadUri)
	return nil
}

func resourceChromePolicyFileRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// No read API exists for uploaded policy files. State is authoritative.
	log.Printf("[DEBUG] Reading Chrome Policy file resource %s (no-op, state is authoritative)", d.Id())
	return nil
}

func resourceChromePolicyFileDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// No delete API exists for uploaded policy files.
	log.Printf("[DEBUG] Removing Chrome Policy file %s from state (uploaded file cannot be deleted via API)", d.Id())
	return nil
}

// resourceChromePolicyFileImport accepts an import ID in the format:
//
//	{policy_field}|{file_path}|{download_uri}
//
// The local file at file_path must exist so the SHA-256 hash can be computed.
// After import, terraform plan will show no changes provided the local file
// matches the one that was originally uploaded.
func resourceChromePolicyFileImport(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	parts := strings.SplitN(d.Id(), "|", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, fmt.Errorf(
			"expected import ID format: {policy_field}|{file_path}|{download_uri}, got: %s", d.Id())
	}

	policyField := parts[0]
	filePath := parts[1]
	downloadURI := parts[2]

	fileHash, err := hashFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s for hash computation: %w", filePath, err)
	}

	if err := d.Set("policy_field", policyField); err != nil {
		return nil, err
	}
	if err := d.Set("file_path", filePath); err != nil {
		return nil, err
	}
	if err := d.Set("download_uri", downloadURI); err != nil {
		return nil, err
	}
	if err := d.Set("file_sha256", fileHash); err != nil {
		return nil, err
	}

	d.SetId(fmt.Sprintf("%s/%s", policyField, fileHash))
	return []*schema.ResourceData{d}, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
