package googleworkspace

import (
	"encoding/json"
	"testing"

	"google.golang.org/api/chromepolicy/v1"
)

func TestSchemaNameMatchesFilter_exact(t *testing.T) {
	cases := []struct {
		name   string
		filter string
		want   bool
	}{
		{"chrome.users.MaxConnectionsPerProxy", "chrome.users.MaxConnectionsPerProxy", true},
		{"chrome.users.MaxConnectionsPerProxy", "chrome.users.OtherPolicy", false},
		{"chrome.users.MaxConnectionsPerProxy", "chrome.users.*", true},
		{"chrome.users.apps.InstallType", "chrome.users.apps.*", true},
		{"chrome.users.apps.InstallType", "chrome.users.*", false},     // dots in leaf
		{"chrome.users.MaxConnectionsPerProxy", "chrome.devices.*", false}, // wrong prefix
		{"chrome.users.MaxConnectionsPerProxy", "chrome.*", false},     // leaf has dots
		{"", "chrome.users.*", false},                                  // empty name
		{"chrome.users.Foo", "chrome.users.Foo", true},                 // exact match, no wildcard
	}
	for _, c := range cases {
		got := schemaNameMatchesFilter(c.name, c.filter)
		if got != c.want {
			t.Errorf("schemaNameMatchesFilter(%q, %q) = %v, want %v", c.name, c.filter, got, c.want)
		}
	}
}

func TestCanonicalAdditionalTargetKeys(t *testing.T) {
	// Should produce deterministic sorted output
	keys := map[string]interface{}{
		"zebra_id": "z1",
		"app_id":   "a1",
		"mid_id":   "m1",
	}
	got := canonicalAdditionalTargetKeys(keys)
	want := "app_id=a1,mid_id=m1,zebra_id=z1"
	if got != want {
		t.Errorf("canonicalAdditionalTargetKeys = %q, want %q", got, want)
	}
}

func TestCanonicalAdditionalTargetKeys_empty(t *testing.T) {
	got := canonicalAdditionalTargetKeys(map[string]interface{}{})
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestChromePolicySetHash_deterministic(t *testing.T) {
	pol := map[string]interface{}{
		"schema_name":   "chrome.users.MaxConnectionsPerProxy",
		"schema_values": map[string]interface{}{},
	}
	h1 := chromePolicySetHash(pol)
	h2 := chromePolicySetHash(pol)
	if h1 != h2 {
		t.Errorf("hash not deterministic: %d != %d", h1, h2)
	}
}

func TestChromePolicySetHash_differentSchemas(t *testing.T) {
	pol1 := map[string]interface{}{
		"schema_name":   "chrome.users.Foo",
		"schema_values": map[string]interface{}{},
	}
	pol2 := map[string]interface{}{
		"schema_name":   "chrome.users.Bar",
		"schema_values": map[string]interface{}{},
	}
	if chromePolicySetHash(pol1) == chromePolicySetHash(pol2) {
		t.Error("different schemas should produce different hashes")
	}
}

func TestChromePolicySetHash_withAdditionalKeys(t *testing.T) {
	pol1 := map[string]interface{}{
		"schema_name":          "chrome.users.apps.InstallType",
		"schema_values":        map[string]interface{}{},
		"additional_target_keys": map[string]interface{}{"app_id": "chrome:abc"},
	}
	pol2 := map[string]interface{}{
		"schema_name":          "chrome.users.apps.InstallType",
		"schema_values":        map[string]interface{}{},
		"additional_target_keys": map[string]interface{}{"app_id": "chrome:def"},
	}
	if chromePolicySetHash(pol1) == chromePolicySetHash(pol2) {
		t.Error("different additional_target_keys should produce different hashes")
	}
}

func TestPolicyIdentityKey_noAdditionalKeys(t *testing.T) {
	id := policyIdentity{SchemaName: "chrome.users.Foo"}
	if got := id.key(); got != "chrome.users.Foo" {
		t.Errorf("key() = %q, want %q", got, "chrome.users.Foo")
	}
}

func TestPolicyIdentityKey_withAdditionalKeys(t *testing.T) {
	id := policyIdentity{
		SchemaName:           "chrome.users.apps.InstallType",
		AdditionalTargetKeys: map[string]string{"app_id": "chrome:abc"},
	}
	got := id.key()
	want := "chrome.users.apps.InstallType\x00app_id=chrome:abc"
	if got != want {
		t.Errorf("key() = %q, want %q", got, want)
	}
}

func TestPolicyIdentityKey_sortedAdditionalKeys(t *testing.T) {
	id := policyIdentity{
		SchemaName: "schema",
		AdditionalTargetKeys: map[string]string{
			"z_key": "z",
			"a_key": "a",
		},
	}
	got := id.key()
	want := "schema\x00a_key=a,z_key=z"
	if got != want {
		t.Errorf("key() = %q, want %q", got, want)
	}
}

func TestIdentityFromPolicy(t *testing.T) {
	pol := map[string]interface{}{
		"schema_name":          "chrome.users.apps.InstallType",
		"schema_values":        map[string]interface{}{},
		"additional_target_keys": map[string]interface{}{"app_id": "chrome:abc"},
	}
	id := identityFromPolicy(pol)
	if id.SchemaName != "chrome.users.apps.InstallType" {
		t.Errorf("SchemaName = %q", id.SchemaName)
	}
	if id.AdditionalTargetKeys["app_id"] != "chrome:abc" {
		t.Errorf("AdditionalTargetKeys = %v", id.AdditionalTargetKeys)
	}
}

func TestIdentityFromPolicy_noAdditionalKeys(t *testing.T) {
	pol := map[string]interface{}{
		"schema_name":   "chrome.users.Foo",
		"schema_values": map[string]interface{}{},
	}
	id := identityFromPolicy(pol)
	if id.SchemaName != "chrome.users.Foo" {
		t.Errorf("SchemaName = %q", id.SchemaName)
	}
	if len(id.AdditionalTargetKeys) != 0 {
		t.Errorf("expected no additional keys, got %v", id.AdditionalTargetKeys)
	}
}

func TestFlattenResolvedPolicies(t *testing.T) {
	valueJSON, _ := json.Marshal(map[string]interface{}{
		"maxConnectionsPerProxy": 32,
	})
	entries := []resolvedPolicyEntry{
		{
			Identity: policyIdentity{SchemaName: "chrome.users.MaxConnectionsPerProxy"},
			Value: &chromepolicy.GoogleChromePolicyVersionsV1PolicyValue{
				PolicySchema: "chrome.users.MaxConnectionsPerProxy",
				Value:        valueJSON,
			},
		},
	}
	result, diags := flattenResolvedPolicies(entries)
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0]["schema_name"] != "chrome.users.MaxConnectionsPerProxy" {
		t.Errorf("schema_name = %v", result[0]["schema_name"])
	}
	sv := result[0]["schema_values"].(map[string]interface{})
	if sv["maxConnectionsPerProxy"] != "32" {
		t.Errorf("maxConnectionsPerProxy = %v, want \"32\"", sv["maxConnectionsPerProxy"])
	}
}

func TestFlattenResolvedPolicies_withAdditionalKeys(t *testing.T) {
	valueJSON, _ := json.Marshal(map[string]interface{}{"appInstallType": "FORCED"})
	entries := []resolvedPolicyEntry{
		{
			Identity: policyIdentity{
				SchemaName:           "chrome.users.apps.InstallType",
				AdditionalTargetKeys: map[string]string{"app_id": "chrome:abc"},
			},
			Value: &chromepolicy.GoogleChromePolicyVersionsV1PolicyValue{
				PolicySchema: "chrome.users.apps.InstallType",
				Value:        valueJSON,
			},
		},
	}
	result, diags := flattenResolvedPolicies(entries)
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	atk := result[0]["additional_target_keys"].(map[string]interface{})
	if atk["app_id"] != "chrome:abc" {
		t.Errorf("additional_target_keys = %v", atk)
	}
}

func TestFlattenAdditionalTargetKeys(t *testing.T) {
	got := flattenAdditionalTargetKeys(map[string]string{"app_id": "chrome:abc", "profile": "xyz"})
	if got["app_id"] != "chrome:abc" || got["profile"] != "xyz" || len(got) != 2 {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestFlattenAdditionalTargetKeys_empty(t *testing.T) {
	got := flattenAdditionalTargetKeys(map[string]string{})
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestBuildPolicyTargetKey_noAdditionalKeys(t *testing.T) {
	pol := map[string]interface{}{
		"schema_name":   "chrome.users.Foo",
		"schema_values": map[string]interface{}{},
	}
	key := buildPolicyTargetKey("orgunits/abc", pol)
	if key.TargetResource != "orgunits/abc" {
		t.Errorf("TargetResource = %q", key.TargetResource)
	}
	if len(key.AdditionalTargetKeys) != 0 {
		t.Errorf("expected no additional keys, got %v", key.AdditionalTargetKeys)
	}
}

func TestBuildPolicyTargetKey_withAdditionalKeys(t *testing.T) {
	pol := map[string]interface{}{
		"schema_name":          "chrome.users.apps.InstallType",
		"schema_values":        map[string]interface{}{},
		"additional_target_keys": map[string]interface{}{"app_id": "chrome:abc"},
	}
	key := buildPolicyTargetKey("groups/def", pol)
	if key.TargetResource != "groups/def" {
		t.Errorf("TargetResource = %q", key.TargetResource)
	}
	if key.AdditionalTargetKeys["app_id"] != "chrome:abc" {
		t.Errorf("AdditionalTargetKeys = %v", key.AdditionalTargetKeys)
	}
}

func TestChromePolicySetResourceID(t *testing.T) {
	cases := []struct {
		kind     chromePolicyTargetKind
		targetID string
		filter   string
		want     string
	}{
		{targetOrgUnit, "abc123", "chrome.users.*", "orgunits/abc123/chrome.users.*"},
		{targetGroup, "def456", "chrome.users.apps.*", "groups/def456/chrome.users.apps.*"},
	}
	for _, c := range cases {
		got := chromePolicySetResourceID(c.kind, c.targetID, c.filter)
		if got != c.want {
			t.Errorf("chromePolicySetResourceID(%s, %s, %s) = %q, want %q", c.kind, c.targetID, c.filter, got, c.want)
		}
	}
}
