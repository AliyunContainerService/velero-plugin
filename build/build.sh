#!/usr/bin/env bash
set -e
cd  ${GOPATH}/src/github.com/AliyunContainerService/velero-plugin
GIT_SHA=`git rev-parse --short HEAD || echo "HEAD"`
export GOARCH="amd64"
export GOOS="linux"

branch="master"
version="v1.2"
commitId=$GIT_SHA
buildTime=`date "+%Y-%m-%d-%H:%M:%S"`
CGO_ENABLED=0 go build -ldflags "-X main._BRANCH_='$branch' -X main._VERSION_='$version-$commitId' -X main._BUILDTIME_='$buildTime'" -o _output/velero-plugin-for-alibabacloud ${GOPATH}/src/github.com/AliyunContainerService/velero-plugin/velero-plugin-for-alibabacloud

docker build -t=registry.cn-hangzhou.aliyuncs.com/acs/velero-plugin-alibabacloud:$version-$GIT_SHA .
docker push registry.cn-hangzhou.aliyuncs.com/acs/velero-plugin-alibabacloud:$version-$GIT_SHA
