---
subcategory: "Deployment"
---
# databricks_aws_unity_catalog_policy Data Source

This data source constructs the necessary AWS Unity Catalog policy for you.

-> This data source can be used with an account or workspace-level provider.

-> This data source has an evolving API, which may change in future versions of the provider. Please always consult [latest documentation](https://docs.databricks.com/data-governance/unity-catalog/get-started.html#configure-a-storage-bucket-and-iam-role-in-aws) in case of any questions.

## Example Usage

```hcl
data "databricks_aws_unity_catalog_policy" "this" {
  aws_account_id = var.aws_account_id
  bucket_name    = "databricks-bucket"
  role_name      = "${var.prefix}-uc-access"
  kms_name       = "arn:aws:kms:us-west-2:111122223333:key/databricks-kms"
}

data "databricks_aws_unity_catalog_assume_role_policy" "this" {
  aws_account_id = var.aws_account_id
  role_name      = "${var.prefix}-uc-access"
  external_id    = "12345"
}

resource "aws_iam_policy" "unity_metastore" {
  name   = "${var.prefix}-unity-catalog-metastore-access-iam-policy"
  policy = data.databricks_aws_unity_catalog_policy.this.json
}

resource "aws_iam_role" "metastore_data_access" {
  name               = "${var.prefix}-uc-access"
  assume_role_policy = data.databricks_aws_unity_catalog_assume_role_policy.this.json
}

resource "aws_iam_role_policy_attachment" "metastore_data_access" {
  role       = aws_iam_role.metastore_data_access.name
  policy_arn = aws_iam_policy.unity_metastore.arn
}
```

## Argument Reference

* `aws_account_id` (Required) The Account ID of the current AWS account (not your Databricks account).
* `aws_partition` - (Optional) AWS partition. The options are `aws`, `aws-us-gov`, or `aws-us-gov-dod`. Defaults to `aws`
* `bucket_name` (Required) The name of the S3 bucket used as root storage location for [managed tables](https://docs.databricks.com/data-governance/unity-catalog/index.html#managed-table) in Unity Catalog.  The name must follow the [S3 bucket naming rules](https://docs.aws.amazon.com/AmazonS3/latest/userguide/bucketnamingrules.html).
* `role_name` (Required) The name of the AWS IAM role that you created in the previous step in the [official documentation](https://docs.databricks.com/data-governance/unity-catalog/get-started.html#configure-a-storage-bucket-and-iam-role-in-aws).
* `kms_name` (Optional) If encryption is enabled, provide the ARN of the KMS key that encrypts the S3 bucket contents. If encryption is disabled, do not provide this argument.

## Attribute Reference

In addition to all arguments above, the following attributes are exported:

* `json` - AWS IAM Policy JSON document
