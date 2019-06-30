## Overview

[![GoReportCard Widget]][GoReportCardResult]

Velero is a utility to back up and restore your Kubernetes resource and persistent volumes.

To do backup/restore on Alibaba Cloud through Velero utility, you need to install and configure velero and velero-plugin for alibabacloud.

## Run velero on AlibabaCloud

To set up Velero on AlibabaCloud, you:

* Download an official release of Velero
* Create your OSS bucket
* Create an RAM user for Velero
* Install the velero and velero-plugin for alibabacloud

## Download Velero

1. Download the [latest official release's](https://github.com/heptio/velero/releases) tarball for your client platform.

    _We strongly recommend that you use an [official release](https://github.com/heptio/velero/releases) of
Velero. The tarballs for each release contain the `velero` command-line client. The code in the master branch
of the Velero repository is under active development and is not guaranteed to be stable!_

1. Extract the tarball:

    ```bash
    tar -xvf <RELEASE-TARBALL-NAME>.tar.gz -C /dir/to/extract/to 
    ```
    
    We'll refer to the directory you extracted to as the "Velero directory" in subsequent steps.

2. Move the `velero` binary from the Velero directory to somewhere in your PATH.

## Create OSS bucket

Velero requires an object storage bucket to store backups in, preferrably unique to a single Kubernetes cluster. Create an OSS bucket, replacing placeholders appropriately:

```bash
BUCKET=<YOUR_BUCKET>
REGION=<YOUR_REGION>
ossutil mb oss://$BUCKET \
        --storage-class Standard \
        --acl=private
```

## Create RAM user

For more information, see [the AlibabaCloud documentation on RAM users guides][14].

1. Create the RAM user:

    Follow [the AlibabaCloud documentation on RAM users][22].
    
    > If you'll be using Velero to backup multiple clusters with multiple OSS buckets, it may be desirable to create a unique username per cluster rather than the default `velero`.

2. Attach policies to give `velero` the necessary permissions:

    ```bash
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
                    "oss:GetBucket"
                ],
                "Resource": [
                    "*"
                ],
                "Effect": "Allow"
            }
        ]
    }
    ```
3. Create an access key for the user:

    Follow [the AlibabaCloud documentation on create AK][24].

4. Create a Velero-specific credentials file (`credentials-velero`) in your `install` directory:

    ```
    ALIBABA_CLOUD_ACCESS_KEY_ID=<ALIBABA_CLOUD_ACCESS_KEY_ID>
    ALIBABA_CLOUD_ACCESS_KEY_SECRET=<ALIBABA_CLOUD_ACCESS_KEY_SECRET>
    ALIBABA_CLOUD_OSS_ENDPOINT=<ALIBABA_CLOUD_OSS_ENDPOINT>
    ```

    where the access key id and secret are the values get from the step 3 and the oss endpoint is the value oss-$REGION.aliyuncs.com.  
     
## Install velero and velero-plugin for alibabacloud

1. Set some environment variables

	```bash
	BUCKET=<YOUR_BUCKET>
	REGION=<YOUR_REGION>
	```

2. Create a namespace velero and a secret cloud-credentials

	Run the following command to create a namespace named `velero`

	`kubectl create namespace velero`

	Run the following command to create a secret named `cloud-credentials`

	`kubectl create secret generic cloud-credentials --namespace velero --from-file cloud=install/credentials-velero`
	
3. Create and run velero and velero-plugin for alibabacloud

	Run the following command to create and run velero and velero-plugin for alibabacloud
	
	```
	sed -i "s#<BUCKET>#$BUCKET#" install/velero.yaml
	sed -i "s#<REGION>#$REGION#" install/velero.yaml
	kubectl apply -f install/velero.yaml
	```
	
4. Cleanup velero installation

	Run the following command to cleanup the velero installation
	
	```
	kubectl delete namespace/velero clusterrolebinding/velero
	kubectl delete crds -l component=velero
	```
	
## Installing the nginx example (optional)

1. nginx example without persistent volumes

	Run the following command to create a nginx example without persistent volumes:
	
	`kubectl apply -f examples/base.yaml`
	
	Create a backup:
	
	`velero backup create nginx-backup --include-namespaces nginx-example`
	
	Destroy the nginx example:
	
	`kubectl delete namespaces nginx-example`
	
	Create a restore from nginx-backup:
	
	`velero  restore create --from-backup nginx-backup`

2. nginx example with persistent volumes

	Run the following command to create a nginx example with persistent volumes:

	```
	VOLUME_ID=<YOUR_ECS_DISK_ID>
	sed -i "s#{PV_NAME}#$VOLUME_ID#" examples/with-pv.yaml`
	kubectl apply -f examples/with-pv.yaml
	```
	
	Create a backup:
	
	`velero backup create nginx-backup-volume --snapshot-volumes --include-namespaces nginx-example`
	
	Destroy the nginx example:
	
	```
	kubectl delete namespaces nginx-example
	kubectl delete pv $(kubectl get pv|grep nginx-example/nginx-logs|cut -d ' ' -f1)
	```
	
	Create a restore from nginx-backup-volume:
	
	`velero  restore create --from-backup nginx-backup-volume --restore-volumes`
	

[14]: https://www.alibabacloud.com/help/doc-detail/28645.htm
[22]: https://www.alibabacloud.com/help/doc-detail/93720.htm
[23]: https://www.alibabacloud.com/help/doc-detail/50452.htm
[24]: https://www.alibabacloud.com/help/doc-detail/53045.htm

[GoReportCard Widget]: https://goreportcard.com/badge/github.com/AliyunContainerService/velero-plugin
[GoReportCardResult]: https://goreportcard.com/report/github.com/AliyunContainerService/velero-plugin
