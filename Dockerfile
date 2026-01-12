# Copyright 2017, 2019 the Velero contributors.
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


FROM --platform=$BUILDPLATFORM golang:1.24.5 as builder
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG GOPROXY=https://proxy.golang.org
ARG PKG=github.com/AliyunContainerService/velero-plugin
ARG BIN=velero-plugin-alibabacloud
ARG VERSION=main
ARG GIT_SHA
ARG GIT_TREE_STATE
ARG REGISTRY=velero

WORKDIR /workspace

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies (using BuildKit cache mount if available)
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code
COPY . .

# Build the binary (using BuildKit cache mount for Go build cache)
RUN --mount=type=cache,target=/root/.cache/go-build \
    TARGETARCH=$(echo $TARGETPLATFORM | cut -f2 -d '/') && \
    CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    GOPROXY=${GOPROXY} \
    go build \
    -installsuffix "static" \
    -o /workspace/${BIN} \
    ./${BIN}

FROM --platform=$TARGETPLATFORM alpine:3.22

ARG BIN=velero-plugin-alibabacloud

RUN mkdir -p /plugins
COPY --from=builder /workspace/${BIN} /plugins/${BIN}

RUN addgroup -S nonroot && adduser -u 65530 -S nonroot -G nonroot
USER 65530

ENTRYPOINT ["/bin/sh", "-c", "cp /plugins/* /target/."]
