## 1.3.12 (March 06, 2026)

BUG FIX

* `googleworkspace_chrome_policy`: Fix HTTP 400 "BatchInheritOrgUnitPolicies request must contain at least one request" during OU policy **update**. When Terraform detects drift and triggers an update, the update function calls `BatchInherit` to clear old schema values before writing new ones. If the old state has no policies recorded (e.g., after drift detection returns 0 resolved policies), the requests slice is empty and the API rejects the call. The provider now skips the `BatchInherit` call when the requests list is empty in both the update and delete paths. Non-fatal 400 errors (including "apps are not installed", "Install Type can only be inherited", and "BatchInheritOrgUnitPolicies request must contain at least one request") are also now suppressed in the update path via `isNonFatalDeleteError`.

## 1.3.10 (March 05, 2026)

BUG FIX

* `googleworkspace_chrome_policy`: Fix HTTP 400 "BatchInheritOrgUnitPolicies request must contain at least one request" during OU policy deletion. This occurs when Terraform state has no policies recorded for the resource (e.g., after a failed import or partial apply). The provider now skips the `BatchInherit` call when the requests list is empty, treating the resource as already absent.

## 1.3.9 (March 05, 2026)

BUG FIX

* `googleworkspace_chrome_policy`, `googleworkspace_chrome_group_policy`: Fix 429 quota errors not being retried at the application level. `retryTimeDuration` previously only retried on eventual-consistency errors ("timed out while waiting"); it now delegates to `isRetryableError` which covers 429 rate-limit, 403 quota-exceeded, 5xx, and network errors. The retry window is also extended from 1 minute to 5 minutes to allow the transport-level Fibonacci backoff (up to 90 seconds per attempt) to complete without context cancellation.

## 1.3.8 (March 05, 2026)

BUG FIX

* `googleworkspace_chrome_policy`: Gracefully handle HTTP 400 "Install Type can only be inherited if it is configured in a parent Organizational Unit" during OU policy deletion. This error occurs when an extension is only configured on a child OU with no parent OU configuration — the Chrome Policy API has no `batchDelete` equivalent for OUs, and `batchInherit` rejects the call when there is nothing to inherit from. The error is non-fatal because it proves the policy is already absent from the parent scope.

## 1.3.7 (March 04, 2026)

FEATURES

* `data.googleworkspace_chrome_policy_group_priority_ordering`: Add computed `exists` boolean attribute; handle HTTP 400 gracefully (returns empty `group_ids` and `exists=false` instead of failing)

## 1.3.6 (March 04, 2026)

BUG FIX

* Retry logic on ordering resource

## 1.3.5 (March 03, 2026)

FEATURES

* Create before delete logic

## 1.3.4 (February 19, 2026)

BUG FIX

* Fix delete ordering resources

## 1.3.3 (February 19, 2026)

BUG FIX

* Fix delete ordering resources

## 1.3.2 (February 19, 2026)

BUG FIX

* Imports fix

## 1.3.1 (February 19, 2026)

BUG FIX

* Imports fix

## 1.3.0 (February 19, 2026)

FEATURES

* Support Imports

## 1.2.31 (February 19, 2026)

BUG FIX

* Fix root path lookup.


## 1.2.29 (December 18, 2025)

BUG FIX
* Fix policyTargetKey use for policy group ordering

## 1.2.28 (December 18, 2025)

BUG FIX
* Fix policyTargetKey use for policy group ordering

## 1.2.27 (December 18, 2025)

FEATURE
* Add Chrome policy group ordering support

## 1.2.26 (October 20, 2025)

BUG FIX
* Fix update state handling with additional_target_keys
  
## 1.2.23 (October 20, 2025)

BUG FIX
* Fix update state handling with additional_target_keys

## 1.2.22 (October 20, 2025)

BUG FIX
* Fix update state handling

## 1.2.21 (October 20, 2025)

BUG FIX
* Fix state change detection

## 1.2.20 (October 20, 2025)

BUG FIX
* Fix state change detection

## 1.2.19 (October 17, 2025)

IMPROVEMENTS
* Slight refactor (removed chrome policy common as the code is not really common between group and org unit API calls) and cleaned up warnings

## 1.2.18 (October 17, 2025)

BUG FIX
* Fix state change detection

## 1.2.17 (October 16, 2025)

BUG FIX
* Dynamic group creation bug fix

## 1.2.16 (October 16, 2025)

BUG FIX
* Dynamic group creation bug fix

## 1.2.15 (October 16, 2025)

BUG FIX
* Dynamic group creation bug fix


## 1.2.14 (October 16, 2025)

FEATURE
* Support secuirty groups through the googleworkspace_group resource, required cloudidentity api scope

## 1.2.13 (October 16, 2025)

FEATURE
* Support secuirty groups through the googleworkspace_group resource, required cloudidentity api scope

## 1.2.12 (October 16, 2025)

FEATURE
* Support secuirty groups through the googleworkspace_group resource, required cloudidentity api scope

## 1.2.11 (October 16, 2025)

FEATURE
* Support secuirty groups through the googleworkspace_group resource, required cloudidentity api scope

## 1.2.10 (October 16, 2025)

FEATURE
* Support secuirty groups through the googleworkspace_group resource, required cloudidentity api scope

## 1.2.9 (October 16, 2025)

BUG FIXES:
* fix image type

## 1.2.8 (October 15, 2025)

BUG FIXES:
* add debug logs

## 1.2.7 (October 15, 2025)

BUG FIXES:
* add debug logs

## 1.2.6 (October 15, 2025)

BUG FIXES:
* add debug logs

## 1.2.5 (October 15, 2025)

BUG FIXES:
* asset file upload fix

## 1.2.4 (October 15, 2025)

FEATURES:
* add Chrome policy file resource for uploading files to reference in chrome policies (ex: wallpapers)

## 1.2.3 (October 15, 2025)

BUG FIXES:
* groups: fix dynamic group api call

## 1.2.2 (October 15, 2025)

BUG FIXES:
* groups: fix dynamic group api call

## 1.2.1 (October 15, 2025)

BUG FIXES:
* groups: fix dynamic group api call

## 1.2.0 (October 14, 2025)

FEATURES:
* **New Resource**: `googleworkspace_group_dynamic` - Create and manage dynamic Google Workspace groups with query-based membership using the Cloud Identity API
* **New Service**: Added Cloud Identity API support with `NewCloudIdentityService()` and `GetCloudIdentityGroupsService()` helper functions

IMPROVEMENTS:
* provider: Added `cloud-identity.groups` scope to default OAuth scopes for dynamic group support

## 1.1.11 (October 13, 2025)

BUG FIXES:
* chrome: workaround batch update issues with Google's API

## 1.1.10 (October 13, 2025)

FEATURES:
* support passing a group email to googleworkspace_chrome_group_policy

## 1.1.9 (October 13, 2025)

BUG FIXES:
* chrome: workaround batch delete issues with Google's API

## 1.1.8 (October 13, 2025)

BUG FIXES:
* chrome: fix group policy updates not deleting removed policies

## 1.1.7 (October 13, 2025)

BUG FIXES:
* chrome: test

## 1.1.6 (October 13, 2025)

BUG FIXES:
* chrome: fix handling multiple policies in one resource

## 1.1.5 (October 13, 2025)

BUG FIXES:
* chrome: fix group based policy deletes

## 1.1.4 (October 13, 2025)

BUG FIXES:
* chrome: fix group based policy updates

## 1.1.3 (October 10, 2025)

IMPROVEMENTS:
* provider: updated Chrome Policy API and dependencies to latest versions

BUG FIXES:
* chrome: fixed a bug where when validating a `googleworkspace_chrome_policy` schema value the proto `LABEL_REPEATED` label was not being respected to require an array of the associated type ([#336](https://github.com/hashicorp/terraform-provider-googleworkspace/pull/336))

## 0.7.0 (June 10, 2022)

FEATURES:

* **New Datasource:** `googleworkspace_groups` ([#313](https://github.com/hashicorp/terraform-provider-googleworkspace/pull/313))

IMPROVEMENTS:

* provider: added support for using the `GOOGLE_OAUTH_ACCESS_TOKEN` environment variable for authentication ([#319](https://github.com/hashicorp/terraform-provider-googleworkspace/pull/319))

BUG FIXES:

* directory: fixed bug where `custom_schemas` were not populating in `googleworkspace_users` [GH-279]
* directory: fixed bug where `recovery_phone` and `recovery_email` could not be removed from the `googleworkspace_user` resource [GH-261]
* directory: fixed bug where creation of `user`, `group`, `group_member`, and `group_settings`resources were affected by 404s from the API [GH-287]
* directory: fixed bug where creating a `googleworkspace_org_unit` would fail with `401` despite it being created [GH-327]

## 0.6.0 (December 20, 2021)

FEATURES:

* **New Datasource:** `googleworkspace_users` ([#240](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/240))

IMPROVEMENTS:

* directory: added `include_derived_membership` attribute to `data.googleworkspace_group_members` ([#234](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/234))
* directory: added `preference` attribute to `googleworkspace_user.languages` ([#238](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/238))

BUG FIXES:

* directory: fixed bug where data sources that allow exactly one field to be defined would drop the `id` field when the data source was recomputed. ([#218](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/218))
* directory: fixed bug where `googleworkspace_group_member` would crash instead of surfacing an error if needed ([#220](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/220))

## 0.5.2 (November 17, 2021)

BUG FIXES:

* directory: fixed bug where `googleworkspace_group_member` would error if a member already existed ([#211](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/211))
* directory: fixed bug where `googleworkspace_group_members` would only show the first 200 members ([#209](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/209))
* directory: fixed bug where `googleworkspace_user` showed a perma-diff on `languages.language_code` ([#208](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/208))
* directory: fixed bug where `googleworkspace_user` errored when adding `aliases` on initial create ([#208](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/208))
* groups settings: added validation options `ALL_OWNERS_CAN_CONTACT` validation to `who_can_contact_owner` and `ALL_OWNERS_CAN_VIEW` to `who_can_view_membership` and `who_can_view_group`. ([#207](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/207))

## 0.5.1 (October 27, 2021)

BUG FIXES:

* directory: fixed eventual consistency for `googleworkspace_group_member` ([#186](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/186))
* directory: fixed bug where `googleworkspace_group_members` would error if a member already existed ([#194](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/194))
* directory: fixed bug where `googleworkspace_group_members` would error if a `members` was empty ([#193](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/193))

## 0.5.0 (October 13, 2021)

FEATURES:

* **New Resource:**   `googleworkspace_group_members` ([#155](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/155))

* **New Datasource:** `googleworkspace_group_members` ([#155](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/155))

IMPROVEMENTS:

* provider: added ability to authenticate using user credentials ([#156](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/156))
* provider: added ability to authenticate using `access_token` rather than just credentials ([#165](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/165))
* provider: added retryTransport that will retry after common Google errors (like network errors, rate limiting errors, etc) ([#163](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/163))

BUG FIXES:

* directory: fixed bug where `googleworkspace_group_member` would not force new change on change of email ([#152](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/152))
* directory: fixed bug where `googleworkspace_user` would show permadiff on user alias emails from secondary domains ([#154](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/154))

## 0.4.1 (August 16, 2021)

BUG FIXES:

* provider: fixed validation of credentials ([#126](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/126))
* directory: fixed bug where `googleworkspace_user.password` was always required, which would break on import, now it is only required on create. ([#125](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/125))
* directory: fixed permadiff on`googleworkspace_user.custom_schemas` ([#129](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/129))
* directory: fixed bug where different fields on `googleworkspace_user` would error if the value was empty. ([#133](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/133))
* directory: fixed bugs on `googleworkspace_group`, `googleworkspace_group_member` and `googleworkspace_org_unit` where changes made out of band would not refresh appropriately. ([#136](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/136))
* directory: changed nested `type` fields on `googleworkspace_org_user` from optional to required. ([#139](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/139))
* gmail: fixed bugs on `googleworkspace_gmail_send_as_alias` where changes made out of band would not refresh appropriately. ([#136](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/136))

## 0.4.0 (July 27, 2021)

FEATURES:

* **New Resource:** `googleworkspace_gmail_send_as_alias` ([#122](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/122))

IMPROVEMENTS:

* provider: allow impersonation to be unset when service account has sufficient role assignments. ([#115](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/115))

BUG FIXES:

* groups settings: fixed bug where consistency check was prone to failure/timeout. ([#119](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/119))
* directory: fixed bug where `googleworkspace_schema.fields.indexed` would break if it was nil. ([#108](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/108))
* chrome: fixed validation bug on message type schema values, ([#116](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/116))

## 0.3.0 (July 07, 2021)

FEATURES:

* **New Resource:** `googleworkspace_chrome_policy` ([#97](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/97))
* **New Resource:** `googleworkspace_domain_alias` ([#92](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/92))

* **New Datasource:**   `googleworkspace_chrome_policy_schema` ([#97](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/97))
* **New Datasource:**   `googleworkspace_domain_alias` ([#92](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/92))

IMPROVEMENTS:

* provider: added support for using the `GOOGLE_CREDENTIALS` environment variable for authentication ([#87](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/87))

BUG FIXES:

* all: added logging of the http requests/responses ([#93](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/93))

## 0.2.0 (June 21, 2021)

FEATURES:

* **New Resource:** `googleworkspace_org_unit` ([#63](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/63))
* **New Resource:** `googleworkspace_role` ([#66](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/66))
* **New Resource:** `googleworkspace_role_assignment` ([#66](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/66))

* **New Datasource:**   `googleworkspace_org_unit` ([#63](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/63))
* **New Datasource:**   `googleworkspace_role` ([#66](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/66))
* **New Datasource:**   `googleworkspace_privileges` ([#82](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/82))

## 0.1.0 (June 03, 2021)

FEATURES:

* **New Resource:** `googleworkspace_domain` ([#12](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/12))
* **New Resource:** `googleworkspace_group` ([#18](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/18))
* **New Resource:** `googleworkspace_group_member` ([#31](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/31))
* **New Resource:** `googleworkspace_group_settings` ([#29](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/29))
* **New Resource:** `googleworkspace_schema` ([#20](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/20))
* **New Resource:** `googleworkspace_user` ([#15](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/15))

* **New Datasource:**   `googleworkspace_domain` ([#12](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/12))
* **New Datasource:**   `googleworkspace_group` ([#18](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/18))
* **New Datasource:**   `googleworkspace_group_member` ([#31](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/31))
* **New Datasource:**   `googleworkspace_group_settings` ([#32](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/32))
* **New Datasource:**   `googleworkspace_schema` ([#20](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/20))
* **New Datasource:**   `googleworkspace_user` ([#15](https://github.com/hashicorp/terraform-provider-googleworkspace/issues/15))
