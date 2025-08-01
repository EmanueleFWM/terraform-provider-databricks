package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/databricks/databricks-sdk-go/apierr"
	sdk_uc "github.com/databricks/databricks-sdk-go/service/catalog"
	sdk_compute "github.com/databricks/databricks-sdk-go/service/compute"
	sdk_dashboards "github.com/databricks/databricks-sdk-go/service/dashboards"
	"github.com/databricks/databricks-sdk-go/service/iam"
	sdk_jobs "github.com/databricks/databricks-sdk-go/service/jobs"
	"github.com/databricks/databricks-sdk-go/service/ml"
	"github.com/databricks/databricks-sdk-go/service/pipelines"
	"github.com/databricks/databricks-sdk-go/service/serving"
	"github.com/databricks/databricks-sdk-go/service/settings"
	"github.com/databricks/databricks-sdk-go/service/sharing"
	sdk_sql "github.com/databricks/databricks-sdk-go/service/sql"
	sdk_vs "github.com/databricks/databricks-sdk-go/service/vectorsearch"
	sdk_workspace "github.com/databricks/databricks-sdk-go/service/workspace"

	"github.com/databricks/terraform-provider-databricks/aws"
	"github.com/databricks/terraform-provider-databricks/clusters"
	"github.com/databricks/terraform-provider-databricks/commands"
	"github.com/databricks/terraform-provider-databricks/common"
	"github.com/databricks/terraform-provider-databricks/internal/service/workspace_tf"
	"github.com/databricks/terraform-provider-databricks/qa"
	"github.com/databricks/terraform-provider-databricks/repos"
	"github.com/databricks/terraform-provider-databricks/scim"
	tf_sql "github.com/databricks/terraform-provider-databricks/sql"
	tf_workspace "github.com/databricks/terraform-provider-databricks/workspace"

	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/stretchr/testify/assert"
)

// nolint
func getJSONObject(filename string) any {
	var obj map[string]any
	data, err := os.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(data, &obj)
	if err != nil {
		fmt.Printf("[ERROR] error! file=%s err=%v\n", filename, err)
		fmt.Printf("[ERROR] data=%s\n", string(data))
	}
	return obj
}

func workspaceConfKeysToURL() string {
	keys := make([]string, 0, len(workspaceConfKeys))
	for k := range workspaceConfKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, "%2C")
}

func (ic *importContext) setClientsForTests() {
	ic.accountLevel = ic.Client.Config.IsAccountClient()
	if ic.accountLevel {
		ic.meAdmin = true
		ic.accountClient, _ = ic.Client.AccountClient()
	} else {
		ic.workspaceClient, _ = ic.Client.WorkspaceClient()
	}
}

func TestImportingMounts(t *testing.T) {
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Me",
				Response: scim.User{},
			},
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/2.0/clusters/list",
				Response: clusters.ClusterList{
					Clusters: []clusters.ClusterInfo{
						{
							ClusterName: "terraform-mount",
							ClusterID:   "mount",
						},
						{
							ClusterName: "terraform-mount-shard-s3-access",
							ClusterID:   "mount",
						},
					},
				},
			},
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/2.0/clusters/get?cluster_id=mount",
				Response: clusters.ClusterInfo{
					State:       "RUNNING",
					ClusterID:   "mount",
					ClusterName: "dummy",
				},
			},
			{
				Method:       "POST",
				ReuseRequest: true,
				Resource:     "/api/1.2/contexts/create",
				Response: commands.Command{
					ID: "context",
				},
			},
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/1.2/contexts/status?clusterId=mount&contextId=context",
				Response: commands.Command{
					Status: "Running",
				},
			},
			{
				Method:       "POST",
				ReuseRequest: true,
				Resource:     "/api/1.2/commands/execute",
				Response: commands.Command{
					ID: "run",
				},
			},
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/1.2/commands/status?clusterId=mount&commandId=run&contextId=context",
				Response: commands.Command{
					Status: "Finished",
					Results: &common.CommandResults{
						ResultType: "text",
						Data: `{"foo": "s3a://foo", "bar": "abfss://bar@baz.com/thing", "third": "adls://foo3.bar.com/path", "fourth":"wasbs://bar4@baz4.com/dir", "fifth":"gs://foo5", "sixth":"abc://foo5"}
					and some chatty messages`,
					},
				},
			},
			{
				Method:       "POST",
				Resource:     "/api/1.2/contexts/destroy",
				ReuseRequest: true,
			},
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/2.0/instance-profiles/list",
				Response: aws.InstanceProfileList{
					InstanceProfiles: []aws.InstanceProfileInfo{
						{
							InstanceProfileArn: "arn:aws:iam::12345:instance-profile/shard-s3-access",
						},
					},
				},
			},
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/2.1/clusters/spark-versions",
				Response: sdk_compute.GetSparkVersionsResponse{
					Versions: []sdk_compute.SparkVersion{
						{
							Key: "Foo LTS",
						},
					},
				},
			},
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/2.1/clusters/list-node-types",
				Response: sdk_compute.ListNodeTypesResponse{
					NodeTypes: []sdk_compute.NodeType{
						{
							NodeTypeId: "m5d.large",
						},
					},
				},
			},
			{
				Method:       "POST",
				ReuseRequest: true,
				Resource:     "/api/2.1/clusters/events",
				Response: clusters.EventsResponse{
					Events: []clusters.ClusterEvent{},
				},
			},
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/2.0/libraries/cluster-status?cluster_id=mount",
				Response: sdk_compute.InstallLibraries{
					Libraries: []sdk_compute.Library{},
				},
			},
		}, func(ctx context.Context, client *common.DatabricksClient) {
			ic := newImportContext(client)
			ic.setClientsForTests()
			ic.enableListing("mounts")
			ic.mounts = true

			err := ic.Importables["databricks_mount"].List(ic)
			assert.NoError(t, err)

			resources := ic.Scope.Sorted()
			for i := range resources {
				err = ic.Importables["databricks_mount"].Body(ic,
					hclwrite.NewEmptyFile().Body(), resources[i])
				assert.NoError(t, err)
			}
		})
}

var meAdminFixture = qa.HTTPFixture{
	Method:       "GET",
	ReuseRequest: true,
	Resource:     "/api/2.0/preview/scim/v2/Me",
	Response: scim.User{
		Groups: []scim.ComplexValue{
			{
				Display: "admins",
			},
		},
	},
}

var getTokensPermissionsFixture = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.0/permissions/authorization/tokens?",
	Response:     getJSONObject("test-data/get-tokens-permissions.json"),
	ReuseRequest: true,
}

var emptyPipelines = qa.HTTPFixture{
	Method:       "GET",
	ReuseRequest: true,
	Resource:     "/api/2.0/pipelines?max_results=100",
	Response:     pipelines.ListPipelinesResponse{},
}

var emptyClusterPolicies = qa.HTTPFixture{
	Method:       "GET",
	ReuseRequest: true,
	Resource:     "/api/2.0/policies/clusters/list?",
	Response:     sdk_compute.ListPoliciesResponse{},
}

var emptyPolicyFamilies = qa.HTTPFixture{
	Method:   "GET",
	Resource: "/api/2.0/policy-families?",
	Response: sdk_compute.ListPolicyFamiliesResponse{
		PolicyFamilies: []sdk_compute.PolicyFamily{},
	},
	ReuseRequest: true,
}

var emptyMlflowWebhooks = qa.HTTPFixture{
	Method:       "GET",
	ReuseRequest: true,
	Resource:     "/api/2.0/mlflow/registry-webhooks/list?",
	Response:     ml.ListRegistryWebhooks{},
}

var emptyExternalLocations = qa.HTTPFixture{
	Method:   "GET",
	Resource: "/api/2.1/unity-catalog/external-locations?",
	Status:   200,
	Response: &sdk_uc.ListExternalLocationsResponse{},
}

var emptyStorageCredentials = qa.HTTPFixture{
	Method:   "GET",
	Resource: "/api/2.1/unity-catalog/storage-credentials?",
	Status:   200,
	Response: &sdk_uc.ListStorageCredentialsResponse{},
}

var emptyUcCredentials = qa.HTTPFixture{
	Method:   "GET",
	Resource: "/api/2.1/unity-catalog/credentials?",
	Status:   200,
	Response: &sdk_uc.ListCredentialsResponse{},
}

var emptyConnections = qa.HTTPFixture{
	Method:   "GET",
	Resource: "/api/2.1/unity-catalog/connections?",
	Response: sdk_uc.ListConnectionsResponse{},
}

var emptyRepos = qa.HTTPFixture{
	Method:       "GET",
	ReuseRequest: true,
	Resource:     "/api/2.0/repos?path_prefix=%2FWorkspace",
	Response:     repos.ReposListResponse{},
}

var emptyVectorSearch = qa.HTTPFixture{
	Method:       "GET",
	ReuseRequest: true,
	Resource:     "/api/2.0/vector-search/endpoints?",
	Response:     sdk_vs.ListEndpointResponse{},
}

var emptyShares = qa.HTTPFixture{
	Method:       "GET",
	ReuseRequest: true,
	Resource:     "/api/2.1/unity-catalog/shares?",
	Response:     sharing.ListSharesResponse{},
}

var emptyRecipients = qa.HTTPFixture{
	Method:       "GET",
	ReuseRequest: true,
	Resource:     "/api/2.1/unity-catalog/recipients?",
	Response:     sharing.ListRecipientsResponse{},
}

var emptyGitCredentials = qa.HTTPFixture{
	Method:   http.MethodGet,
	Resource: "/api/2.0/git-credentials",
	Response: []sdk_workspace.CredentialInfo{
		{},
	},
}

var emptyModelServing = qa.HTTPFixture{
	Method:   "GET",
	Resource: "/api/2.0/serving-endpoints",
	Response: serving.ListEndpointsResponse{
		Endpoints: []serving.ServingEndpoint{},
	},
}

var emptyIpAccessLIst = qa.HTTPFixture{
	Method:   http.MethodGet,
	Resource: "/api/2.0/ip-access-lists",
	Response: map[string]any{},
}

var emptyWorkspace = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.0/workspace/list?path=%2F",
	Response:     tf_workspace.ObjectList{},
	ReuseRequest: true,
}

var emptySqlEndpoints = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.0/sql/warehouses?",
	Response:     map[string]any{},
	ReuseRequest: true,
}

var emptyInstancePools = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.0/instance-pools/list",
	Response:     map[string]any{},
	ReuseRequest: true,
}

var emptySqlDashboards = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.0/preview/sql/dashboards?page_size=100",
	Response:     map[string]any{},
	ReuseRequest: true,
}

var emptyGlobalInitScripts = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.0/global-init-scripts",
	ReuseRequest: true,
	Response: map[string]any{
		"scripts": []map[string]any{},
	},
}

var emptySqlQueries = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.0/sql/queries?page_size=100",
	Response:     map[string]any{},
	ReuseRequest: true,
}

var emptySqlAlerts = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.0/sql/alerts?page_size=100",
	Response:     []tf_sql.AlertEntity{},
	ReuseRequest: true,
}

var emptyWorkspaceConf = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.0/workspace-conf?",
	Response:     map[string]any{},
	ReuseRequest: true,
}

var allKnownWorkspaceConfsNoData = qa.HTTPFixture{
	Method:       "GET",
	Resource:     fmt.Sprintf("/api/2.0/workspace-conf?keys=%s", workspaceConfKeysToURL()),
	Response:     map[string]any{},
	ReuseRequest: true,
}

var emptyGlobalSQLConfig = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.0/sql/config/warehouses",
	Response:     tf_sql.GlobalConfigForRead{},
	ReuseRequest: true,
}

var noCurrentMetastoreAttached = qa.HTTPFixture{
	Method:   "GET",
	Resource: "/api/2.1/unity-catalog/metastore_summary",
	Status:   404,
	Response: &apierr.APIError{
		ErrorCode:  "NOT_FOUND",
		StatusCode: 404,
		Message:    "nope",
	},
	ReuseRequest: true,
}

var currentMetastoreResponse = &sdk_uc.GetMetastoreSummaryResponse{
	MetastoreId: "12345678-1234",
	Name:        "test",
}

var currentMetastoreSuccess = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.1/unity-catalog/metastore_summary",
	Response:     currentMetastoreResponse,
	ReuseRequest: true,
}

var emptyMetastoreList = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.1/unity-catalog/metastores",
	Response:     sdk_uc.ListMetastoresResponse{},
	ReuseRequest: true,
}

var emptyLakeviewList = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.0/lakeview/dashboards?page_size=1000",
	Response:     sdk_dashboards.ListDashboardsResponse{},
	ReuseRequest: true,
}

var emptyDestinationNotficationsList = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.0/notification-destinations?",
	Response:     settings.ListNotificationDestinationsResponse{},
	ReuseRequest: true,
}

var emptyUsersList = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.0/preview/scim/v2/Users?attributes=id%2CuserName&count=10000&startIndex=1",
	Response:     map[string]any{},
	ReuseRequest: true,
}

var emptySpnsList = qa.HTTPFixture{
	Method:       "GET",
	Resource:     "/api/2.0/preview/scim/v2/ServicePrincipals?attributes=id%2CuserName&count=10000&startIndex=1",
	Response:     map[string]any{},
	ReuseRequest: true,
}

func TestImportingUsersGroupsSecretScopes(t *testing.T) {
	listSpFixtures := qa.ListServicePrincipalsFixtures([]iam.ServicePrincipal{
		{
			Id:            "345",
			ApplicationId: "spn",
		},
	})
	listUserFixtures := qa.ListUsersFixtures([]iam.User{
		{
			Id:       "123",
			UserName: "test@test.com",
		},
	})
	listGroupFixtures := qa.ListGroupsFixtures([]iam.Group{
		{Id: "a"},
		{Id: "b"},
		{Id: "c"},
	})
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			emptyDestinationNotficationsList,
			noCurrentMetastoreAttached,
			emptyLakeviewList,
			emptyMetastoreList,
			meAdminFixture,
			emptyRepos,
			emptyShares,
			emptyConnections,
			emptyRecipients,
			emptyGitCredentials,
			emptyWorkspace,
			emptyIpAccessLIst,
			emptyInstancePools,
			emptyModelServing,
			emptyExternalLocations,
			emptyStorageCredentials,
			emptyUcCredentials,
			emptyMlflowWebhooks,
			emptySqlDashboards,
			emptySqlEndpoints,
			emptySqlQueries,
			emptySqlAlerts,
			emptyVectorSearch,
			emptyPipelines,
			emptyClusterPolicies,
			emptyPolicyFamilies,
			emptyWorkspaceConf,
			allKnownWorkspaceConfsNoData,
			emptyGlobalSQLConfig,
			listSpFixtures[0],
			listSpFixtures[1],
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/ServicePrincipals/345?attributes=userName,displayName,active,externalId,entitlements",
				Response: iam.ServicePrincipal{
					Id:            "345",
					ApplicationId: "spn",
				},
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/ServicePrincipals/345?attributes=userName,displayName,active,externalId,entitlements,groups,roles",
				Response: iam.ServicePrincipal{
					Id:            "345",
					ApplicationId: "spn",
				},
				ReuseRequest: true,
			},
			listUserFixtures[0],
			listUserFixtures[1],
			listGroupFixtures[0],
			listGroupFixtures[1],
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Groups/a",
				Response: scim.Group{
					// TODO: add another user for which there is no filter resut
					ID: "a", DisplayName: "admins",
					Members: []scim.ComplexValue{
						{Display: "test@test.com", Value: "123", Ref: "Users/123"},
						{Display: "Test group", Value: "f", Ref: "Groups/f"},
						{Display: "spn", Value: "spn", Ref: "ServicePrincipals/spn"},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Groups/b",
				Response: scim.Group{
					ID: "b", DisplayName: "users",
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Groups/c",
				Response: scim.Group{
					ID: "b", DisplayName: "test",
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/ServicePrincipals/spn?attributes=userName,displayName,active,externalId,entitlements",
				Response: scim.User{ID: "321", DisplayName: "spn", ApplicationID: "spn",
					Groups: []scim.ComplexValue{
						{Display: "admins", Value: "a", Ref: "Groups/a", Type: "direct"},
					}},
				ReuseRequest: true,
			},
			emptyGlobalInitScripts,
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Groups/a?attributes=members",
				Response: scim.Group{ID: "a", DisplayName: "admins",
					Members: []scim.ComplexValue{
						{Display: "test@test.com", Value: "123", Ref: "Users/123"},
						{Display: "Test group", Value: "f", Ref: "Groups/f"},
						{Display: "spn", Value: "spn", Ref: "ServicePrincipals/spn"},
					},
				},
				ReuseRequest: true,
			},
			// Get requests appear to be made with two different sets of parameters. The first set is defined in `util.go`, the second
			// in the groups resource definition itself.
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Groups/a?attributes=id,displayName,active,externalId,entitlements,groups,roles,members,meta",
				Response: scim.Group{ID: "a", DisplayName: "admins",
					Members: []scim.ComplexValue{
						{Display: "test@test.com", Value: "123", Ref: "Users/123"},
						{Display: "Test group", Value: "f", Ref: "Groups/f"},
						{Display: "spn", Value: "spn", Ref: "ServicePrincipals/spn"},
					},
				},
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Groups/b?attributes=id,displayName,active,externalId,entitlements,groups,roles,members,meta",
				Response: scim.Group{ID: "b", DisplayName: "users"},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Groups/c?attributes=id,displayName,active,externalId,entitlements,groups,roles,members,meta",
				Response: scim.Group{ID: "c", DisplayName: "test",
					Groups: []scim.ComplexValue{
						{Display: "admins", Value: "a", Ref: "Groups/a", Type: "direct"},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Groups/a?attributes=displayName,externalId,entitlements",
				Response: scim.Group{ID: "a", DisplayName: "admins",
					Members: []scim.ComplexValue{
						{Display: "test@test.com", Value: "123", Ref: "Users/123"},
						{Display: "Test group", Value: "f", Ref: "Groups/f"},
						{Display: "spn", Value: "spn", Ref: "ServicePrincipals/spn"},
					},
				},
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Groups/b?attributes=displayName,externalId,entitlements",
				Response: scim.Group{ID: "b", DisplayName: "users"},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Groups/c?attributes=displayName,externalId,entitlements",
				Response: scim.Group{ID: "c", DisplayName: "test",
					Groups: []scim.ComplexValue{
						{Display: "admins", Value: "a", Ref: "Groups/a", Type: "direct"},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Groups/f?attributes=displayName,externalId,entitlements",
				Response: scim.Group{ID: "f", DisplayName: "nested"},
			},
			// TODO: add groups to the output
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Users/123?attributes=userName,displayName,active,externalId,entitlements",
				Response: scim.User{ID: "123", DisplayName: "test@test.com", UserName: "test@test.com"},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Users/123?attributes=id,userName,displayName,active,externalId,entitlements,groups,roles",
				Response: scim.User{ID: "123", DisplayName: "test@test.com", UserName: "test@test.com"},
			},
			{
				Method:   "GET",
				Resource: "/api/2.2/jobs/list?limit=100",
				Response: sdk_jobs.ListJobsResponse{},
			},
			{
				Method:   "GET",
				Resource: "/api/2.1/clusters/list?filter_by.cluster_sources=UI&filter_by.cluster_sources=API&page_size=100",
				Response: clusters.ClusterList{},
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/scopes/list",
				ReuseRequest: true,
				Response: sdk_workspace.ListScopesResponse{
					Scopes: []sdk_workspace.SecretScope{
						{Name: "a"},
					},
				},
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/list?scope=a",
				ReuseRequest: true,
				Response: sdk_workspace.ListSecretsResponse{
					Secrets: []sdk_workspace.SecretMetadata{
						{Key: "b"},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/secrets/acls/list?scope=a",
				Response: sdk_workspace.ListAclsResponse{
					Items: []sdk_workspace.AclItem{
						{Permission: "MANAGE", Principal: "test"},
						{Permission: "READ", Principal: "users"},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/secrets/acls/list?scope=a",
				Response: sdk_workspace.ListAclsResponse{
					Items: []sdk_workspace.AclItem{
						{Permission: "MANAGE", Principal: "test"},
						{Permission: "READ", Principal: "users"},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/secrets/acls/get?principal=test&scope=a",
				Response: sdk_workspace.AclItem{Permission: "MANAGE", Principal: "test"},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/secrets/acls/get?principal=users&scope=a",
				Response: sdk_workspace.AclItem{Permission: "READ", Principal: "users"},
			},
			emptyWorkspace,
			{
				Method:   "GET",
				Resource: "/api/2.0/secrets/get?key=b&scope=a",

				Response: sdk_workspace.GetSecretResponse{
					Value: "dGVzdA==",
					Key:   "b",
				},
			},
			getTokensPermissionsFixture,
		}, func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.Directory = tmpDir
			_, listing := ic.allServicesAndListing()
			ic.enableListing(listing)
			ic.exportSecrets = true
			ic.noFormat = true

			err := ic.Run()
			assert.NoError(t, err)
		})
}

func TestImportingNoResourcesError(t *testing.T) {
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/2.0/preview/scim/v2/Me",
				Response: scim.User{
					Groups: []scim.ComplexValue{},
				},
			},
			emptyUsersList,
			emptySpnsList,
			noCurrentMetastoreAttached,
			emptyLakeviewList,
			emptyDestinationNotficationsList,
			emptyMetastoreList,
			emptyRepos,
			emptyExternalLocations,
			emptyStorageCredentials,
			emptyUcCredentials,
			emptyShares,
			emptyConnections,
			emptyRecipients,
			emptyModelServing,
			emptyMlflowWebhooks,
			emptyWorkspaceConf,
			emptyInstancePools,
			emptyClusterPolicies,
			allKnownWorkspaceConfsNoData,
			qa.ListGroupsFixtures([]iam.Group{})[0],
			emptyGitCredentials,
			emptyIpAccessLIst,
			emptyWorkspace,
			emptySqlEndpoints,
			emptyVectorSearch,
			emptySqlQueries,
			emptySqlDashboards,
			emptySqlAlerts,
			emptyPipelines,
			emptyPolicyFamilies,
			{
				Method:       "GET",
				Resource:     "/api/2.0/global-init-scripts",
				ReuseRequest: true,
				Response: map[string]any{
					"scripts": []map[string]any{},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.2/jobs/list?limit=100",
				Response: sdk_jobs.ListJobsResponse{},
			},
			{
				Method:   "GET",
				Resource: "/api/2.1/clusters/list?filter_by.cluster_sources=UI&filter_by.cluster_sources=API&page_size=100",
				Response: clusters.ClusterList{},
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/scopes/list",
				ReuseRequest: true,
				Response: sdk_workspace.ListScopesResponse{
					Scopes: []sdk_workspace.SecretScope{},
				},
			},
			emptyWorkspace,
		}, func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.Directory = tmpDir
			ic.noFormat = true
			_, listing := ic.allServicesAndListing()
			ic.enableListing(listing)

			err := ic.Run()
			assert.EqualError(t, err, "no resources to import or delete")
		})
}

func TestImportingClusters(t *testing.T) {
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			emptyRepos,
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/get-status?path=%2FUsers%2Fuser%40domain.com%2Flibs%2Ftest.whl&return_git_info=true",
				Response: tf_workspace.ObjectStatus{},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/get-status?path=%2FUsers%2Fuser%40domain.com%2Frepo%2Ftest.sh&return_git_info=true",
				Response: tf_workspace.ObjectStatus{},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Groups?",
				Response: scim.GroupList{Resources: []scim.Group{}},
			},
			{
				Method:   "GET",
				Resource: "/api/2.2/jobs/list?limit=100",
				Response: sdk_jobs.ListJobsResponse{},
			},
			{
				Method:       "GET",
				Resource:     "/api/2.1/clusters/list?filter_by.cluster_sources=UI&filter_by.cluster_sources=API&page_size=100",
				Response:     getJSONObject("test-data/clusters-list-response.json"),
				ReuseRequest: true,
			},
			{
				Method:       "GET",
				Resource:     "/api/2.1/clusters/get?cluster_id=test1",
				Response:     getJSONObject("test-data/get-cluster-test1-response.json"),
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.1/clusters/list?filter_by.is_pinned=true&page_size=100",
				Response: sdk_compute.ListClustersResponse{
					Clusters: []sdk_compute.ClusterDetails{},
				},
				ReuseRequest: true,
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/libraries/cluster-status?cluster_id=test1",
				Response:     getJSONObject("test-data/libraries-cluster-status-test1.json"),
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/clusters/test1?",
				Response: getJSONObject("test-data/get-cluster-permissions-test1-response.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/dbfs/get-status?path=dbfs%3A%2FFileStore%2Fjars%2Ftest.jar",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/get-dbfs-library-status.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/dbfs/read?length=1000000&path=dbfs%3A%2FFileStore%2Fjars%2Ftest.jar",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/get-dbfs-library-data.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.1/clusters/get?cluster_id=test2",
				Response: getJSONObject("test-data/get-cluster-test2-response.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/libraries/cluster-status?cluster_id=test2",
				Response: getJSONObject("test-data/libraries-cluster-status-test2.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/clusters/test2?",
				Response: getJSONObject("test-data/get-cluster-permissions-test2-response.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/policies/clusters/get?policy_id=123",
				Response: getJSONObject("test-data/get-cluster-policy.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/cluster-policies/123?",
				Response: getJSONObject("test-data/get-cluster-policy-permissions.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.1/clusters/get?cluster_id=awscluster",
				Response: getJSONObject("test-data/get-cluster-awscluster-response.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/libraries/cluster-status?cluster_id=awscluster",
				Response:     getJSONObject("test-data/libraries-cluster-status-test2.json"),
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/clusters/awscluster?",
				Response: getJSONObject("test-data/get-cluster-permissions-awscluster-response.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/instance-profiles/list",
				Response: getJSONObject("test-data/list-instance-profiles.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/preview/scim/v2/Me",
				ReuseRequest: true,
				Response:     scim.User{ID: "a", DisplayName: "test@test.com"},
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/instance-pools/get?instance_pool_id=pool1",
				Response:     getJSONObject("test-data/get-instance-pool1.json"),
				ReuseRequest: true,
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/permissions/instance-pools/pool1?",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/get-job-permissions-14.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/list?scope=some-kv-scope",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/secret-scopes-list-scope-response.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/acls/list?scope=some-kv-scope",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/secret-scopes-list-scope-acls-response.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/acls/get?principal=test%40test.com&scope=some-kv-scope",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/secret-scopes-get-principal-response.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/scopes/list",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/secret-scopes-response.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/libraries/cluster-status?cluster_id=test2",
				Response: sdk_compute.ClusterLibraryStatuses{
					ClusterId: "test2",
					LibraryStatuses: []sdk_compute.LibraryFullStatus{
						{
							Library: &sdk_compute.Library{
								Pypi: &sdk_compute.PythonPyPiLibrary{
									Package: "chispa",
								},
							},
						},
					},
				},
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/preview/scim/v2/Users?attributes=id%2CuserName&count=10000&startIndex=1",
				ReuseRequest: true,
				Response: scim.UserList{
					Resources: []scim.User{
						{ID: "123", DisplayName: "test@test.com", UserName: "test@test.com"},
					},
				},
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			os.Setenv("EXPORTER_PARALLELISM_default", "1")
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.Directory = tmpDir
			ic.noFormat = true
			ic.enableListing("compute")
			ic.enableServices("access,users,policies,compute,secrets,groups,storage")

			err := ic.Run()
			os.Unsetenv("EXPORTER_PARALLELISM_default")
			assert.NoError(t, err)
			content, err := os.ReadFile(tmpDir + "/compute.tf")
			assert.NoError(t, err)
			contentStr := string(content)
			assert.True(t, strings.Contains(contentStr, `resource "databricks_cluster" "test1_test1"`))
			assert.True(t, strings.Contains(contentStr, `resource "databricks_cluster" "test_cluster_policy_test2"`))
			assert.True(t, strings.Contains(contentStr, `policy_id                    = databricks_cluster_policy.users_cluster_policy.id`))
			assert.True(t, strings.Contains(contentStr, `autotermination_minutes = 0`))
			assert.True(t, strings.Contains(contentStr, `autotermination_minutes = 120`))
			assert.True(t, strings.Contains(contentStr, `library {
    jar = databricks_dbfs_file._0eee4efe7411a5bdca65d7b79188026c_test_jar.dbfs_path
  }`))
			assert.True(t, strings.Contains(contentStr, `init_scripts {
    dbfs {
      destination = databricks_dbfs_file._0eee4efe7411a5bdca65d7b79188026c_test_jar.dbfs_path
    }
  }`))
		})
}

func TestImportingJobs_JobList(t *testing.T) {
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			emptyRepos,
			{
				Method:   "GET",
				Resource: "/api/2.2/jobs/list?limit=100",
				Response: sdk_jobs.ListJobsResponse{
					Jobs: []sdk_jobs.BaseJob{
						{
							JobId: 14,
							Settings: &sdk_jobs.JobSettings{
								Name: "Demo job",
							},
						},
						{
							JobId: 15,
							Settings: &sdk_jobs.JobSettings{
								Name: "Demo job",
							},
						},
						{
							JobId: 16,
							Settings: &sdk_jobs.JobSettings{
								Name: "Demo job",
							},
						},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/jobs/14?",
				Response: getJSONObject("test-data/get-job-permissions-14.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/dbfs/get-status?path=dbfs%3A%2FFileStore%2Fjars%2Ftest.jar",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/get-dbfs-library-status.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/dbfs/read?length=1000000&path=dbfs%3A%2FFileStore%2Fjars%2Ftest.jar",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/get-dbfs-library-data.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/instance-pools/get?instance_pool_id=pool1",
				Response:     getJSONObject("test-data/get-instance-pool1.json"),
				ReuseRequest: true,
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/permissions/instance-pools/pool1?",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/get-job-permissions-14.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.2/jobs/get?job_id=14",
				Response: sdk_jobs.Job{
					JobId: 14,
					Settings: &sdk_jobs.JobSettings{
						RunAs: &sdk_jobs.JobRunAs{
							UserName:             "user@domain.com",
							ServicePrincipalName: "0000-1111-2222-3333-4444-5555",
						},
						EmailNotifications: &sdk_jobs.JobEmailNotifications{
							OnFailure: []string{"user@domain.com"},
						},
						WebhookNotifications: &sdk_jobs.WebhookNotifications{
							OnSuccess: []sdk_jobs.Webhook{
								{Id: "123"},
							},
						},
						Name: "Dummy",
						Tasks: []sdk_jobs.Task{
							{
								TaskKey: "test",
								Libraries: []sdk_compute.Library{
									{Jar: "dbfs:/FileStore/jars/test.jar"},
									{Whl: "/Workspace/Repos/user@domain.com/repo/test.whl"},
									{Whl: "/Workspace/Users/user@domain.com/libs/test.whl"},
								},
								NewCluster: &sdk_compute.ClusterSpec{
									InstancePoolId: "pool1",
									NumWorkers:     2,
									SparkVersion:   "6.4.x-scala2.11",
									PolicyId:       "123",
								},
								SparkJarTask: &sdk_jobs.SparkJarTask{
									JarUri:        "dbfs:/FileStore/jars/test.jar",
									MainClassName: "com.databricks.examples.ProjectDriver",
								},
								SparkPythonTask: &sdk_jobs.SparkPythonTask{
									// this makes no sense for prod, but does for tests ;-)
									PythonFile: "/foo/bar.py",
									Parameters: []string{
										"dbfs:/FileStore/jars/test.jar",
										"etc",
									},
								},
								NotebookTask: &sdk_jobs.NotebookTask{
									NotebookPath: "/Workspace/Test",
								},
								PipelineTask: &sdk_jobs.PipelineTask{
									PipelineId: "123",
								},
							},
						},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/policies/clusters/get?policy_id=123",
				Response: sdk_compute.Policy{
					PolicyId: "123",
					Name:     "dummy",
					Definition: `{
						"aws_attributes.instance_profile_arn": {
							"type": "fixed",
							"value": "arn:aws:iam::12345:instance-profile/shard-s3-access",
							"hidden": true
						},
						"instance_pool_id": {
							"type": "fixed",
							"value": "pool1",
							"hidden": true
						}
					}`,
				},
			},
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/2.0/instance-profiles/list",
				Response: aws.InstanceProfileList{
					InstanceProfiles: []aws.InstanceProfileInfo{
						{
							InstanceProfileArn: "arn:aws:iam::12345:instance-profile/shard-s3-access",
						},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/cluster-policies/123?",
				Response: getJSONObject("test-data/get-cluster-policy-permissions.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/instance-profiles/list",
				Response: getJSONObject("test-data/list-instance-profiles.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/instance-pools/get?instance_pool_id=pool1",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/get-instance-pool1.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/permissions/instance-pools/pool1?",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/get-job-permissions-14.json"),
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			ic := importContextForTestWithClient(ctx, client)
			ic.enableServices("jobs,access,storage,clusters,pools")
			ic.enableListing("jobs")
			ic.mounts = true
			ic.noFormat = true
			ic.meAdmin = true
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)
			ic.Directory = tmpDir

			err := ic.Importables["databricks_job"].List(ic)
			assert.NoError(t, err)

			resources := ic.Scope.Sorted()
			for _, res := range resources {
				if res.Resource != "databricks_job" {
					continue
				}
				// simulate complex HCL write
				err = ic.dataToHcl(
					ic.Importables["databricks_job"],
					[]string{},
					ic.Resources["databricks_job"],
					res,
					hclwrite.NewEmptyFile().Body())

				assert.NoError(t, err)
			}
		})
}

func TestImportingJobs_JobListMultiTask(t *testing.T) {
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			emptyRepos,
			{
				Method:   "GET",
				Resource: "/api/2.2/jobs/list?limit=100",
				Response: sdk_jobs.ListJobsResponse{
					Jobs: []sdk_jobs.BaseJob{
						{
							JobId: 14,
							Settings: &sdk_jobs.JobSettings{
								Name: "Demo job",
							},
						},
					},
				},
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/permissions/jobs/14?",
				Response:     getJSONObject("test-data/get-job-permissions-14.json"),
				ReuseRequest: true,
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/dbfs/get-status?path=dbfs%3A%2FFileStore%2Fjars%2Ftest.jar",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/get-dbfs-library-status.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/dbfs/read?length=1000000&path=dbfs%3A%2FFileStore%2Fjars%2Ftest.jar",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/get-dbfs-library-data.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/instance-pools/get?instance_pool_id=pool1",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/get-instance-pool1.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/permissions/instance-pools/pool1?",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/get-job-permissions-14.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.1/jobs/get?job_id=14",
				Response: sdk_jobs.Job{
					JobId: 14,
					Settings: &sdk_jobs.JobSettings{
						Tasks: []sdk_jobs.Task{
							{
								TaskKey: "dummy",
								Libraries: []sdk_compute.Library{
									{Jar: "dbfs:/FileStore/jars/test.jar"},
								},
								NewCluster: &sdk_compute.ClusterSpec{
									InstancePoolId:       "pool1",
									DriverInstancePoolId: "pool1",
									NumWorkers:           2,
									SparkVersion:         "6.4.x-scala2.11",
									PolicyId:             "123",
								},
								SparkJarTask: &sdk_jobs.SparkJarTask{
									JarUri:        "dbfs:/FileStore/jars/test.jar",
									MainClassName: "com.databricks.examples.ProjectDriver",
								},
								SparkPythonTask: &sdk_jobs.SparkPythonTask{
									// this makes no sense for prod, but does for tests ;-)
									PythonFile: "/foo/bar.py",
									Parameters: []string{
										"dbfs:/FileStore/jars/test.jar",
										"etc",
									},
								},
								NotebookTask: &sdk_jobs.NotebookTask{
									NotebookPath: "/Test",
								},
								PipelineTask: &sdk_jobs.PipelineTask{
									PipelineId: "123",
								},
								SqlTask: &sdk_jobs.SqlTask{
									Dashboard: &sdk_jobs.SqlTaskDashboard{
										DashboardId: "123",
									},
									WarehouseId: "123",
								},
								DbtTask: &sdk_jobs.DbtTask{
									WarehouseId: "123",
									Commands:    []string{"dbt init"},
								},
								RunJobTask: &sdk_jobs.RunJobTask{
									JobId: 14,
								},
								WebhookNotifications: &sdk_jobs.WebhookNotifications{
									OnSuccess: []sdk_jobs.Webhook{
										{Id: "123"},
									},
								},
								EmailNotifications: &sdk_jobs.TaskEmailNotifications{
									OnFailure: []string{"user@domain.com"},
								},
							},
							{
								TaskKey: "dummy2",
								SqlTask: &sdk_jobs.SqlTask{
									Query: &sdk_jobs.SqlTaskQuery{
										QueryId: "123",
									},
								},
							},
							{
								TaskKey: "dummy3",
								SqlTask: &sdk_jobs.SqlTask{
									Alert: &sdk_jobs.SqlTaskAlert{
										AlertId: "123",
									},
								},
							},
						},
						Name:   "Dummy",
						Format: "MULTI_TASK",
						WebhookNotifications: &sdk_jobs.WebhookNotifications{
							OnSuccess: []sdk_jobs.Webhook{
								{Id: "123"},
							},
						},
						EmailNotifications: &sdk_jobs.JobEmailNotifications{
							OnFailure: []string{"user@domain.com"},
						},
						JobClusters: []sdk_jobs.JobCluster{
							{
								JobClusterKey: "shared",
								NewCluster: sdk_compute.ClusterSpec{
									InstancePoolId: "pool1",
									NumWorkers:     2,
									SparkVersion:   "6.4.x-scala2.11",
									PolicyId:       "123",
								},
							},
						},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/policies/clusters/get?policy_id=123",
				Response: sdk_compute.Policy{
					PolicyId: "123",
					Name:     "dummy",
					Definition: `{
						"aws_attributes.instance_profile_arn": {
							"type": "fixed",
							"value": "arn:aws:iam::12345:instance-profile/shard-s3-access",
							"hidden": true
						},
						"instance_pool_id": {
							"type": "fixed",
							"value": "pool1",
							"hidden": true
						}
					}`,
				},
			},
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/2.0/instance-profiles/list",
				Response: aws.InstanceProfileList{
					InstanceProfiles: []aws.InstanceProfileInfo{
						{
							InstanceProfileArn: "arn:aws:iam::12345:instance-profile/shard-s3-access",
						},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/cluster-policies/123?",
				Response: getJSONObject("test-data/get-cluster-policy-permissions.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/instance-profiles/list",
				Response: getJSONObject("test-data/list-instance-profiles.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/instance-pools/get?instance_pool_id=pool1",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/get-instance-pool1.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/permissions/instance-pools/pool1?",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/get-job-permissions-14.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/get-status?path=%2Ffoo%2Fbar.py&return_git_info=true",
				Response: tf_workspace.ObjectStatus{},
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			ic := newImportContext(client)
			ic.enableServices("jobs,access,storage,clusters,pools")
			ic.enableListing("jobs")
			ic.mounts = true
			ic.noFormat = true
			ic.meAdmin = true
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)
			ic.Directory = tmpDir

			err := ic.Run()
			assert.NoError(t, err)

			content, err := os.ReadFile(tmpDir + "/jobs.tf")
			assert.NoError(t, err)
			contentStr := string(content)
			assert.True(t, strings.Contains(contentStr, `resource "databricks_job" "dummy_14"`))
			assert.True(t, strings.Contains(contentStr, `spark_jar_task {
      main_class_name = "com.databricks.examples.ProjectDriver"
      jar_uri         = databricks_dbfs_file._0eee4efe7411a5bdca65d7b79188026c_test_jar.dbfs_path
    }`))
			assert.True(t, strings.Contains(contentStr, `run_job_task {
      job_id = databricks_job.dummy_14.id
    }`))
			assert.True(t, strings.Contains(contentStr, `notebook_task {
      notebook_path = "/Test"
    }`))
			assert.True(t, strings.Contains(contentStr, `library {
      jar = databricks_dbfs_file._0eee4efe7411a5bdca65d7b79188026c_test_jar.dbfs_path
    }`))
			assert.True(t, strings.Contains(contentStr, `job_cluster {
    new_cluster {
      spark_version    = "6.4.x-scala2.11"
      policy_id        = "123"
      num_workers      = 2
      instance_pool_id = databricks_instance_pool.pool_1.id
    }
    job_cluster_key = "shared"
  }`))
		})
}

func TestImportingWithError(t *testing.T) {
	err := Run("-directory", "/bin/sh", "-services", "groups,users", "-skip-interactive")
	assert.EqualError(t, err, "the path /bin/sh is not a directory")

	err = Run("-directory", "/bin/abcd", "-services", "groups,users", "-prefix", "abc", "-skip-interactive")
	assert.EqualError(t, err, "can't create directory /bin/abcd")
}

func TestImportingSecrets(t *testing.T) {
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			emptyRepos,
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Groups?",
				Response: scim.GroupList{Resources: []scim.Group{}},
			},
			{
				Method:   "GET",
				Resource: "/api/2.2/jobs/list?limit=100",
				Response: sdk_jobs.ListJobsResponse{},
			},
			{
				Method:   "GET",
				Resource: "/api/2.1/clusters/list?filter_by.cluster_sources=UI&filter_by.cluster_sources=API&page_size=100",
				Response: clusters.ClusterList{},
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/scopes/list",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/secret-scopes-response.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/list?scope=some-kv-scope",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/secret-scopes-list-scope-response.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/acls/list?scope=some-kv-scope",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/secret-scopes-list-scope-acls-response.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/acls/get?principal=test%40test.com&scope=some-kv-scope",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/secret-scopes-get-principal-response.json"),
			},
		}, func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.Directory = tmpDir
			ic.noFormat = true
			ic.enableListing("secrets")
			services, _ := ic.allServicesAndListing()
			ic.enableServices(services)
			ic.generateDeclaration = true

			err := ic.Run()
			assert.NoError(t, err)
		})
}

func TestResourceName(t *testing.T) {
	ic := newImportContext(&common.DatabricksClient{})
	norm := ic.ResourceName(&resource{
		Name: "9721431b_bcd3_4526_b90f_f5de2befec8c-dbutils_extensions_2_11_0_0_1-18dc8.jar",
	})
	assert.Equal(t, "dbutils_extensions_2_11_0_0_1_18dc8_jar", norm)

	norm = ic.ResourceName(&resource{
		Name: "9721431b_bcd3_4526_b90f_f5de2befec8c|8737798193",
	})
	assert.Equal(t, "r56cde0f5eda", norm)

	assert.NotEqual(t, ic.ResourceName(&resource{
		Name: "0A"}), ic.ResourceName(&resource{
		Name: "0a",
	}))

	norm = ic.ResourceName(&resource{
		Name: "General Policy - All Users",
	})
	assert.Equal(t, "general_policy_all_users", norm)
}

func TestImportingGlobalInitScriptsAndWorkspaceConf(t *testing.T) {
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			emptyWorkspaceConf,
			emptyGlobalSQLConfig,
			{
				Method:   "GET",
				Resource: fmt.Sprintf("/api/2.0/workspace-conf?keys=%s", workspaceConfKeysToURL()),
				Response: map[string]any{
					"enableWebTerminal": "true",
				},
				ReuseRequest: true,
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/global-init-scripts",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/global-init-scripts-list.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/global-init-scripts/C39FD6BAC8088BBC?",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/global-init-script-get1.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/global-init-scripts/F931E63C248C1D8C?",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/global-init-script-get2.json"),
			},
		}, func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.noFormat = true
			ic.Directory = tmpDir
			ic.enableListing("wsconf")
			services, _ := ic.allServicesAndListing()
			ic.enableServices(services)
			ic.generateDeclaration = true

			err := ic.Run()
			assert.NoError(t, err)
			content, err := os.ReadFile(tmpDir + "/wsconf.tf")
			assert.NoError(t, err)
			contentStr := string(content)
			assert.True(t, strings.Contains(contentStr, `resource "databricks_global_init_script" "init_script1" {
  source  = "${path.module}/global_init_scripts/init_script1.sh"
  name    = "init_script1"
  enabled = true
}`))
			assert.True(t, strings.Contains(contentStr, `resource "databricks_workspace_conf" "global_workspace_conf" {
  custom_config = {
    enableWebTerminal = "true"
  }
}`))
		})
}

func TestImportingUser(t *testing.T) {
	userFixture := qa.ListUsersFixtures([]iam.User{
		{
			Id:       "123",
			UserName: "me",
		},
	})
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			userFixture[0],
			userFixture[1],
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/2.0/preview/scim/v2/Users/123?attributes=id,userName,displayName,active,externalId,entitlements,groups,roles",
				Response: scim.User{
					ID:       "123",
					UserName: "me",
					Groups: []scim.ComplexValue{
						{
							Value: "abc",
							Type:  "direct",
						},
					},
				},
			},
		}, func(ctx context.Context, client *common.DatabricksClient) {
			ic := newImportContext(client)
			ic.noFormat = true
			ic.setClientsForTests()
			err := resourcesMap["databricks_user"].Search(ic, &resource{
				Resource: "databricks_user",
				Value:    "me",
			})
			assert.NoError(t, err)

			d := ic.Resources["databricks_user"].TestResourceData()
			d.Set("user_name", "me")
			err = resourcesMap["databricks_user"].Import(ic, &resource{
				Resource: "databricks_user",
				Data:     d,
			})
			assert.NoError(t, err)
		})
}

func TestImportingRepos(t *testing.T) {
	resp := repos.ReposInformation{
		ID:           121232342,
		Url:          "https://github.com/user/test.git",
		Provider:     "gitHub",
		Path:         "/Repos/user@domain/test",
		HeadCommitID: "1124323423abc23424",
		Branch:       "releases",
	}

	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			userListIdUsernameFixture,
			userListIdUsernameFixture2,
			userListFixture,
			userReadFixture,
			{
				Method:   "GET",
				Resource: "/api/2.0/repos?path_prefix=%2FWorkspace",
				Response: repos.ReposListResponse{
					Repos: []repos.ReposInformation{
						resp,
					},
				},
			},
			emptyGitCredentials,
			{
				Method:   "GET",
				Resource: "/api/2.0/repos/121232342",
				Response: resp,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/repos/121232342?",
				Response: getJSONObject("test-data/get-repo-permissions.json"),
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.noFormat = true
			ic.Directory = tmpDir
			ic.enableListing("repos")

			err := ic.Run()
			assert.NoError(t, err)
		})
}

func TestImportingIPAccessLists(t *testing.T) {
	resp := settings.IpAccessListInfo{
		ListId:       "123",
		Label:        "block_list",
		ListType:     "BLOCK",
		IpAddresses:  []string{"1.2.3.4"},
		AddressCount: 2,
		Enabled:      true,
	}
	resp2 := resp
	resp2.IpAddresses = []string{}
	resp2.ListId = "124"
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			emptyRepos,
			emptyWorkspaceConf,
			allKnownWorkspaceConfsNoData,
			getTokensPermissionsFixture,
			{
				Method:   "GET",
				Resource: "/api/2.0/global-init-scripts",
				Response: map[string]any{},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/ip-access-lists",
				Response: settings.GetIpAccessListsResponse{
					IpAccessLists: []settings.IpAccessListInfo{resp, resp2},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/ip-access-lists/123?",
				Response: settings.GetIpAccessListResponse{
					IpAccessList: &resp,
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/ip-access-lists/124?",
				Response: settings.GetIpAccessListResponse{
					IpAccessList: &resp2,
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace-conf?keys=enableIpAccessLists%2CenableTokensConfig%2CmaxTokenLifetimeDays",
				Response: map[string]any{
					"enableIpAccessLists":  "true",
					"maxTokenLifetimeDays": "90",
					"enableTokensConfig":   "true",
				},
				ReuseRequest: true,
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.noFormat = true
			ic.Directory = tmpDir
			services := "workspace,access"
			ic.enableListing(services)

			err := ic.Run()
			assert.NoError(t, err)
		})
}

func TestImportingSqlObjects(t *testing.T) {
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			emptyRepos,
			emptyIpAccessLIst,
			emptyGlobalSQLConfig,
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/list?path=%2F",
				Response: tf_workspace.ObjectList{
					Objects: []tf_workspace.ObjectStatus{
						{
							Path:       "/Shared",
							ObjectID:   4451965692354143,
							ObjectType: tf_workspace.Directory,
						},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/list?path=%2FShared",
				Response: tf_workspace.ObjectList{},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/get-status?path=%2FShared",
				Response: tf_workspace.ObjectStatus{
					Path:       "/Shared",
					ObjectID:   4451965692354143,
					ObjectType: tf_workspace.Directory,
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/directories/4451965692354143?",
				Response: getJSONObject("test-data/get-directory-permissions.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/global-init-scripts",
				Response: map[string]any{},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/sql/warehouses?",
				Response: getJSONObject("test-data/get-sql-endpoints.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/sql/warehouses/f562046bc1272886?",
				Response: getJSONObject("test-data/get-sql-endpoint.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/sql/data_sources",
				Response: []sdk_sql.DataSource{
					{
						Id:          "147164a6-8316-4a9d-beff-f57261801374",
						WarehouseId: "f562046bc1272886",
					},
				},
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/sql/warehouses/f562046bc1272886?",
				Response: getJSONObject("test-data/get-sql-endpoint-permissions.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/preview/sql/dashboards?page_size=100",
				Response:     getJSONObject("test-data/get-sql-dashboards.json"),
				ReuseRequest: true,
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/preview/sql/dashboards/9cb0c8f5-6262-4a1f-a741-2181de76028f",
				Response:     getJSONObject("test-data/get-sql-dashboard.json"),
				ReuseRequest: true,
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/sql/queries?page_size=100",
				Response:     getJSONObject("test-data/get-queries.json"),
				ReuseRequest: true,
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/sql/queries/16c4f969-eea0-4aad-8f82-03d79b078dcc?",
				Response:     getJSONObject("test-data/get-query.json"),
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/sql/queries/16c4f969-eea0-4aad-8f82-03d79b078dcc",
				Response: getJSONObject("test-data/get-sql-query.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/sql/queries/16c4f969-eea0-4aad-8f82-03d79b078dcc?",
				Response: getJSONObject("test-data/get-sql-query-permissions.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/dbsql-dashboards/9cb0c8f5-6262-4a1f-a741-2181de76028f?",
				Response: getJSONObject("test-data/get-sql-dashboard-permissions.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/sql/alerts?page_size=100",
				Response:     getJSONObject("test-data/get-alerts.json"),
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/sql/alerts/3cf91a42-6217-4f3c-a6f0-345d489051b9?",
				Response: getJSONObject("test-data/get-alert.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/sql/alerts/3cf91a42-6217-4f3c-a6f0-345d489051b9?",
				Response: getJSONObject("test-data/get-sql-alert-permissions.json"),
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.noFormat = true
			ic.Directory = tmpDir
			ic.enableListing("sql-dashboards,queries,sql-endpoints,alerts")
			ic.enableServices("sql-dashboards,queries,alerts,sql-endpoints,access")

			err := ic.Run()
			assert.NoError(t, err)

			// check the generated HCL for SQL Warehouses
			content, err := os.ReadFile(tmpDir + "/sql-endpoints.tf")
			assert.NoError(t, err)
			contentStr := string(content)
			assert.True(t, strings.Contains(contentStr, `enable_serverless_compute = false`))
			assert.True(t, strings.Contains(contentStr, `resource "databricks_sql_endpoint" "test" {`))
			assert.False(t, strings.Contains(contentStr, `tags {`))
			// check the generated HCL for SQL Dashboards
			content, err = os.ReadFile(tmpDir + "/sql-dashboards.tf")
			assert.NoError(t, err)
			contentStr = string(content)
			assert.True(t, strings.Contains(contentStr, `resource "databricks_sql_dashboard" "test_9cb0c8f5_6262_4a1f_a741_2181de76028f" {`))
			assert.True(t, strings.Contains(contentStr, `dashboard_id = databricks_sql_dashboard.test_9cb0c8f5_6262_4a1f_a741_2181de76028f.id`))
			assert.True(t, strings.Contains(contentStr, `resource "databricks_sql_widget" "rd4dd2082685" {`))
			assert.True(t, strings.Contains(contentStr, `resource "databricks_sql_visualization" "chart_16c4f969_eea0_4aad_8f82_03d79b078dcc_1a062d3a_eefe_11eb_9559_dc7cd9c86087"`))
			// check the generated HCL for Qieries
			content, err = os.ReadFile(tmpDir + "/queries.tf")
			assert.NoError(t, err)
			contentStr = string(content)
			assert.True(t, strings.Contains(contentStr, `resource "databricks_query" "jobs_per_day_per_status_last_30_days_16c4f969_eea0_4aad_8f82_03d79b078dcc"`))
			assert.True(t, strings.Contains(contentStr, `warehouse_id    = databricks_sql_endpoint.test.id`))
			assert.True(t, strings.Contains(contentStr, `owner_user_name = "user@domain.com"`))
			assert.True(t, strings.Contains(contentStr, `display_name    = "Jobs per day per status last 30 days"`))
			// check the generated HCL for Alerts
			content, err = os.ReadFile(tmpDir + "/alerts.tf")
			assert.NoError(t, err)
			contentStr = string(content)
			assert.True(t, strings.Contains(contentStr, `resource "databricks_alert" "test_alert_3cf91a42_6217_4f3c_a6f0_345d489051b9"`))
			assert.True(t, strings.Contains(contentStr, `query_id        = databricks_query.jobs_per_day_per_status_last_30_days_16c4f969_eea0_4aad_8f82_03d79b078dcc.id`))
			assert.True(t, strings.Contains(contentStr, `display_name    = "Test Alert"`))
			assert.True(t, strings.Contains(contentStr, `op = "GREATER_THAN"`))
			assert.True(t, strings.Contains(contentStr, `owner_user_name = "test@domain.com"`))
		})
}

func TestImportingDLTPipelines(t *testing.T) {
	userFixture := qa.ListUsersFixtures([]iam.User{
		{Id: "123", UserName: "user@domain.com"},
	})
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			emptyRepos,
			emptyWorkspace,
			emptyIpAccessLIst,
			{
				Method:   "GET",
				Resource: "/api/2.0/pipelines?max_results=100",
				Response: pipelines.ListPipelinesResponse{
					Statuses: []pipelines.PipelineStateInfo{
						{
							PipelineId: "123",
							Name:       "Pipeline1",
						},
					},
				},
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/workspace/get-status?path=%2FUsers%2Fuser%40domain.com",
				Response:     map[string]any{},
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/repos/123?",
				Response: getJSONObject("test-data/get-repo-permissions.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/get-status?path=%2FRepos%2Fuser%40domain.com%2Frepo",
				Response: tf_workspace.ObjectStatus{
					ObjectID:   123,
					ObjectType: "REPO",
					Path:       "/Repos/user@domain.com/repo",
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/repos/123",
				Response: repos.ReposInformation{
					ID:           123,
					Url:          "https://github.com/user/test.git",
					Provider:     "gitHub",
					Path:         "/Repos/user@domain.com/repo",
					HeadCommitID: "1124323423abc23424",
					Branch:       "releases",
				},
				ReuseRequest: true,
			},
			userFixture[0],
			userFixture[1],
			{
				Method:       "GET",
				Resource:     "/api/2.0/preview/scim/v2/Users/123?attributes=userName,displayName,active,externalId,entitlements",
				Response:     scim.User{ID: "123", DisplayName: "user@domain.com", UserName: "user@domain.com"},
				ReuseRequest: true,
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/preview/scim/v2/Users/123?attributes=id,userName,displayName,active,externalId,entitlements,groups,roles",
				Response:     scim.User{ID: "123", DisplayName: "user@domain.com", UserName: "user@domain.com"},
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/pipelines/123?",
				Response: getJSONObject("test-data/get-dlt-pipeline.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/pipelines/123?",
				Response: getJSONObject("test-data/get-pipeline-permissions.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/notebooks/123?",
				Response: getJSONObject("test-data/get-notebook-permissions.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/get-status?path=%2FUsers%2Fuser%40domain.com%2FTest+DLT",
				Response: tf_workspace.ObjectStatus{
					Language:   tf_workspace.Python,
					ObjectID:   123,
					ObjectType: tf_workspace.Notebook,
					Path:       "/Users/user@domain.com/Test DLT",
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/export?format=SOURCE&path=%2FUsers%2Fuser%40domain.com%2FTest+DLT",
				Response: tf_workspace.ExportPath{
					Content: "spark.range(10)",
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/preview/scim/v2/Users?attributes=userName%2Cid",
				Response: scim.UserList{
					Resources: []scim.User{
						{
							ID:       "id",
							UserName: "id@domain.com",
						},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/instance-profiles/list",
				Response: getJSONObject("test-data/list-instance-profiles.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/scopes/list",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/secret-scopes-response.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/list?scope=some-kv-scope",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/secret-scopes-list-scope-response.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/acls/list?scope=some-kv-scope",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/secret-scopes-list-scope-acls-response.json"),
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/secrets/acls/get?principal=test%40test.com&scope=some-kv-scope",
				ReuseRequest: true,
				Response:     getJSONObject("test-data/secret-scopes-get-principal-response.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/get-status?path=%2Finit.sh",
				Response: tf_workspace.ObjectStatus{
					ObjectID:   789,
					ObjectType: tf_workspace.File,
					Path:       "/init.sh",
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/export?format=AUTO&path=%2Finit.sh",
				Response: tf_workspace.ExportPath{
					Content: "dGVzdA==",
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/files/789?",
				Response: getJSONObject("test-data/get-workspace-file-permissions.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/get-status?path=%2FUsers%2Fuser%40domain.com%2FTest%20DLT&return_git_info=true",
				Response: tf_workspace.ObjectStatus{},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/get-status?path=%2Finit.sh&return_git_info=true",
				Response: tf_workspace.ObjectStatus{},
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.noFormat = true
			ic.Directory = tmpDir
			ic.enableListing("dlt")
			ic.enableServices("dlt,access,notebooks,users,repos,secrets,wsfiles")

			err := ic.Run()
			assert.NoError(t, err)
			content, err := os.ReadFile(tmpDir + "/dlt.tf")
			assert.NoError(t, err)
			contentStr := string(content)
			assert.True(t, strings.Contains(contentStr, `resource "databricks_pipeline" "test_dlt_123"`))
			assert.True(t, strings.Contains(contentStr, `library {
    notebook {
      path = databricks_notebook.users_user_domain_com_test_dlt_123.id
    }
  }`))
			assert.True(t, strings.Contains(contentStr, `cluster {
    spark_conf = {
      "fs.azure.account.auth.type"            = "OAuth"
      "fs.azure.account.oauth.provider.type"  = "org.apache.hadoop.fs.azurebfs.oauth2.ClientCredsTokenProvider"
      "fs.azure.account.oauth2.client.secret" = "{{secrets/some-kv-scope/test-secret}}"
    }
    num_workers      = 1
    label            = "default"
    instance_pool_id = "123"
    init_scripts {
      dbfs {
        destination = "dbfs:/FileStore/jars/test.jar"
      }
    }
    init_scripts {
      workspace {
        destination = databricks_workspace_file.init_sh_789.id
      }
    }
  }`))
			assert.True(t, strings.Contains(contentStr, `notification {
    email_recipients = [databricks_user.user_123.user_name]
    alerts           = ["on-flow-failure", "on-update-failure"]
  }`))
			assert.True(t, strings.Contains(contentStr, `edition     = "advanced"
  development = true
  continuous  = true`))
		})
}

func TestImportingDLTPipelinesMatchingOnly(t *testing.T) {
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			emptyRepos,
			emptyIpAccessLIst,
			userListIdUsernameFixture,
			userListIdUsernameFixture2,
			userListFixture,
			userReadFixture,
			{
				Method:   "GET",
				Resource: "/api/2.0/pipelines?max_results=100",
				Response: pipelines.ListPipelinesResponse{
					Statuses: []pipelines.PipelineStateInfo{
						{
							PipelineId: "123",
							Name:       "Pipeline1 test",
						},
						{
							PipelineId: "124",
							Name:       "Pipeline1",
						},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/pipelines/123?",
				Response: getJSONObject("test-data/get-dlt-pipeline.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/permissions/pipelines/123?",
				Response: getJSONObject("test-data/get-pipeline-permissions.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/instance-profiles/list",
				Response: getJSONObject("test-data/list-instance-profiles.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/get-status?path=%2FUsers%2Fuser%40domain.com%2FTest%20DLT&return_git_info=true",
				Response: tf_workspace.ObjectStatus{},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/get-status?path=%2Finit.sh&return_git_info=true",
				Response: tf_workspace.ObjectStatus{},
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.noFormat = true
			ic.Directory = tmpDir
			ic.match = "test"
			ic.enableListing("dlt")
			ic.enableServices("dlt,access")

			err := ic.Run()
			assert.NoError(t, err)
		})
}

func TestImportingGlobalSqlConfig(t *testing.T) {
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			allKnownWorkspaceConfsNoData,
			emptyGlobalInitScripts,
			{
				Method:   "GET",
				Resource: "/api/2.0/sql/warehouses?",
				Response: sdk_sql.ListWarehousesResponse{},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/sql/config/warehouses",
				Response: sdk_sql.GetWorkspaceWarehouseConfigResponse{
					EnabledWarehouseTypes: []sdk_sql.WarehouseTypePair{
						{
							WarehouseType: sdk_sql.WarehouseTypePairWarehouseTypeClassic,
							Enabled:       true,
						},
						{
							WarehouseType: sdk_sql.WarehouseTypePairWarehouseTypePro,
							Enabled:       true,
						},
					},
					InstanceProfileArn: "arn:...",
				},
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.noFormat = true
			ic.Directory = tmpDir
			ic.enableListing("wsconf")

			err := ic.Run()
			assert.NoError(t, err)
		})
}

func TestImportingNotebooksWorkspaceFilesWithFilter(t *testing.T) {
	fileStatus := tf_workspace.ObjectStatus{
		ObjectID:   123,
		ObjectType: tf_workspace.File,
		Path:       "/File",
	}
	notebookStatus := tf_workspace.ObjectStatus{
		ObjectID:   456,
		ObjectType: tf_workspace.Notebook,
		Path:       "/Notebook",
		Language:   "PYTHON",
	}
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			emptyRepos,
			emptyIpAccessLIst,
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/list?path=%2F",
				Response: tf_workspace.ObjectList{
					Objects: []tf_workspace.ObjectStatus{notebookStatus, fileStatus,
						{
							ObjectID:   4567,
							ObjectType: tf_workspace.Notebook,
							Path:       "/UnmatchedNotebook",
							Language:   "PYTHON",
						},
						{
							ObjectID:   1234,
							ObjectType: tf_workspace.File,
							Path:       "/UnmatchedFile",
						},
						{
							ObjectID:   456,
							ObjectType: tf_workspace.Directory,
							Path:       "/databricks_automl",
						},
						{
							ObjectID:   456,
							ObjectType: tf_workspace.Directory,
							Path:       "/.bundle",
						},
					},
				},
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/list?path=%2Fdatabricks_automl",
				Response: tf_workspace.ObjectList{},
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/workspace/get-status?path=%2FNotebook",
				Response:     notebookStatus,
				ReuseRequest: true,
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/workspace/get-status?path=%2FFile",
				Response:     fileStatus,
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/export?format=AUTO&path=%2FFile",
				Response: tf_workspace.ExportPath{
					Content: "dGVzdA==",
				},
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/export?format=SOURCE&path=%2FNotebook",
				Response: tf_workspace.ExportPath{
					Content: "dGVzdA==",
				},
				ReuseRequest: true,
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.noFormat = true
			ic.Directory = tmpDir
			ic.enableListing("notebooks,wsfiles")
			ic.excludeRegexStr = "databricks_automl"
			ic.matchRegexStr = "^/[FN].*$"

			err := ic.Run()
			assert.NoError(t, err)
			// check generated code for notebooks
			content, err := os.ReadFile(tmpDir + "/notebooks.tf")
			assert.NoError(t, err)
			contentStr := string(content)
			assert.True(t, strings.Contains(contentStr, `resource "databricks_notebook" "notebook_456"`))
			assert.True(t, strings.Contains(contentStr, `path   = "/Notebook"`))
			assert.False(t, strings.Contains(contentStr, `/UnmatchedNotebook`))
			// check generated code for workspace files
			content, err = os.ReadFile(tmpDir + "/wsfiles.tf")
			assert.NoError(t, err)
			contentStr = string(content)
			assert.True(t, strings.Contains(contentStr, `resource "databricks_workspace_file" "file_123"`))
			assert.True(t, strings.Contains(contentStr, `path   = "/File"`))
			assert.False(t, strings.Contains(contentStr, `/UnmatchedFile`))
		})
}

func TestImportingNotebooksWorkspaceFilesWithFilterDuringWalking(t *testing.T) {
	fileStatus := tf_workspace.ObjectStatus{
		ObjectID:   123,
		ObjectType: tf_workspace.File,
		Path:       "/File",
	}
	notebookStatus := tf_workspace.ObjectStatus{
		ObjectID:   456,
		ObjectType: tf_workspace.Notebook,
		Path:       "/Notebook",
		Language:   "PYTHON",
	}
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			emptyRepos,
			emptyIpAccessLIst,
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/list?path=%2F",
				Response: tf_workspace.ObjectList{
					Objects: []tf_workspace.ObjectStatus{notebookStatus, fileStatus,
						{
							ObjectID:   4567,
							ObjectType: tf_workspace.Notebook,
							Path:       "/UnmatchedNotebook",
							Language:   "PYTHON",
						},
						{
							ObjectID:   1234,
							ObjectType: tf_workspace.File,
							Path:       "/UnmatchedFile",
						},
						{
							ObjectID:   456,
							ObjectType: tf_workspace.Directory,
							Path:       "/databricks_automl",
						},
					},
				},
				ReuseRequest: true,
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/workspace/get-status?path=%2FNotebook",
				Response:     notebookStatus,
				ReuseRequest: true,
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/workspace/get-status?path=%2FFile",
				Response:     fileStatus,
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/export?format=AUTO&path=%2FFile",
				Response: tf_workspace.ExportPath{
					Content: "dGVzdA==",
				},
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/export?format=SOURCE&path=%2FNotebook",
				Response: tf_workspace.ExportPath{
					Content: "dGVzdA==",
				},
				ReuseRequest: true,
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.noFormat = true
			ic.Directory = tmpDir
			ic.enableListing("notebooks,wsfiles")
			ic.excludeRegexStr = "databricks_automl"
			ic.matchRegexStr = "^/[FN].*$"
			ic.filterDirectoriesDuringWorkspaceWalking = true

			err := ic.Run()
			assert.NoError(t, err)
			// check generated code for notebooks
			content, err := os.ReadFile(tmpDir + "/notebooks.tf")
			assert.NoError(t, err)
			contentStr := string(content)
			assert.True(t, strings.Contains(contentStr, `resource "databricks_notebook" "notebook_456"`))
			assert.True(t, strings.Contains(contentStr, `path   = "/Notebook"`))
			assert.False(t, strings.Contains(contentStr, `/UnmatchedNotebook`))
			// check generated code for workspace files
			content, err = os.ReadFile(tmpDir + "/wsfiles.tf")
			assert.NoError(t, err)
			contentStr = string(content)
			assert.True(t, strings.Contains(contentStr, `resource "databricks_workspace_file" "file_123"`))
			assert.True(t, strings.Contains(contentStr, `path   = "/File"`))
			assert.False(t, strings.Contains(contentStr, `/UnmatchedFile`))
		})
}

func TestImportingModelServing(t *testing.T) {
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			emptyRepos,
			emptyIpAccessLIst,
			emptyWorkspace,
			{
				Method:   "GET",
				Resource: "/api/2.0/serving-endpoints",
				Response: serving.ListEndpointsResponse{
					Endpoints: []serving.ServingEndpoint{
						{
							Name: "abc",
						},
					},
				},
			},
			{
				Method:       "GET",
				Resource:     "/api/2.0/serving-endpoints/abc?",
				ReuseRequest: true,
				Response: serving.ServingEndpointDetailed{
					Name: "abc",
					Id:   "1234",
					Config: &serving.EndpointCoreConfigOutput{
						AutoCaptureConfig: &serving.AutoCaptureConfigOutput{
							Enabled:         true,
							CatalogName:     "main",
							SchemaName:      "tmp",
							TableNamePrefix: "test",
						},
						ServedEntities: []serving.ServedEntityOutput{
							{
								EntityName:         "main.tmp.model",
								EntityVersion:      "1",
								Name:               "def",
								ScaleToZeroEnabled: true,
							},
							{
								EntityName:         "def",
								EntityVersion:      "1",
								Name:               "def",
								ScaleToZeroEnabled: false,
								InstanceProfileArn: "arn:aws:iam::123456789012:instance-profile/MyInstanceProfile",
							},
							{
								ExternalModel: &serving.ExternalModel{
									Provider: "databricks",
									Task:     "llm/v1/embeddings",
									Name:     "e5_small_v2",
									DatabricksModelServingConfig: &serving.DatabricksModelServingConfig{
										DatabricksApiToken:     "dapi",
										DatabricksWorkspaceUrl: "https://adb-1234.azuredatabricks.net",
									},
								},
							},
						},
					},
				},
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.noFormat = true
			ic.Directory = tmpDir
			ic.enableListing("model-serving")
			ic.enableServices("model-serving")

			err := ic.Run()
			assert.NoError(t, err)

			content, err := os.ReadFile(tmpDir + "/model-serving.tf")
			assert.NoError(t, err)
			contentStr := string(content)
			assert.True(t, strings.Contains(contentStr, `resource "databricks_model_serving" "abc_90015098"`))
			assert.True(t, strings.Contains(contentStr, `scale_to_zero_enabled = false`))
			assert.True(t, strings.Contains(contentStr, `instance_profile_arn  = "arn:aws:iam::123456789012:instance-profile/MyInstanceProfile"`))
			assert.True(t, strings.Contains(contentStr, `databricks_api_token     = "dapi"`))
			assert.True(t, strings.Contains(contentStr, `databricks_workspace_url = "https://adb-1234.azuredatabricks.net"`))
			assert.True(t, strings.Contains(contentStr, `served_entities {
      scale_to_zero_enabled = true
      name                  = "def"
      entity_version        = "1"
      entity_name           = "main.tmp.model"
    }`))
		})
}

func TestImportingMlfloweWebhooks(t *testing.T) {
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			emptyRepos,
			emptyIpAccessLIst,
			emptyWorkspace,
			{
				Method:   "GET",
				Resource: "/api/2.0/mlflow/registry-webhooks/list",
				Response: ml.ListRegistryWebhooks{
					Webhooks: []ml.RegistryWebhook{
						{
							Id: "abc",
							JobSpec: &ml.JobSpecWithoutSecret{
								JobId: "123",
							},
						},
					},
				},
			},
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/2.0/mlflow/registry-webhooks/list?",
				Response: ml.ListRegistryWebhooks{
					Webhooks: []ml.RegistryWebhook{
						{
							Id: "abc",
							JobSpec: &ml.JobSpecWithoutSecret{
								JobId: "123",
							},
						},
					},
				},
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.noFormat = true
			ic.Directory = tmpDir
			ic.enableListing("mlflow-webhooks")

			err := ic.Run()
			assert.NoError(t, err)
		})
}

func TestIncrementalErrors(t *testing.T) {
	// Testing missing `-updated-since`
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{},
		func(ctx context.Context, client *common.DatabricksClient) {
			ic := newImportContext(client)
			ic.noFormat = true
			ic.enableServices("model-serving")
			ic.incremental = true

			err := ic.Run()
			assert.ErrorContains(t, err, "-updated-since is required with -interactive parameter")
		})
	// Testing broken `-updated-since`
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{},
		func(ctx context.Context, client *common.DatabricksClient) {
			ic := newImportContext(client)
			ic.noFormat = true
			ic.enableServices("model-serving")
			ic.incremental = true
			ic.updatedSinceStr = "aaa"

			err := ic.Run()
			assert.ErrorContains(t, err, "can't parse value 'aaa' please specify it")
		})
}

func TestIncrementalDLTAndMLflowWebhooks(t *testing.T) {
	webhooks := []ml.RegistryWebhook{
		{
			LastUpdatedTimestamp: 1681466931226,
			Id:                   "abc",
			HttpUrlSpec: &ml.HttpUrlSpecWithoutSecret{
				Url: "https://....",
			},
		},
		{
			LastUpdatedTimestamp: 1690156900000,
			Id:                   "def",
			JobSpec: &ml.JobSpecWithoutSecret{
				JobId: "123",
			},
		},
	}
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			meAdminFixture,
			noCurrentMetastoreAttached,
			emptyRepos,
			emptyIpAccessLIst,
			emptyWorkspace,
			{
				Method:   "GET",
				Resource: "/api/2.0/mlflow/registry-webhooks/list?",
				Response: ml.ListRegistryWebhooks{
					Webhooks: webhooks,
				},
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/mlflow/registry-webhooks/list",
				Response: ml.ListRegistryWebhooks{
					Webhooks: webhooks,
				},
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/pipelines?max_results=100",
				Response: pipelines.ListPipelinesResponse{
					Statuses: []pipelines.PipelineStateInfo{
						{
							PipelineId: "abc",
							Name:       "abc",
						},
						{
							PipelineId: "def",
							Name:       "def",
						},
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/pipelines/abc?",
				Response: pipelines.GetPipelineResponse{
					PipelineId:   "abc",
					Name:         "abc",
					LastModified: 1681466931226,
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/pipelines/def?",
				Response: pipelines.GetPipelineResponse{
					PipelineId:   "def",
					Name:         "def",
					LastModified: 1690156900000,
					Spec: &pipelines.PipelineSpec{
						Target:  "default",
						Catalog: "main",
					},
				},
				ReuseRequest: true,
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)
			os.Mkdir(tmpDir, 0700)
			os.WriteFile(tmpDir+"/import.sh", []byte(
				`terraform import databricks_pipeline.abc "abc"
terraform import databricks_pipeline.def "def"
`), 0700)

			os.WriteFile(tmpDir+"/import.tf", []byte(
				`import {
  id = "abc"
  to = databricks_pipeline.abc
}
import {
  id = "def"
  to = databricks_pipeline.def
}
`), 0700)

			os.WriteFile(tmpDir+"/dlt.tf", []byte(`resource "databricks_pipeline" "abc" {
}

resource "databricks_pipeline" "def" {
}
`), 0700)
			os.WriteFile(tmpDir+"/vars.tf", []byte(`variable "var1" {
	description = ""
}
`), 0700)

			ic := newImportContext(client)
			ic.noFormat = true
			ic.Directory = tmpDir
			services := "dlt,mlflow-webhooks"
			ic.enableListing(services)
			ic.incremental = true
			ic.updatedSinceStr = "2023-07-24T00:00:00Z"
			ic.meAdmin = false
			ic.nativeImportSupported = true

			err := ic.Run()
			assert.NoError(t, err)

			content, err := os.ReadFile(tmpDir + "/import.sh")
			assert.NoError(t, err)
			contentStr := string(content)
			assert.True(t, strings.Contains(contentStr, `import databricks_pipeline.abc "abc"`))
			assert.True(t, strings.Contains(contentStr, `import databricks_pipeline.def "def"`))

			content, err = os.ReadFile(tmpDir + "/import.tf")
			assert.NoError(t, err)
			contentStr = string(content)
			log.Printf("[DEBUG] contentStr: %s", contentStr)
			assert.True(t, strings.Contains(contentStr, `id = "abc"`))
			assert.True(t, strings.Contains(contentStr, `to = databricks_pipeline.def`))

			content, err = os.ReadFile(tmpDir + "/dlt.tf")
			assert.NoError(t, err)
			contentStr = string(content)
			assert.True(t, strings.Contains(contentStr, `resource "databricks_pipeline" "def"`))
			assert.True(t, strings.Contains(contentStr, `resource "databricks_pipeline" "abc"`))

			content, err = os.ReadFile(tmpDir + "/vars.tf")
			assert.NoError(t, err)
			contentStr = string(content)
			assert.True(t, strings.Contains(contentStr, `variable "var1"`))
			assert.True(t, strings.Contains(contentStr, `variable "job_spec_webhook_def"`))
		})
}

func TestImportingRunJobTask(t *testing.T) {
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/2.0/preview/scim/v2/Me",
				Response: scim.User{
					Groups: []scim.ComplexValue{
						{
							Display: "admins",
						},
					},
					UserName: "user@domain.com",
				},
			},
			noCurrentMetastoreAttached,
			emptyRepos,
			emptyIpAccessLIst,
			emptyWorkspace,
			{
				Method:   "GET",
				Resource: "/api/2.2/jobs/list?limit=100",
				Response: map[string]any{
					"jobs": []any{
						getJSONObject("test-data/run-job-main.json"),
					},
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.1/jobs/get?job_id=1047501313827425",
				Response: getJSONObject("test-data/run-job-main.json"),
			},
			{
				Method:   "GET",
				Resource: "/api/2.1/jobs/get?job_id=932035899730845",
				Response: getJSONObject("test-data/run-job-child.json"),
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.noFormat = true
			ic.Directory = tmpDir
			ic.enableListing("jobs")
			ic.match = "runjobtask"

			err := ic.Run()
			assert.NoError(t, err)

			content, err := os.ReadFile(tmpDir + "/jobs.tf")
			assert.NoError(t, err)
			contentStr := string(content)
			assert.True(t, strings.Contains(contentStr, `job_id = databricks_job.jartask_932035899730845.id`))
			assert.True(t, strings.Contains(contentStr, `resource "databricks_job" "runjobtask_1047501313827425"`))
			assert.True(t, strings.Contains(contentStr, `resource "databricks_job" "jartask_932035899730845"`))
			assert.True(t, strings.Contains(contentStr, `run_as {
    service_principal_name = "c1b2a35b-87c4-481a-a0fb-0508be621957"
  }`))
			assert.False(t, strings.Contains(contentStr, `run_as {
     user_name = "user@domain.com"
  }`))
		})
}

func TestImportingLakeviewDashboards(t *testing.T) {
	qa.HTTPFixturesApply(t,
		[]qa.HTTPFixture{
			{
				Method:       "GET",
				ReuseRequest: true,
				Resource:     "/api/2.0/preview/scim/v2/Me",
				Response: scim.User{
					Groups: []scim.ComplexValue{
						{
							Display: "admins",
						},
					},
					UserName: "user@domain.com",
				},
			},
			noCurrentMetastoreAttached,
			{
				Method:   "GET",
				Resource: "/api/2.0/lakeview/dashboards?page_size=1000",
				Response: sdk_dashboards.ListDashboardsResponse{
					Dashboards: []sdk_dashboards.Dashboard{
						{
							DashboardId: "9cb0c8f562624a1f",
							DisplayName: "Dashboard1",
						},
					},
				},
				ReuseRequest: true,
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/lakeview/dashboards/9cb0c8f562624a1f?",
				Response: sdk_dashboards.Dashboard{
					DashboardId:         "9cb0c8f562624a1f",
					DisplayName:         "Dashboard1",
					ParentPath:          "/",
					Path:                "/Dashboard1.lvdash.json",
					SerializedDashboard: `{}`,
					WarehouseId:         "1234",
				},
			},
			{
				Method:   "GET",
				Resource: "/api/2.0/workspace/get-status?path=%2FDashboard1.lvdash.json&return_git_info=true",
				Response: workspace_tf.ObjectInfo{},
			},
		},
		func(ctx context.Context, client *common.DatabricksClient) {
			tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
			defer os.RemoveAll(tmpDir)

			ic := newImportContext(client)
			ic.noFormat = true
			ic.Directory = tmpDir
			ic.enableListing("dashboards")
			ic.enableServices("dashboards")

			err := ic.Run()
			assert.NoError(t, err)

			content, err := os.ReadFile(tmpDir + "/dashboards.tf")
			assert.NoError(t, err)
			contentStr := string(content)
			assert.True(t, strings.Contains(contentStr, `resource "databricks_dashboard" "dashboard1_9cb0c8f562624a1f"`))
			assert.True(t, strings.Contains(contentStr, `file_path         = "${path.module}/dashboards/Dashboard1_9cb0c8f562624a1f.lvdash.json"`))
			content, err = os.ReadFile(tmpDir + "/dashboards/Dashboard1_9cb0c8f562624a1f.lvdash.json")
			assert.NoError(t, err)
			contentStr = string(content)
			assert.Equal(t, `{}`, contentStr)
		})
}

func TestNotificationDestinationExport(t *testing.T) {
	qa.HTTPFixturesApply(t, []qa.HTTPFixture{
		meAdminFixture,
		noCurrentMetastoreAttached,
		{
			Method:   "GET",
			Resource: "/api/2.0/notification-destinations?",
			Response: settings.ListNotificationDestinationsResponse{
				Results: []settings.ListNotificationDestinationsResult{
					{
						DisplayName:     "email",
						Id:              "123",
						DestinationType: "EMAIL",
					},
					{
						DisplayName:     "slack",
						Id:              "234",
						DestinationType: "SLACK",
					},
					{
						DisplayName:     "teams",
						Id:              "345",
						DestinationType: "MICROSOFT_TEAMS",
					},
					{
						DisplayName:     "pagerdruty",
						Id:              "456",
						DestinationType: "PAGERDUTY",
					},
					{
						DisplayName:     "webhook",
						Id:              "8481e00d-3e55-4c6c-8462-33b60d1cdc94",
						DestinationType: "WEBHOOK",
					},
				},
			},
		},
		{
			Method:   "GET",
			Resource: "/api/2.0/notification-destinations/123?",
			Response: settings.NotificationDestination{
				DisplayName:     "email",
				Id:              "123",
				DestinationType: "EMAIL",
				Config: &settings.Config{
					Email: &settings.EmailConfig{
						Addresses: []string{"user@domain.com"},
					},
				},
			},
		},
		{
			Method:   "GET",
			Resource: "/api/2.0/notification-destinations/234?",
			Response: settings.NotificationDestination{
				DisplayName:     "slack",
				Id:              "234",
				DestinationType: "SLACK",
				Config: &settings.Config{
					Slack: &settings.SlackConfig{
						UrlSet: true,
					},
				},
			},
		},
		{
			Method:   "GET",
			Resource: "/api/2.0/notification-destinations/345?",
			Response: settings.NotificationDestination{
				DisplayName:     "teams",
				Id:              "345",
				DestinationType: "MICROSOFT_TEAMS",
				Config: &settings.Config{
					MicrosoftTeams: &settings.MicrosoftTeamsConfig{
						UrlSet: true,
					},
				},
			},
		},
		{
			Method:   "GET",
			Resource: "/api/2.0/notification-destinations/456?",
			Response: settings.NotificationDestination{
				DisplayName:     "pagerdruty",
				Id:              "456",
				DestinationType: "PAGERDUTY",
				Config: &settings.Config{
					Pagerduty: &settings.PagerdutyConfig{
						IntegrationKeySet: true,
					},
				},
			},
		},
		{
			Method:   "GET",
			Resource: "/api/2.0/notification-destinations/8481e00d-3e55-4c6c-8462-33b60d1cdc94?",
			Response: settings.NotificationDestination{
				DisplayName:     "webhook",
				Id:              "567",
				DestinationType: "WEBHOOK",
				Config: &settings.Config{
					GenericWebhook: &settings.GenericWebhookConfig{
						UrlSet:      true,
						PasswordSet: true,
					},
				},
			},
		},
	}, func(ctx context.Context, client *common.DatabricksClient) {
		tmpDir := fmt.Sprintf("/tmp/tf-%s", qa.RandomName())
		defer os.RemoveAll(tmpDir)

		ic := newImportContext(client)
		ic.noFormat = true
		ic.Directory = tmpDir
		ic.enableListing("settings")
		ic.enableServices("settings")

		err := ic.Run()
		assert.NoError(t, err)

		content, err := os.ReadFile(tmpDir + "/settings.tf")
		assert.NoError(t, err)
		contentStr := string(content)
		assert.True(t, strings.Contains(contentStr, `resource "databricks_notification_destination" "pagerdruty_456"`))
		assert.True(t, strings.Contains(contentStr, `resource "databricks_notification_destination" "teams_345"`))
		assert.True(t, strings.Contains(contentStr, `resource "databricks_notification_destination" "email_123" {
  display_name = "email"
  config {
    email {
      addresses = ["user@domain.com"]
    }
  }
}`))
		assert.True(t, strings.Contains(contentStr, `resource "databricks_notification_destination" "webhook_8481e00d" {
  display_name = "webhook"
  config {
    generic_webhook {
      url      = var.config_webhook_8481e00d
      password = var.config_webhook_8481e00d_1
    }
  }
}`))
		assert.True(t, strings.Contains(contentStr, `resource "databricks_notification_destination" "slack_234" {
  display_name = "slack"
  config {
    slack {
      url = var.config_slack_234
    }
  }
}`))
	})
}
