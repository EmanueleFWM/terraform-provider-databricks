# Contributing to Databricks Terraform Provider

---

- [Issues for new contributors](#issues-for-new-contributors)
- [Contribution Workflow](#contribution-workflow)
- [Changelog](#changelog)
- [Contributing to Databricks Terraform Provider](#contributing-to-databricks-terraform-provider)
- [Installing from source](#installing-from-source)
- [Contributing documentation](#contributing-documentation)
- [Developing provider](#developing-provider)
- [Debugging](#debugging)
- [Adding a new resource](#adding-a-new-resource)
- [Testing](#testing)
- [Code conventions](#code-conventions)
- [Linting](#linting)
- [Developing with Visual Studio Code Devcontainers](#developing-with-visual-studio-code-devcontainers)

We happily welcome contributions to the Databricks Terraform Provider. We use GitHub Issues to track community reported issues and GitHub Pull Requests for accepting changes.

## Issues for new contributors

New contributors should look for the following tags when searching for a first contribution to the Databricks code base. We strongly recommend that new contributors tackle "good first issue" projects first; this helps the contributor become familiar with the contribution workflow, and for the core devs to become acquainted with the contributor.

[good first issue](https://github.com/databricks/terraform-provider-databricks/labels/good%20first%20issue)

## Contribution Workflow

Code contributions—bug fixes, new development, test improvement — all follow a GitHub-centered workflow. To participate in Databricks Terraform provider development, set up a GitHub account. Then:

1. Fork the repo you plan to work on. Go to the project repo page and use the Fork button. This will create a copy of the repo, under your username. (For more details on how to fork a repository see [this guide](https://help.github.com/articles/fork-a-repo/).)

1. Clone down the repo to your local system.

    ```bash
    git clone git@github.com:YOUR_USER_NAME/terraform-provider-databricks.git
    ```

1. Create a new branch to hold your work.

    ```bash
    git checkout -b new-branch-name
    ```

1. Work on your new code. Write and run tests.

1. Commit your changes.

    ```bash
    git add -A

    git commit -m "commit message here"
    ```

1. Document your changes in the `NEXT_CHANGELOG.md` under the appropriate heading. See the [Changelog](#changelog) section for more details.

1. Push your changes to your GitHub repo.

    ```bash
    git push origin branch-name
    ```

1. Open a Pull Request (PR). Go to the original project repo on GitHub. There will be a message about your recently pushed branch, asking if you would like to open a pull request. Follow the prompts, compare across repositories, and submit the PR. This will send an email to the committers. You may want to consider sending an email to the mailing list for more visibility. (For more details, see the [GitHub guide on PRs](https://help.github.com/articles/creating-a-pull-request-from-a-fork).)

Maintainers and other contributors will review your PR. Please participate in the conversation, and try to make any requested changes. Once the PR is approved, the code will be merged.

Additional git and GitHub resources:

[Git documentation](https://git-scm.com/documentation)
[Git development workflow](https://docs.scipy.org/doc/numpy/dev/development_workflow.html)
[Resolving merge conflicts](https://help.github.com/articles/resolving-a-merge-conflict-using-the-command-line/)

## Changelog

All PRs that introduce a new feature, fix a bug, improve documentation, or change the behavior of the exporter must include a description of the change in the `NEXT_CHANGELOG.md` file. This file is prepended to the `CHANGELOG.md` file when a new release is created, then cleared out for the next release. Add your changelog entry to the appropriate section of the `NEXT_CHANGELOG.md` file.

If the proposed change has no user-facing impact or does not require an additional changelog entry (e.g. correcting a typo in the documentation), do not add an entry to the `NEXT_CHANGELOG.md` file, and add the text `NO_CHANGELOG=true` to your PR description.

The entries of the changelog must have the following format:

```
 * <Summary of the change> ([#<PR number>](<PR link>)).

   <Optional additional information>
```

For example:

```
 * Added support for new feature ([#123](https://github.com/databricks/terraform-provider-databricks/pull/123)).
```

Include additional information to provide context for the change, if necessary. For example, you may include links to the Databricks documentation website, the Terraform documentation website, examples, or other relevant resources.

The `NEXT_CHANGELOG.md` file also determines the next version to be released. The version number in the `NEXT_CHANGELOG.md` is automatically set to the next minor version. If there are any new features or breaking changes, leave this as is. Otherwise, set the version to the next patch version. You can see the current version at the top of the `CHANGELOG.md` file.

## Installing from source

On MacOS X, you can install GoLang through `brew install go`, on Debian-based Linux, you can install it by `sudo apt-get install golang -y`.

```bash
git clone https://github.com/databricks/terraform-provider-databricks.git
cd terraform-provider-databricks
make install
```

Most likely, `terraform init -upgrade -verify-plugins=false -lock=false` would be a very great command to use.

## Contributing documentation

Make sure you have [`terrafmt`](https://github.com/katbyte/terrafmt) installed to be able to run `make fmt-docs`

```bash
go install github.com/katbyte/terrafmt@latest
```

All documentation contributions should be as detailed as possible and follow the [required format](https://www.terraform.io/registry/providers/docs). The following additional checks must also be valid:

- `make fmt-docs` to make sure code examples are consistent
- Correct rendering with Terraform Registry Doc Preview Tool - <https://registry.terraform.io/tools/doc-preview>
- Cross-linking integrity between markdown files. Pay special attention, when resource doc refers to data doc or guide.

## Developing provider

In order to simplify development workflow, you should use [dev_overrides](https://www.terraform.io/cli/config/config-file#development-overrides-for-provider-developers) section in your `~/.terraformrc` file. Please run `make build` and replace "provider-binary" with the path to `terraform-provider-databricks` executable in your current working directory:

```bash
$ cat ~/.terraformrc
provider_installation {
   dev_overrides {
     "databricks/databricks" = "provider-binary"
   }
   direct {}
}
```

After installing the necessary software for building provider from sources, you should be able to run `make coverage` to run the tests and see the coverage.

## Developing Resources or Data Sources using Plugin Framework

### Package organization for Providers
We are migrating the resource from SDKv2 to Plugin Framework provider and hence both of them exist in the codebase. For uniform code convention, readability and development, they are organized in the `internal/providers` directory under root as follows:
- `providers`: Contains the changes that `depends` on both internal/providers/sdkv2 and internal/providers/pluginfw packages, eg: `GetProviderServer`.
- `common`: Contains the changes `used by` both internal/providers/sdkv2 and internal/providers/pluginfw packages, eg: `ProviderName`.
- `pluginfw`: Contains the changes specific to Plugin Framework. This package shouldn't depend on sdkv2 or common.
- `sdkv2`: Contains the changes specific to SDKv2. This package shouldn't depend on pluginfw or common.

### Adding a new resource
1. Check if the directory for this particular resource exists under `internal/providers/pluginfw/products`, if not create the directory eg: `cluster`, `volume` etc... Please note: Resources and Data sources are organized under the same package for that service.
2. Create a file with resource_resource-name.go and write the CRUD methods, schema for that resource. For reference, please take a look at existing resources eg: `resource_app.go`.
  - Make sure to set the user agent in all the CRUD methods.
  - In the `Metadata()`, use the method `GetDatabricksProductionName()`.
  - In the `Schema()` method, import the appropriate struct from the `internal/service/{package}_tf` package and use the `ResourceStructToSchema` method to convert the struct to schema. Use the struct that does not have the `_SdkV2` suffix. The schema for the struct is automatically generated and maintained within the `ApplySchemaCustomizations` method of that struct. If you need to customize the schema further, pass in a `CustomizableSchema` to `ResourceStructToSchema` and customize the schema there. If you need to use a manually crafted struct in place of the auto-generated one, you must implement the `ApplySchemaCustomizations` method in a similar way.
3. Create a file with `resource_resource-name_acc_test.go` and add integration tests here.
4. Create a file with `resource_resource-name_test.go` and add unit tests here. Note: Please make sure to abstract specific method of the resource so they are unit test friendly and not testing internal part of terraform plugin framework library. You can compare the diagnostics, for example: please take a look at: `data_cluster_test.go`
5. Add the resource under `internal/providers/pluginfw/pluginfw.go` in `Resources()` method. Please update the list so that it stays in alphabetically sorted order.
6. Create a PR and send it for review.

### Adding a new data source
1. Check if the directory for this particular datasource exists under `internal/providers/pluginfw/products`, if not create the directory eg: `cluster`, `volume` etc... Please note: Resources and Data sources are organized under the same package for that service.
2. Create a file with `data_resource-name.go` and write the CRUD methods, schema for that data source. For reference, please take a look at existing data sources eg: `data_cluster.go`. Make sure to set the user agent in the READ method. In the `Metadata()`, if the resource is to be used as default, use the method `GetDatabricksProductionName()` else use `GetDatabricksStagingName()` which suffixes the name with `_pluginframework`.
3. Create a file with `data_resource-name_acc_test.go` and add integration tests here.
4. Create a file with `data_resource-name_test.go` and add unit tests here. Note: Please make sure to abstract specific method of the resource so they are unit test friendly and not testing internal part of terraform plugin framework library. You can compare the diagnostics, for example: please take a look at: `data_cluster_test.go`
5. Add the resource under `internal/providers/pluginfw/pluginfw.go` in `DataSources()` method. Please update the list so that it stays in alphabetically sorted order.
6. Create a PR and send it for review.

### Migrating resource to plugin framework
There must not be any behaviour change or schema change when migrating a resource or data source to either Go SDK or Plugin Framework.
- Please make sure there are no breaking differences due to changes in schema by running: `make diff-schema`.
- Integration tests shouldn't require any major changes.

By default, `ResourceStructToSchema` will convert a `types.List` field to a `ListAttribute` or `ListNestedAttribute`. For resources or data sources migrated from the SDKv2, `ListNestedBlock` must be used for such fields. To do this, use the `_SdkV2` variant from the `internal/service/{package}_tf` package when defining the resource schema and when interacting with the plan, config and state. Additionally, in the `Schema()` method, call `cs.ConfigureAsSdkV2Compatible()` in the `ResourceStructToSchema` callback:
```go
resp.Schema = tfschema.ResourceStructToSchema(ctx, Resource_SdkV2{}, func(c tfschema.CustomizableSchema) tfschema.CustomizableSchema {
    cs.ConfigureAsSdkV2Compatible()
    // Add any additional configuration here
    return cs
})
```

### Code Organization
Each resource and data source should be defined in package `internal/providers/plugnifw/products/<resource>`, e.g.: `internal/providers/plugnifw/products/volume` package will contain both resource, data sources and other utils specific to volumes. Tests (both unit and integration tests) will also remain in this package.

Note: Only Docs will stay under root docs/ directory.


### Code Conventions
1. Make sure the resource or data source implemented is of the right type:
    ```golang
    var _ resource.ResourceWithConfigure = &QualityMonitorResource{}
    var _ datasource.DataSourceWithConfigure = &VolumesDataSource{}
    ```
2. To get the databricks client, `func (*common.DatabricksClient).GetWorkspaceClient()` or `func (*common.DatabricksClient).GetAccountClient()` will be used instead of directly using the underlying `WorkspaceClient()`, `AccountClient()` functions respectively.
3. Any method that returns only diagnostics should be called inline while appending diagnostics in response. Example:
    ```golang
    resp.Diagnostics.Append(req.Plan.Get(ctx, &monitorInfoTfSDK)...)
    if resp.Diagnostics.HasError() {
        return
    }
    ```
    is preferred over the following:
    ```golang
    diags := req.Plan.Get(ctx, &monitorInfoTfSDK)
    if diags.HasError() {
        resp.Diagnostics.Append(diags...)
        return
    }
    ```
4. Any method returning an error should directly be followed by appending that to the diagnostics.
    ```golang
    err := method()
    if err != nil {
        resp.Diagnostics.AddError("message", err.Error())
        return
    }
    ```
5. Any method returning a value alongside Diagnostics should also directly be followed by appending that to the diagnostics.


## Debugging

**TF_LOG_PROVIDER=DEBUG terraform apply** allows you to see the internal logs from `terraform apply`.

You can [run provider in a debug mode](https://www.terraform.io/plugin/sdkv2/debugging#running-terraform-with-a-provider-in-debug-mode) from VScode IDE by launching `Debug Provider` run configuration and invoking `terraform apply` with `TF_REATTACH_PROVIDERS` environment variable.

## Adding a new resource

Boilerplate for data sources could be generated via `go run provider/gen/main.go -name mws_workspaces -package mws -is-data -dry-run=false`.

The general process for adding a new resource is:

*Define the resource models.* The models for a resource are `struct`s defining the schemas of the objects in the Databricks REST API. Define structures used for multiple resources in a common `models.go` file; otherwise, you can define these directly in your resource file. An example model:

```go
type Field struct {
 A string `json:"a,omitempty"`
 AMoreComplicatedName int `json:"a_more_complicated_name,omitempty"`
}

type Example struct {
 ID string `json:"id"`
 TheField *Field `json:"the_field"`
 AnotherField bool `json:"another_field"`
 Filters []string `json:"filters" tf:"optional"`
}
```

Some interesting points to note here:

- Use the `json` tag to determine the serde properties of the field. The allowed tags are defined here: <https://go.googlesource.com/go/+/go1.16/src/encoding/json/encode.go#158>
- Use the custom `tf` tag indicates properties to be annotated on the Terraform schema for this struct. Supported values are:
  - `optional` for optional fields
  - `computed` for computed fields
  - `alias:X` to use a custom name in HCL for a field
  - `default:X` to set a default value for a field
  - `max_items:N` to set the maximum number of items for a multi-valued parameter
  - `slice_set` to indicate that a the parameter should accept a set instead of a list
  - `sensitive` to mark a field as sensitive and prevent Terraform from showing its value in the plan or apply output
  - `force_new` to indicate a change in this value requires the replacement (destroy and create) of the resource
  - `suppress_diff` to allow comparison based on something other than primitive, list or map equality, either via a `CustomizeDiffFunc`, or the default diff for the type of the schema
- Do not use bare references to structs in the model; rather, use pointers to structs. Maps and slices are permitted, as well as the following primitive types: int, int32, int64, float64, bool, string.
See `typeToSchema` in `common/reflect_resource.go` for the up-to-date list of all supported field types and values for the `tf` tag.

*Define the Terraform schema.* This is made easy for you by the `StructToSchema` method in the `common` package, which converts your struct automatically to a Terraform schema, accepting also a function allowing the user to post-process the automatically generated schema, if needed.

```go
var exampleSchema = common.StructToSchema(Example{}, func(m map[string]*schema.Schema) map[string]*schema.Schema { return m })
```

*Define the API client for the resource.* You will need to implement create, read, update, and delete functions.

```go
type ExampleApi struct {
 client *common.DatabricksClient
 ctx    context.Context
}

func NewExampleApi(ctx context.Context, m interface{}) ExampleApi {
 return ExampleApi{m.(*common.DatabricksClient), ctx}
}

func (a ExampleApi) Create(e Example) (string, error) {
 var id string
 err := a.client.Post(a.ctx, "/example", e, &id)
 return id, err
}

func (a ExampleApi) Read(id string) (e Example, err error) {
 err = a.client.Get(a.ctx, "/example/"+id, nil, &e)
 return
}

func (a ExampleApi) Update(id string, e Example) error {
 return a.client.Put(a.ctx, "/example/"+string(id), e)
}

func (a ExampleApi) Delete(id string) error {
 return a.client.Delete(a.ctx, "/pipelines/"+id, nil)
}
```

*Define the Resource object itself.* This is made quite simple by using the `toResource` function defined on the `Resource` type in the `common` package. A simple example:

```go
func ResourceExample() *schema.Resource {
 return common.Resource{
  Schema: exampleSchema,
  Create: func(ctx context.Context, d *schema.ResourceData, c *common.DatabricksClient) error {
   var e Example
   common.DataToStructPointer(d, exampleSchema, &e)
   id, err := NewExampleApi(ctx, c).Create(e)
   if err != nil {
    return err
   }
   d.SetId(string(id))
   return nil
  },
  Read: func(ctx context.Context, d *schema.ResourceData, c *common.DatabricksClient) error {
   i, err := NewExampleApi(ctx, c).Read(d.Id())
   if err != nil {
    return err
   }
   return common.StructToData(i.Spec, exampleSchema, d)
  },
  Update: func(ctx context.Context, d *schema.ResourceData, c *common.DatabricksClient) error {
   var e Example
   common.DataToStructPointer(d, exampleSchema, &e)
   return NewExampleApi(ctx, c).Update(d.Id(), e)
  },
  Delete: func(ctx context.Context, d *schema.ResourceData, c *common.DatabricksClient) error {
   return NewExampleApi(ctx, c).Delete(d.Id())
  },
 }.ToResource()
}
```

*Add the resource to the top-level provider.* Simply add the resource to the provider definition in `provider/provider.go`.

*Write unit tests for your resource.* To write your unit tests, you can make use of `ResourceFixture` and `HTTPFixture` structs defined in the `qa` package. This starts a fake HTTP server, asserting that your resource provider generates the correct request for a given HCL template body for your resource. Update tests should have `InstanceState` field in order to test various corner-cases, like `ForceNew` schemas. It's possible to expect fixture to require new resource by specifying `RequiresNew` field. With the help of `qa.ResourceCornerCases` and `qa.ResourceFixture` one can achieve 100% code coverage for all of the new code.

A simple example:

```go
func TestExampleCornerCases(t *testing.T) {
 qa.ResourceCornerCases(t, ResourceExample())
}

func TestExampleResourceCreate(t *testing.T) {
 qa.ResourceFixture{
  Fixtures: []qa.HTTPFixture{
   {
    Method:          "POST",
    Resource:        "/api/2.0/example",
    ExpectedRequest: Example{
     TheField: Field{
      A: "test",
     },
    },
    Response: map[string]interface{} {
     "id": "abcd",
     "the_field": map[string]interface{} {
      "a": "test",
     },
    },
   },
   {
    Method:   "GET",
    Resource: "/api/2.0/example/abcd",
    Response: map[string]interface{}{
     "id":    "abcd",
     "the_field": map[string]interface{} {
      "a": "test",
     },
    },
   },
  },
  Create:   true,
  Resource: ResourceExample(),
  HCL: `the_field {
   a = "test"
  }`,
 }.ApplyNoError(t)
}
```

*Write acceptance tests.* These are E2E tests which run terraform against the live cloud and Databricks APIs. For these, you can use the `Step` helpers defined in the `internal/acceptance` package. An example:

```go
func TestAccSecretAclResource(t *testing.T) {
 WorkspaceLevel(t, Step{
  Template: `
  resource "databricks_group" "ds" {
   display_name = "data-scientists-{var.RANDOM}"
  }
  resource "databricks_secret_scope" "app" {
   name = "app-{var.RANDOM}"
  }
  resource "databricks_secret_acl" "ds_can_read_app" {
   principal = databricks_group.ds.display_name
   permission = "READ"
   scope = databricks_secret_scope.app.name
  }`,
  Check: func(s *terraform.State) error {
   w := databricks.Must(databricks.NewWorkspaceClient())

   ctx := context.Background()
   me, err := w.CurrentUser.Me(ctx)
   require.NoError(t, err)

   scope := s.RootModule().Resources["databricks_secret_scope.app"].Primary.ID
   acls, err := w.Secrets.ListAclsByScope(ctx, scope)
   require.NoError(t, err)
   assert.Equal(t, 2, len(acls.Items))
   m := map[string]string{}
   for _, acl := range acls.Items {
    m[acl.Principal] = string(acl.Permission)
   }

   group := s.RootModule().Resources["databricks_group.ds"].Primary.Attributes["display_name"]
   require.Contains(t, m, group)
   assert.Equal(t, "READ", m[group])
   assert.Equal(t, "MANAGE", m[me.UserName])
   return nil
  },
 })
}
```

## Integration Testing

Integration tests are run as part of every PR made to the Databricks Terraform provider. Tests are run against AWS, Azure, and GCP infrastructure, in workspaces and accounts, and in Unity Catalog and non-Unity Catalog environments.

Tests, where possible, should be entirely self-contained. They should not depend on external or preprovisioned resources, and they should not make changes to any existing resources.

There is a default filter for tests based on the test name:
- Tests beginning with `TestAcc` are run in non-UC workspace enviroments across all clouds.
- Tests beginning with `TestMwsAcc` are run in non-UC account environments across all clouds.
- Tests beginning with `TestUcAcc` are run in UC workspace and account enviroments across all clouds.

In general, all PRs that affect the behavior of the provider should include at least an integration test to ensure that any assumptions made about the Databricks platform in the implementation of the feature are correct. Integration tests must be added in the same directory as the resource or data source being tested. The name of the file should be `<resource_name>_test.go` for resources or `data_<data_source_name>_test.go` for data sources.

## Code conventions

- Files should not be larger than 600 lines
- Single function should fit to be seen on 13" screen: no more than 40 lines of code. Only exception to this rule is `*_test.go` files.
- There should be no unnecessary package exports: no structs & types with leading capital letter, unless they are of value outside of the package.
- `fmt.Sprintf` with more than 4 placeholders is considered too complex to maintain. Should be avoided at all cost. Use `qa.EnvironmentTemplate(t, "This is {env.DATABRICKS_HOST} with {var.RANDOM} name.")` instead
- Import statements should all be first ordered by "GoLang internal", "Vendor packages" and then "current provider packages". Within those sections imports must follow alphabetical order.

## Linting

Please use makefile for linting. If you run `staticcheck` by itself it will fail due to different tags containing same functions.
So please run `make lint` instead.

## Developing with Visual Studio Code Devcontainers

NOTE: This use of devcontainers for terraform-provider-databricks development is **experimental** and not officially supported by Databricks

This project has configuration for working with [Visual Studio Code Devcontainers](https://code.visualstudio.com/docs/remote/containers) - this allows you to containerize your development prerequisites (e.g. golang, terraform). To use this you will need [Visual Studio Code](https://code.visualstudio.com/) and [Docker](https://www.docker.com/products/docker-desktop).

To get started, clone this repo and open the folder with Visual Studio Code. If you don't have the [Remote Development extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.vscode-remote-extensionpack) then you should be prompted to install it.

Once the folder is loaded and the extension is installed you should be prompted to re-open the folder in a devcontainer. This will built and run the container image with the correct tools (and versions) ready to start working on and building the code. The in-built terminal will launch a shell inside the container for running `make` commands etc.

See the docs for more details on working with [devcontainers](https://code.visualstudio.com/docs/remote/containers).
