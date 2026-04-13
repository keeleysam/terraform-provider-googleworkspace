# CLAUDE.md - terraform-provider-googleworkspace

## What This Is

A Terraform provider for managing Google Workspace resources (users, groups, domains, org units, roles, Chrome policies, Gmail send-as aliases). Built with the Terraform Plugin SDK v2 and Google's Go API client libraries.

## Build & Test Commands

```bash
make build          # Format check + go install
make fmt            # Run gofmt on provider code
make lint           # Run golangci-lint
make test           # Unit tests (30s timeout)
make testacc        # Acceptance tests against real Google Workspace (requires env vars, 120m timeout)
make testacc TESTARGS='-run=TestAccResourceGroup_basic'  # Run a specific acceptance test
make generate       # Build + run go generate (formats examples, regenerates docs)
make sweep          # Clean up dangling test resources (destructive)
```

## Project Layout

```
main.go                              # Entry point, go:generate directives for docs
internal/provider/                   # ALL provider code lives here (single package: googleworkspace)
  provider.go                        # Provider schema, resource/datasource registry, configure()
  provider_config.go                 # apiClient struct, auth setup, service constructors
  services.go                        # GetXxxService() helpers that extract sub-services
  chrome_policy_common.go             # Shared chrome policy CRUD (Read, Import, Create), helpers, types
  resource_*.go                      # Resource implementations (18 resources)
  data_source_*.go                   # Data source implementations (17 data sources)
  *_test.go                          # Tests (acceptance + unit, co-located with implementations)
  *_sweeper_test.go                  # Test resource cleanup sweepers
  utils.go                           # Shared helpers (error handling, string conversion, etc.)
  datasource_helpers.go              # datasourceSchemaFromResourceSchema() and related helpers
  eventual_consistency.go            # ETag-based consistency checking for eventually consistent APIs
  retry_utils.go                     # retryTimeDuration() wrapper around SDK retry
  retry_transport.go                 # HTTP transport with Fibonacci backoff
  retry_predicates.go                # Error classification for retry decisions
  logging_transport.go               # HTTP logging with sensitive value scrubbing
examples/                            # Terraform HCL examples (used by doc generator)
docs/                                # Provider documentation (hand-edited for new resources)
```

## Architecture

### Provider Configuration

The provider is configured via `provider.go:New()` which registers all resources/data sources and calls `configure()` to build an `apiClient`. The `apiClient` (defined in `provider_config.go`) holds the authenticated HTTP client and config:

```go
type apiClient struct {
    client                *http.Client
    AccessToken           string
    BillingProject        string       // optional - sets X-Goog-User-Project for quota/billing
    ClientScopes          []string
    Credentials           string
    Customer              string       // required - Google Workspace customer ID
    ImpersonatedUserEmail string
    ServiceAccount        string
    UserAgent             string
}
```

Auth chain: access_token > credentials JSON > application default credentials. The HTTP client is built with three transport layers: MTLS auth -> scrubbed logging -> retry with Fibonacci backoff.

### Service Instantiation Pattern

Each API call goes through a two-step service resolution:

1. `client.NewDirectoryService()` / `client.NewChromePolicyService()` / etc. — creates the top-level Google API service
2. `GetGroupsService(directoryService)` / `GetUsersService(directoryService)` / etc. — extracts the specific sub-service

This pattern is used in every CRUD function. Always check `diags.HasError()` after each call.

### Resource CRUD Pattern

Every resource follows this structure:

```go
func resourceXxx() *schema.Resource {
    return &schema.Resource{
        Description:   "...",
        CreateContext: resourceXxxCreate,
        ReadContext:   resourceXxxRead,
        UpdateContext: resourceXxxUpdate,
        DeleteContext: resourceXxxDelete,
        Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
        Timeouts:      &schema.ResourceTimeout{Create: ..., Update: ...},
        Schema:        map[string]*schema.Schema{...},
    }
}
```

CRUD functions:
- **Create**: Build API object from `d.Get()`, call API with `retryTimeDuration()`, set `d.SetId()`, wait for eventual consistency via `consistencyCheck`, then call Read.
- **Read**: Get resource by `d.Id()`, call `handleNotFoundError()` on 404 (which clears the ID to remove from state), then `d.Set()` each field.
- **Update**: Check `d.HasChange()`/`d.HasChanges()` for changed fields, build partial update object, call API, wait for consistency, then call Read.
- **Delete**: Call API delete, use `handleNotFoundError()` for idempotency, set ID to empty.

### Eventual Consistency

Google Workspace APIs are eventually consistent. After Create/Update, the provider polls using ETag-based `consistencyCheck` (in `eventual_consistency.go`):

- Polls the resource with `IfNoneMatch(lastEtag)`
- Tracks ETag changes and consecutive consistent responses
- Requires `numConsistent` (4) consecutive unchanged ETags AND at least as many ETag changes as API mutations
- Has a `maxConsistent` fallback based on timeout duration

The retry wrapper `retryTimeDuration()` classifies errors: "timed out while waiting" is retryable (eventual consistency not yet reached), everything else is non-retryable.

### Data Source Pattern

Data sources reuse resource schemas via `datasourceSchemaFromResourceSchema()` which converts all fields to `Computed: true`. Then specific lookup fields are made `Optional` with `addExactlyOneOfFieldsToSchema()` (e.g., look up a group by `id` or `email`).

### Error Handling & Retry

- `handleNotFoundError()` — on HTTP 404, clears resource ID (removes from state) instead of erroring
- `isApiErrorWithCode()` — checks Google API errors for specific HTTP codes
- `isNonFatalDeleteError()` — suppresses known-safe 400 errors during Chrome Policy OU deletion (apps uninstalled, nothing to inherit, target gone)
- HTTP retry transport (`retry_transport.go`) handles transient errors: 429 rate limit, 500/502/503, network timeouts, connection resets, io.EOF
- Fibonacci backoff starting at 500ms, max 90 seconds

### Utility Functions (utils.go)

- `SnakeToCamel()` / `CameltoSnake()` — convert between Terraform snake_case and Google API camelCase field names
- `expandInterfaceObjects()` / `flattenInterfaceObjects()` — transform nested objects between Terraform state format and API format, handling the snake/camel conversion
- `Nprintf()` — named printf for test configs, e.g. `Nprintf("email = %{email}@%{domain}", vals)`
- `pathOrContents()` — loads a file path or treats the string as inline content (used for credentials)
- `listOfInterfacestoStrings()`, `stringInSlice()`, `isEmail()` — common collection/validation helpers

### Chrome Policy Architecture

Chrome policies use a shared CRUD pattern in `chrome_policy_common.go`:

- **`chromePolicyCreateCommon()`** — validates, expands, groups by additional_target_keys, then calls a `chromePolicyBatchModifyFunc` callback. Org units use `orgUnitBatchModify` (batches all policies in one call); groups use `groupBatchModify` (one policy per call as API workaround).
- **`chromePolicyReadCommon()`** — resolves each policy via the Resolve API, parameterized by target kind.
- **`chromePolicyImportCommon()`** — parses import ID, validates schemas exist on target.
- **`groupAdditionalTargetKeys()`** — parses the `additional_target_keys` schema attribute into grouped key/value pairs.

Org-unit-specific logic (`resource_chrome_policy.go`):
- **Update**: Inherit-then-recreate pattern using `orgUnitBatchInherit` + `isNonFatalDeleteError`
- **Delete**: Same `orgUnitBatchInherit` pattern

Group-specific logic (`resource_chrome_group_policy.go`):
- **Update**: Create-first-then-delete-removed using `computeRemovedPolicies` + `deleteChromePoliciesFromGroup`
- **Delete**: `deleteChromePoliciesFromGroup` using `Groups.BatchDelete` with "apps not installed" error suppression

`chrome_policy_set` is a separate authoritative resource that manages all policies matching a wildcard filter — it has its own independent CRUD in `resource_chrome_policy_set.go`.

## Resources & Data Sources

### Resources (18)
| Resource | API | Notes |
|---|---|---|
| `googleworkspace_user` | Admin Directory | Largest resource (~1800 lines), complex nested schemas (name, emails, phones, custom schemas) |
| `googleworkspace_group` | Admin Directory + Cloud Identity | Supports security_group label (immutable once set) via Cloud Identity API |
| `googleworkspace_group_member` | Admin Directory | Single member management |
| `googleworkspace_group_members` | Admin Directory | Bulk member management |
| `googleworkspace_group_settings` | Groups Settings API | Group configuration (who can post, join, etc.) |
| `googleworkspace_group_dynamic` | Cloud Identity | Dynamic groups with membership queries |
| `googleworkspace_domain` | Admin Directory | Domain management |
| `googleworkspace_domain_alias` | Admin Directory | Domain alias management |
| `googleworkspace_org_unit` | Admin Directory | Organization units |
| `googleworkspace_role` | Admin Directory | Custom admin roles |
| `googleworkspace_role_assignment` | Admin Directory | Assign roles to users |
| `googleworkspace_schema` | Admin Directory | Custom user schema definitions with nested fields |
| `googleworkspace_gmail_send_as_alias` | Gmail API | Requires per-user impersonation (creates new client per user) |
| `googleworkspace_chrome_policy` | Chrome Policy API | Org-unit-scoped Chrome policies (shared CRUD via chrome_policy_common.go) |
| `googleworkspace_chrome_group_policy` | Chrome Policy API | Group-scoped Chrome policies (shared CRUD via chrome_policy_common.go) |
| `googleworkspace_chrome_policy_set` | Chrome Policy API | Authoritative policy management — manages all policies matching a filter, removing unmanaged ones |
| `googleworkspace_chrome_policy_file` | Chrome Policy API | File uploads (wallpapers, etc.) for Chrome policies |
| `googleworkspace_chrome_policy_group_priority_ordering` | Chrome Policy API | Policy priority ordering across groups |

### Data Sources (17)
Singular lookups (by ID or key field) and plural list versions for users, groups, group members, domains, org units, roles, schemas, Chrome policy schemas, Chrome policy group priority ordering, privileges.

## Testing Conventions

### Acceptance Tests
- Require real Google Workspace environment
- Required env vars: `GOOGLEWORKSPACE_CUSTOMER_ID`, `GOOGLEWORKSPACE_DOMAIN`, `GOOGLEWORKSPACE_IMPERSONATED_USER_EMAIL`
- Test names: `TestAccResourceXxx_basic`, `TestAccResourceXxx_full`, `TestAccDataSourceXxx_basic`
- Use `t.Parallel()` for concurrent execution
- Test resources prefixed with `tf-test-` for sweeper identification
- Config helpers use `Nprintf()` with `map[string]interface{}` for named substitution
- Import state tests verify import by both ID and key fields, ignoring `etag`

### Sweepers
Files named `*_sweeper_test.go` clean up resources matching `tf-test-` prefix. Run with `make sweep`.

### Test Structure
```go
func TestAccResourceXxx_basic(t *testing.T) {
    t.Parallel()
    domainName := os.Getenv("GOOGLEWORKSPACE_DOMAIN")
    if domainName == "" {
        t.Skip("GOOGLEWORKSPACE_DOMAIN needs to be set to run this test")
    }
    testVals := map[string]interface{}{
        "domainName": domainName,
        "email":      fmt.Sprintf("tf-test-%s", acctest.RandString(10)),
    }
    resource.Test(t, resource.TestCase{
        PreCheck:          func() { testAccPreCheck(t) },
        ProviderFactories: providerFactories,
        Steps: []resource.TestStep{
            {Config: testAccResourceXxx_basic(testVals)},
            {ResourceName: "googleworkspace_xxx.name", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"etag"}},
        },
    })
}
```

## Adding a New Resource Checklist

1. Create `internal/provider/resource_xxx.go` with `resourceXxx()` returning `*schema.Resource` with CRUD context functions
2. Register in `provider.go` ResourcesMap: `"googleworkspace_xxx": resourceXxx()`
3. Add service getter in `services.go` if a new sub-service is needed
4. If a data source is needed, create `data_source_xxx.go` using `datasourceSchemaFromResourceSchema(resourceXxx().Schema)` and register in DataSourcesMap
5. Create `resource_xxx_test.go` with acceptance tests (basic, full, update steps + import verification)
6. Create `resource_xxx_sweeper_test.go` for test cleanup
7. Add example in `examples/resources/googleworkspace_xxx/resource.tf`
8. Add or generate docs in `docs/resources/xxx.md`

## Key Dependencies

- `github.com/hashicorp/terraform-plugin-sdk/v2` — Terraform provider SDK
- `google.golang.org/api/admin/directory/v1` — Admin Directory API (users, groups, domains, org units, roles, schemas)
- `google.golang.org/api/chromepolicy/v1` — Chrome Policy API
- `google.golang.org/api/cloudidentity/v1` — Cloud Identity API (dynamic groups, security labels)
- `google.golang.org/api/groupssettings/v1` — Groups Settings API
- `google.golang.org/api/gmail/v1` — Gmail API (send-as aliases)
- `golang.org/x/oauth2` — OAuth2 authentication

## Common Gotchas

- **Eventual consistency**: All Create/Update operations must poll for consistency before returning. Use the `consistencyCheck` + `retryTimeDuration` pattern.
- **ETag handling**: Many resources use ETags for optimistic concurrency. Always `d.Set("etag", ...)` in Read and always `ImportStateVerifyIgnore: []string{"etag"}` in tests.
- **Gmail service impersonation**: The Gmail send-as-alias resource creates a _new_ `apiClient` per user because the OAuth token must impersonate the specific user whose alias is being managed.
- **404 in Read/Delete**: Always use `handleNotFoundError()` — it removes the resource from state rather than erroring, which is the expected Terraform behavior for deleted resources.
- **Schema descriptions use Markdown** (`schema.DescriptionKind = schema.StringMarkdown` in `init()`).
- **Documentation**: Older docs were auto-generated via `make generate`, but newer resources (chrome_policy_set, chrome_policy_schemas, org_units, etc.) have hand-written docs. Either approach works — just ensure every registered resource/data source has a corresponding `docs/` file.
- **snake_case <-> camelCase**: When mapping between Terraform schema fields and Google API objects, use `SnakeToCamel()`/`CameltoSnake()` or the `expandInterfaceObjects()`/`flattenInterfaceObjects()` helpers for nested structures.
