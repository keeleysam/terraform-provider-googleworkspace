package googleworkspace

import (
	"fmt"
	"log"
	"net/mail"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/hashicorp/errwrap"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/mitchellh/go-homedir"

	"google.golang.org/api/googleapi"
)

// If the argument is a path, pathOrContents loads it and returns the contents,
// otherwise the argument is assumed to be the desired contents and is simply
// returned.
//
// The boolean second return value can be called `wasPath` - it indicates if a
// path was detected and a file loaded.
func pathOrContents(poc string) (string, bool, error) {
	if len(poc) == 0 {
		return poc, false, nil
	}

	path := poc
	if path[0] == '~' {
		var err error
		path, err = homedir.Expand(path)
		if err != nil {
			return path, true, err
		}
	}

	if _, err := os.Stat(path); err == nil {
		contents, err := os.ReadFile(path)
		if err != nil {
			return string(contents), true, err
		}
		return string(contents), true, nil
	}

	return poc, false, nil
}

// Check Error Code
func isApiErrorWithCode(err error, errCode int) bool {
	gerr, ok := errwrap.GetType(err, &googleapi.Error{}).(*googleapi.Error)
	return ok && gerr != nil && gerr.Code == errCode
}

// isNonFatalDeleteError returns true for 400 errors that can be safely ignored
// when deleting OU-based Chrome policies via BatchInherit.
//
// The Chrome Policy API for OrgUnits only provides BatchModify (set values) and
// BatchInherit (reset to parent). There is no BatchDelete equivalent for OUs —
// BatchInherit is the only deletion mechanism.
//
// Three known non-fatal cases:
//
//  1. "apps are not installed": The app was uninstalled from the domain. The policy
//     no longer applies and there is nothing to inherit.
//
//  2. "Install Type can only be inherited if it is configured in a parent
//     Organizational Unit": The app was only configured on this specific OU and
//     has no configuration on any ancestor OU. The API rejects the BatchInherit
//     call because there is nothing to inherit from. This error proves the policy
//     is already absent from the parent scope, so the deletion is effectively a
//     no-op. A batchDelete equivalent for OUs does not exist in the API (see:
//     https://developers.google.com/chrome/policy/reference/rest/v1/customers.policies.orgunits).
//     This case has been reported to Google (support case pending).
//
//  3. "BatchInheritOrgUnitPolicies request must contain at least one request":
//     The API rejects the call even when the requests slice is non-empty. This
//     happens when the target OU no longer exists (e.g., was deleted outside
//     Terraform) or the policy schema is no longer valid for that target. In both
//     cases the policy is already absent, so the deletion is a no-op.
func isNonFatalDeleteError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "apps are not installed") ||
		strings.Contains(msg, "Install Type can only be inherited") ||
		strings.Contains(msg, "BatchInheritOrgUnitPolicies request must contain at least one request")
}

func handleNotFoundError(err error, d *schema.ResourceData, resource string) diag.Diagnostics {
	if isApiErrorWithCode(err, 404) {
		log.Printf("[WARN] Removing %s because it's gone", resource)
		// The resource doesn't exist anymore
		d.SetId("")

		return nil
	}

	return diag.Errorf("Error when reading or editing %s: %s", resource, err.Error())
}

// This is a Printf sibling (Nprintf; Named Printf), which handles strings like
// Nprintf("Hello %{target}!", map[string]interface{}{"target":"world"}) == "Hello world!".
// This is particularly useful for generated tests, where we don't want to use Printf,
// since that would require us to generate a very particular ordering of arguments.
func Nprintf(format string, params map[string]interface{}) string {
	for key, val := range params {
		format = strings.Replace(format, "%{"+key+"}", fmt.Sprintf("%v", val), -1)
	}
	return format
}

// This will translate a snake cased string to a camel case string
// Note: the first letter of the camel case string will be lower case
func SnakeToCamel(s string) string {
	titled := strings.Title(strings.ReplaceAll(s, "_", " "))
	cameled := strings.Join(strings.Split(titled, " "), "")

	// Lower the first letter
	result := []rune(cameled)
	result[0] = unicode.ToLower(result[0])
	return string(result)
}

// This will translate a snake cased string to a camel case string
// Note: the first letter of the camel case string will be lower case
func CameltoSnake(s string) string {
	var res = make([]rune, 0, len(s))
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			res = append(res, '_', unicode.ToLower(r))
		} else {
			res = append(res, unicode.ToLower(r))
		}
	}
	return string(res)
}

// For resources that have many nested interfaces, we can pass them to the API as is,
// only each field name needs to be camel case rather than snake case. Additionally,
// fields that are not set should not be sent to the API.
func expandInterfaceObjects(parent interface{}) []interface{} {
	objList := parent.([]interface{})
	if len(objList) == 0 {
		return nil
	}

	newObjList := []interface{}{}

	for _, o := range objList {
		obj := o.(map[string]interface{})
		for k, v := range obj {
			if strings.Contains(k, "_") || v == "" {
				delete(obj, k)

				// In the case that the field is not set, don't send it to the API
				if v == "" {
					continue
				}

				obj[SnakeToCamel(k)] = v
			}
		}
		newObjList = append(newObjList, obj)
	}

	return newObjList
}

// For resources that have many nested interfaces, we can set was was returned from the API as is
// only the field names need to be snake case rather than the camel case that is returned
func flattenInterfaceObjects(objList interface{}) interface{} {
	if objList == nil || len(objList.([]interface{})) == 0 {
		return nil
	}

	newObjList := []map[string]interface{}{}

	for _, o := range objList.([]interface{}) {
		obj := o.(map[string]interface{})
		for k, v := range obj {
			delete(obj, k)
			obj[CameltoSnake(k)] = v
		}

		newObjList = append(newObjList, obj)
	}

	return newObjList
}

// Converts a list of interfaces to a list of strings
func listOfInterfacestoStrings(v interface{}) []string {
	result := []string{}

	if v == nil {
		return result
	}

	for _, s := range v.([]interface{}) {
		result = append(result, s.(string))
	}

	return result
}

func stringInSlice(arr []string, str string) bool {
	for _, i := range arr {
		if i == str {
			return true
		}
	}

	return false
}

// sort a slice of interfaces regardless the type, return the equivalent slice of strings
func sortListOfInterfaces(v []interface{}) []string {
	newVal := make([]string, len(v))
	for idx, attr := range v {
		kind := reflect.ValueOf(v).Kind()
		if kind == reflect.Float64 {
			attr = strconv.FormatFloat(attr.(float64), 'f', -1, 64)
		}
		newVal[idx] = fmt.Sprintf("%+v", attr)
	}
	sort.Strings(newVal)
	return newVal
}

// isEmail returns a boolean indicating if the input string is parsable as an email
func isEmail(input string) bool {
	_, err := mail.ParseAddress(input)
	return err == nil
}
