# Velero Plugin for Alibaba Cloud

<div align="right">

[![English](https://img.shields.io/badge/English-0066CC?style=for-the-badge&logo=github&logoColor=white)](README_EN.md) [![中文](https://img.shields.io/badge/中文-DC143C?style=for-the-badge&logo=github&logoColor=white)](README.md)

</div>

[![GoReportCard Widget]][GoReportCardResult]

Velero Plugin for Alibaba Cloud 是用于在阿里云上使用 Velero 进行 Kubernetes 资源备份和恢复的插件。

**当前版本**: v2.0.0（适用于 Velero v1.17.x）

## 概述

Velero 是一个用于备份和恢复 Kubernetes 资源和持久卷的工具。

要在阿里云上通过 Velero 进行备份/恢复，您需要安装和配置 Velero 以及 velero-plugin-for-alibabacloud。

## 在阿里云上运行 Velero

要在阿里云上设置 Velero，您需要：

* 创建 OSS bucket
* 创建 RAM 用户
* 安装 Velero 和 velero-plugin-for-alibabacloud

## 创建 OSS bucket

Velero 需要一个对象存储 bucket 来存储备份，建议为每个 Kubernetes 集群创建独立的 bucket。

请参考 [创建存储空间文档](https://help.aliyun.com/zh/oss/user-guide/create-a-bucket-4) 创建 OSS bucket。

## 创建 RAM 用户

1. 创建 RAM 用户：

   请参考 [创建 RAM 用户文档](https://help.aliyun.com/zh/ram/user-guide/create-a-ram-user)。

2. 创建自定义策略：

   请参考 [创建自定义策略文档](https://help.aliyun.com/zh/ram/create-a-custom-policy) 创建策略，策略内容如下：

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

3. 为 RAM 用户授权：

   请参考 [为 RAM 用户授权文档](https://help.aliyun.com/zh/ram/user-guide/grant-permissions-to-the-ram-user) 将上述策略授权给 RAM 用户。

4. 创建 AccessKey：

   请参考 [创建 AccessKey 文档](https://help.aliyun.com/zh/ram/user-guide/create-an-accesskey-pair) 为 RAM 用户创建 AccessKey。

5. 创建 Velero 凭证文件：

   在您的 `install` 目录下创建 Velero 凭证文件（`credentials-velero`）：

    ```
    ALIBABA_CLOUD_ACCESS_KEY_ID=<ALIBABA_CLOUD_ACCESS_KEY_ID>
    ALIBABA_CLOUD_ACCESS_KEY_SECRET=<ALIBABA_CLOUD_ACCESS_KEY_SECRET>
    ```

    其中 AccessKey ID 和 Secret 来自步骤 4。

## 安装 Velero 和 velero-plugin-for-alibabacloud

### 下载 Velero

下载 [Velero 官方发布版本](https://github.com/vmware-tanzu/velero/releases) 中适合您操作系统的最新版本。

### 安装 Velero

运行以下命令在 Kubernetes 集群中安装 Velero 和 velero-plugin-for-alibabacloud。此命令将安装 Velero 的服务端组件，包括 CRDs、Deployment、ServiceAccount 等资源。

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

### 配置参数说明

#### Backup Storage Location 配置参数

| 参数 | 类型 | 说明 | 示例 |
|------|------|------|------|
| `region` | 必需 | OSS bucket 所在区域 | `cn-hangzhou` |
| `network` | 可选 | 网络类型。可选值：`internal`（内网）、`accelerate`（加速域名）。默认为公网 | `internal` |
| `endpoint` | 可选 | 自定义 OSS 端点 | `https://oss-custom.example.com` |

#### Volume Snapshot Location 配置参数

| 参数 | 类型 | 说明 | 示例 |
|------|------|------|------|
| `region` | 必需 | ECS 快照所在区域 | `cn-hangzhou` |

#### 其他常见可选参数

| 参数 | 类型 | 说明 | 示例 |
|------|------|------|------|
| `--prefix` | 可选 | 用于在同一 bucket 中存储多个集群的备份，指定 OSS bucket 中的路径前缀 | `--prefix cluster1/backups` |
| `--use-node-agent` | 可选 | 启用 node agent 支持，用于文件系统级别的备份 | `--use-node-agent` |

（可选）根据您的需求进一步自定义 Velero 安装。更多参数请参考 [Velero 官方文档](https://velero.io/docs/)。

## 卸载 Velero

要卸载 Velero，请参考 [Velero 官方卸载文档](https://velero.io/docs/uninstall/)。

[GoReportCard Widget]: https://goreportcard.com/badge/github.com/AliyunContainerService/velero-plugin
[GoReportCardResult]: https://goreportcard.com/report/github.com/AliyunContainerService/velero-plugin
