# Velero Plugin for Alibaba Cloud

<div align="right">

[![English](https://img.shields.io/badge/English-0066CC?style=for-the-badge&logo=github&logoColor=white)](README_EN.md) [![中文](https://img.shields.io/badge/中文-DC143C?style=for-the-badge&logo=github&logoColor=white)](README.md)

</div>

[![GoReportCard Widget]][GoReportCardResult]

Velero Plugin for Alibaba Cloud is a plugin for using Velero to backup and restore Kubernetes resources on Alibaba Cloud.

**Current Version**: v2.0.0 (for Velero v1.17.x)

## Overview

Velero is a utility to back up and restore your Kubernetes resource and persistent volumes.

To do backup/restore on Alibaba Cloud through Velero utility, you need to install and configure velero and velero-plugin-for-alibabacloud.

## Run velero on Alibaba Cloud

To set up Velero on Alibaba Cloud, you:

* Create your OSS bucket
* Create a RAM user for Velero
* Install Velero and velero-plugin-for-alibabacloud

## Create OSS bucket

Velero requires an object storage bucket to store backups in, preferably unique to a single Kubernetes cluster.

Please refer to the [Create a bucket documentation](https://www.alibabacloud.com/help/en/oss/user-guide/create-a-bucket-4) to create an OSS bucket.

## Create RAM user

1. Create the RAM user:

    Follow the [Create a RAM user documentation](https://www.alibabacloud.com/help/en/ram/user-guide/create-a-ram-user).

2. Create a custom policy:

    Follow the [Create a custom policy documentation](https://www.alibabacloud.com/help/en/ram/create-a-custom-policy) to create a policy with the following content:

    ```json
    {
        "Version": "1",
        "Statement": [
            {
                "Action": [
                    "ecs:DescribeSnapshots",
                    "ecs:CreateSnapshot",
                    "ecs:DeleteSnapshot",
                    "ecs:DescribeDisks",
                    "ecs:CreateDisk",
                    "ecs:Addtags",
                    "oss:PutObject",
                    "oss:GetObject",
                    "oss:DeleteObject",
                    "oss:GetBucket",
                    "oss:ListObjects",
                    "oss:ListBuckets"
                ],
                "Resource": [
                    "*"
                ],
                "Effect": "Allow"
            }
        ]
    }
    ```

3. Grant permissions to the RAM user:

    Follow the [Grant permissions to the RAM user documentation](https://www.alibabacloud.com/help/en/ram/user-guide/grant-permissions-to-the-ram-user) to grant the above policy to the RAM user.

4. Create an access key for the user:

    Follow the [Create an AccessKey pair documentation](https://www.alibabacloud.com/help/en/ram/user-guide/create-an-accesskey-pair) to create an AccessKey for the RAM user.

5. Create a Velero-specific credentials file (`credentials-velero`) in your `install` directory:

    ```
    ALIBABA_CLOUD_ACCESS_KEY_ID=<ALIBABA_CLOUD_ACCESS_KEY_ID>
    ALIBABA_CLOUD_ACCESS_KEY_SECRET=<ALIBABA_CLOUD_ACCESS_KEY_SECRET>
    ```

    where the access key id and secret are the values from step 4.

## Install Velero and velero-plugin-for-alibabacloud

### Download Velero

Download the [latest official release's](https://github.com/vmware-tanzu/velero/releases) tarball for your client platform.

### Install Velero

Run the following command to install Velero and velero-plugin-for-alibabacloud into the cluster. This will create a namespace called `velero`, and place a deployment named `velero` in it.

```bash
velero install \
    --provider alibabacloud \
    --plugins velero/velero-plugin-for-alibabacloud:v2.0.0 \
    --bucket <YOUR_BUCKET> \
    --secret-file ./credentials-velero \
    --backup-location-config region=<REGION>,network=<NETWORK> \
    --snapshot-location-config region=<REGION> \
    --wait
```

### Configuration Parameters

#### Backup Storage Location Configuration Parameters

| Parameter | Type | Description | Example |
|:-----|:-----|:-----|:-----|
| `region` | Required | The region where the OSS bucket is located | `cn-hangzhou` |
| `network` | Optional | Network type. Options: `internal` (internal network), `accelerate` (accelerate domain). Default is public network | `internal` |
| `endpoint` | Optional | Custom OSS endpoint | `https://oss-custom.example.com` |

#### Volume Snapshot Location Configuration Parameters

| Parameter | Type | Description | Example |
|:-----|:-----|:-----|:-----|
| `region` | Required | The region where ECS snapshots are located | `cn-hangzhou` |

#### Other common Optional Parameters

| Parameter | Type | Description | Example |
|:-----|:-----|:-----|:-----|
| `--prefix` | Optional | Used to store backups from multiple clusters in the same bucket, specifies the path prefix in the OSS bucket | `--prefix cluster1/backups` |
| `--use-node-agent` | Optional | Enable node agent support for file system level backups | `--use-node-agent` |

(Optional) Customize the Velero installation further to meet your needs.

## Uninstall Velero

To uninstall Velero, please refer to the [Velero official uninstall documentation](https://velero.io/docs/uninstall/).

[GoReportCard Widget]: https://goreportcard.com/badge/github.com/AliyunContainerService/velero-plugin
[GoReportCardResult]: https://goreportcard.com/report/github.com/AliyunContainerService/velero-plugin
