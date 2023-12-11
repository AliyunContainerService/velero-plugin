# Copyright the Velero contributors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM --platform=$BUILDPLATFORM golang:1.21 AS build

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG GOPROXY

ENV GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    GOARM=${TARGETVARIANT} \
    GOPROXY=${GOPROXY}

COPY . /go/src/velero-plugin-for-alibabacloud
WORKDIR /go/src/velero-plugin-for-alibabacloud
RUN CGO_ENABLED=0 GOOS=linux go build -v -o /go/bin/velero-plugin-for-alibabacloud ./velero-plugin-for-alibabacloud

FROM busybox
COPY --from=build /go/bin/velero-plugin-for-alibabacloud /plugin/
USER 65532:65532
ENTRYPOINT ["/plugin/velero-plugin-for-alibabacloud", "/target/velero-plugin-for-alibabacloud"]