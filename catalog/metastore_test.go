package catalog_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/databricks/terraform-provider-databricks/internal/acceptance"
	"github.com/stretchr/testify/require"
)

func getStorageRoot(t *testing.T) string {
	if acceptance.IsAws(t) {
		return "s3://{env.TEST_BUCKET}/test{var.RANDOM}"
	} else if acceptance.IsAzure(t) {
		return "abfss://{var.RANDOM}@{var.RANDOM}/"
	} else if acceptance.IsGcp(t) {
		return "gs://{var.RANDOM}/metastore"
	}
	return ""
}

func getRegion(t *testing.T) string {
	if acceptance.IsAws(t) {
		return "us-east-1"
	} else if acceptance.IsAzure(t) {
		return "eastus"
	} else if acceptance.IsGcp(t) {
		return "us-east1"
	}
	return ""
}

func TestUcAccRootlessMetastore(t *testing.T) {
	acceptance.LoadUcacctEnv(t)
	runMetastoreTest(t, map[string]any{
		"region": getRegion(t),
	})
}

func TestUcAccMetastore(t *testing.T) {
	acceptance.LoadUcacctEnv(t)
	runMetastoreTest(t, map[string]any{
		"storage_root": getStorageRoot(t),
		"region":       getRegion(t),
	})
}

func TestUcAccMetastoreDeltaSharing(t *testing.T) {
	acceptance.LoadUcacctEnv(t)
	runMetastoreTest(t, map[string]any{
		"storage_root":        getStorageRoot(t),
		"region":              getRegion(t),
		"delta_sharing_scope": "INTERNAL",
		"delta_sharing_recipient_token_lifetime_in_seconds": 3600,
		"delta_sharing_organization_name":                   "databricks-tf-provider-test",
	})
}

func TestUcAccMetastoreDeltaSharingInfiniteLifetime(t *testing.T) {
	acceptance.LoadUcacctEnv(t)
	runMetastoreTest(t, map[string]any{
		"storage_root":        getStorageRoot(t),
		"region":              getRegion(t),
		"delta_sharing_scope": "INTERNAL",
		"delta_sharing_recipient_token_lifetime_in_seconds": 0,
	})
}

func TestUcAccMetastoreWithOwnerUpdates(t *testing.T) {
	acceptance.LoadUcacctEnv(t)
	runMetastoreTestWithOwnerUpdates(t, map[string]any{
		"storage_root": getStorageRoot(t),
		"region":       getRegion(t),
	})
}

func getTemplateFromExtraAttributes(t *testing.T, extraAttributes map[string]any) string {
	params := make([]string, len(extraAttributes))
	for k, v := range extraAttributes {
		jsonValue, err := json.Marshal(v)
		require.NoError(t, err)
		params = append(params, k+" = "+string(jsonValue))
	}
	return strings.Join(params, "\n\t\t\t")
}

func runMetastoreTest(t *testing.T, extraAttributes map[string]any) {
	template := getTemplateFromExtraAttributes(t, extraAttributes)
	acceptance.UnityAccountLevel(t, acceptance.Step{
		Template: fmt.Sprintf(`resource "databricks_metastore" "this" {
			name = "{var.RANDOM}"
			force_destroy = true
			%s
		}`, template),
	})
}

func runMetastoreTestWithOwnerUpdates(t *testing.T, extraAttributes map[string]any) {
	template := getTemplateFromExtraAttributes(t, extraAttributes)
	acceptance.UnityAccountLevel(t, acceptance.Step{
		Template: fmt.Sprintf(`resource "databricks_metastore" "this" {
			name = "{var.STICKY_RANDOM}"
			force_destroy = true
			owner = "account users"
			%s
		}`, template),
	}, acceptance.Step{
		Template: fmt.Sprintf(`resource "databricks_metastore" "this" {
			name = "{var.STICKY_RANDOM}"
			force_destroy = true
			owner = "{env.TEST_DATA_ENG_GROUP}"
			%s
		}`, template),
	}, acceptance.Step{
		Template: fmt.Sprintf(`resource "databricks_metastore" "this" {
			name = "{var.STICKY_RANDOM}-updated"
			force_destroy = true
			owner = "{env.TEST_METASTORE_ADMIN_GROUP_NAME}"
			%s
		}`, template),
	})
}
