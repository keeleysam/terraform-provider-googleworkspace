package googleworkspace

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestChromePolicyTargetKindString(t *testing.T) {
	if string(targetOrgUnit) != "orgunits" {
		t.Errorf("targetOrgUnit constant changed: %s", string(targetOrgUnit))
	}
	if string(targetGroup) != "groups" {
		t.Errorf("targetGroup constant changed: %s", string(targetGroup))
	}
}

func TestChromePolicyTargetResource(t *testing.T) {
	cases := []struct {
		kind     chromePolicyTargetKind
		id       string
		expected string
	}{
		{targetOrgUnit, "abc123", "orgunits/abc123"},
		{targetGroup, "def456", "groups/def456"},
	}
	for _, c := range cases {
		if got := chromePolicyTargetResource(c.kind, c.id); got != c.expected {
			t.Errorf("chromePolicyTargetResource(%s, %s) = %s, want %s", c.kind, c.id, got, c.expected)
		}
	}
}

func TestGroupAdditionalTargetKeys(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{"target_key": "app_id", "target_value": "chrome:abc"},
		map[string]interface{}{"target_key": "app_id", "target_value": "chrome:def"},
		map[string]interface{}{"target_key": "profile_id", "target_value": "xyz"},
	}
	got := groupAdditionalTargetKeys(input)

	if len(got) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(got))
	}
	if len(got["app_id"]) != 2 {
		t.Errorf("expected 2 entries for app_id, got %d", len(got["app_id"]))
	}
	if len(got["profile_id"]) != 1 {
		t.Errorf("expected 1 entry for profile_id, got %d", len(got["profile_id"]))
	}
	if got["profile_id"][0]["value"] != "xyz" {
		t.Errorf("expected profile_id value 'xyz', got %s", got["profile_id"][0]["value"])
	}
}

func TestGroupAdditionalTargetKeys_empty(t *testing.T) {
	got := groupAdditionalTargetKeys([]interface{}{})
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d entries", len(got))
	}
}

func TestValidatePolicyFieldValueType(t *testing.T) {
	cases := []struct {
		fieldType string
		value     interface{}
		expect    bool
	}{
		{"TYPE_BOOL", true, true},
		{"TYPE_BOOL", "true", false},
		{"TYPE_DOUBLE", 1.23, true},
		{"TYPE_INT64", float64(10), true},
		{"TYPE_INT64", float64(10.5), false},
		{"TYPE_STRING", "abc", true},
		{"TYPE_ENUM", "SOME_ENUM", true},
		{"TYPE_MESSAGE", map[string]interface{}{"k": "v"}, true},
		{"TYPE_MESSAGE", []string{"x"}, false},
		{"TYPE_UINT32", float32(3), true},
		{"TYPE_UINT32", float32(3.1), false},
	}
	for _, c := range cases {
		if got := validatePolicyFieldValueType(c.fieldType, c.value); got != c.expect {
			t.Errorf("validatePolicyFieldValueType(%s,%v) expected %v got %v", c.fieldType, c.value, c.expect, got)
		}
	}
}

func TestConvertPolicyFieldValueType(t *testing.T) {
	cases := []struct {
		fieldType string
		in        interface{}
		want      interface{}
		wantErr   bool
	}{
		{"TYPE_BOOL", "true", true, false},
		{"TYPE_BOOL", "notbool", nil, true},
		{"TYPE_DOUBLE", "1.25", float64(1.25), false},
		{"TYPE_INT64", "42", int64(42), false},
		{"TYPE_INT64", "4.2", nil, true},
		{"TYPE_UINT32", "7", int64(7), false},
		{"TYPE_STRING", "abc", "abc", false},
		{"TYPE_ENUM", "ENUM_VAL", "ENUM_VAL", false},
	}
	for _, c := range cases {
		got, err := convertPolicyFieldValueType(c.fieldType, c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("expected error for %s input %v", c.fieldType, c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("unexpected error for %s input %v: %v", c.fieldType, c.in, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("convertPolicyFieldValueType(%s,%v) expected %v got %v", c.fieldType, c.in, c.want, got)
		}
	}
}

func TestExpandChromePoliciesAdditionalTargetKeys(t *testing.T) {
	in := []interface{}{
		map[string]interface{}{"target_key": "app_id", "target_value": "chrome:abc"},
		map[string]interface{}{"target_key": "profile_id", "target_value": "def"},
	}
	got := expandChromePoliciesAdditionalTargetKeys(in)
	if got["app_id"] != "chrome:abc" || got["profile_id"] != "def" || len(got) != 2 {
		t.Errorf("unexpected map result: %#v", got)
	}
}

func TestExpandChromePoliciesValues(t *testing.T) {
	input := []interface{}{map[string]interface{}{
		"schema_name": "chrome.users.MaxConnectionsPerProxy",
		"schema_values": map[string]interface{}{
			"maxConnectionsPerProxy": jsonMustMarshalToString(8),
		},
	}}
	vals, diags := expandChromePoliciesValues(input)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 policy value, got %d", len(vals))
	}
	if vals[0].PolicySchema != "chrome.users.MaxConnectionsPerProxy" {
		t.Errorf("unexpected schema name: %s", vals[0].PolicySchema)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(vals[0].Value, &decoded); err != nil {
		t.Fatalf("error unmarshalling stored value: %v", err)
	}
	if decoded["maxConnectionsPerProxy"].(float64) != 8 {
		t.Errorf("expected stored numeric value 8, got %#v", decoded["maxConnectionsPerProxy"])
	}
}

func jsonMustMarshalToString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
