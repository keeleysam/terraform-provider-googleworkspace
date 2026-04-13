# Terraform Provider Google Workspace

This is a community fork of the [HashiCorp Google Workspace provider](https://github.com/hashicorp/terraform-provider-googleworkspace), which has been archived and is no longer maintained. This fork is based on [Yohan460's fork](https://github.com/Yohan460/terraform-provider-googleworkspace) and incorporates contributions from multiple maintainers.

This provider allows you to manage Google Workspace resources including users, groups, domains, org units, roles, Chrome policies, and Gmail send-as aliases using Terraform.

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.24 (for building from source)

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command or `make build`:
```sh
make build
```

## Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

## Developing the Provider

You'll need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `make generate`.

To run unit tests:

```sh
make test
```

To run acceptance tests (creates real resources, requires Google Workspace credentials):

```sh
make testacc
```

Required environment variables for acceptance tests:
- `GOOGLEWORKSPACE_CUSTOMER_ID`
- `GOOGLEWORKSPACE_DOMAIN`
- `GOOGLEWORKSPACE_IMPERSONATED_USER_EMAIL`

## Lineage

* Original provider by [HashiCorp](https://github.com/hashicorp/terraform-provider-googleworkspace) (archived)
* Inspired by [DeviaVir/terraform-provider-gsuite](https://github.com/DeviaVir/terraform-provider-gsuite) by [Chase](https://github.com/DeviaVir)
* Continued by [Yohan460](https://github.com/Yohan460/terraform-provider-googleworkspace)
* This fork!