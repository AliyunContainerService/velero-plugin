# Velero Plugin for Alibaba Cloud - Build Guide

> [中文版](BUILD.md) | English Version

This document provides detailed instructions on how to build Velero Plugin for Alibaba Cloud from source.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Build Methods](#build-methods)
  - [Local Build](#local-build)
  - [Container Image Build](#container-image-build)
- [Architecture Support](#architecture-support)
- [Environment Variables](#environment-variables)
- [Common Issues](#common-issues)

## Prerequisites

### Required Tools

1. **Go 1.24+** - For compiling Go code
   ```bash
   go version
   ```

2. **Docker or Podman** - For building container images
   - Docker: Requires buildx support (for multi-architecture builds)
   - Podman: Version 4.0+ (single architecture only)

### Docker Buildx Setup (Docker only)

If using Docker for multi-architecture builds, you need to set up buildx first:

```bash
# Create and enable buildx builder
docker buildx create --name multiarch --use

# Verify buildx status
docker buildx inspect --bootstrap
```

## Build Methods

### Local Build

Local build compiles the binary directly on your machine without creating a container image.

#### Basic Usage

```bash
# Build binary for current platform
make local

# Build binary for specific architecture
ARCH=linux-amd64 make local
ARCH=linux-arm64 make local
```

#### Build Output

After building, the binary will be located at:
```
_output/bin/<GOOS>/<GOARCH>/velero-plugin-alibabacloud
```

Examples:
- Linux AMD64: `_output/bin/linux/amd64/velero-plugin-alibabacloud`
- Linux ARM64: `_output/bin/linux/arm64/velero-plugin-alibabacloud`
- Darwin ARM64: `_output/bin/darwin/arm64/velero-plugin-alibabacloud`

### Container Image Build

Container image build generates Docker/Podman images that can be deployed to Kubernetes.

#### Build with Docker (Recommended, supports multi-architecture)

```bash
# Build image for default architecture (linux-amd64)
make container

# Build image for specific architecture
ARCH=linux-arm64 make container

# Build multi-architecture image (requires Docker buildx)
ARCH=linux-amd64,linux-arm64 make container

# Specify version and image registry
VERSION=v1.0.0 REGISTRY=myregistry.com make container

# Also tag as latest
TAG_LATEST=true VERSION=v1.0.0 make container
```

#### Build with Podman (Single architecture)

```bash
# Build with Podman (single architecture only)
CONTAINER_RUNTIME=podman make container

# Specify architecture
CONTAINER_RUNTIME=podman ARCH=linux-arm64 make container
```

#### Build Output

After building, the image tag format is:
```
<REGISTRY>/<BIN>:<VERSION>
```

Examples:
- `velero/velero-plugin-alibabacloud:main`
- `myregistry.com/velero-plugin-alibabacloud:v1.0.0`

## Architecture Support

### Supported Architectures

| Architecture | GOOS | GOARCH | Description |
|--------------|------|--------|-------------|
| Linux AMD64 | linux | amd64 | Default architecture, suitable for most servers |
| Linux ARM64 | linux | arm64 | ARM servers (e.g., AWS Graviton) |
| Linux ARM | linux | arm | ARM 32-bit devices |
| Darwin AMD64 | darwin | amd64 | Intel Mac |
| Darwin ARM64 | darwin | arm64 | Apple Silicon Mac |
| Windows AMD64 | windows | amd64 | Windows servers |
| Linux PPC64LE | linux | ppc64le | PowerPC servers |

### Architecture Compatibility Notes

1. **Docker Buildx**: Supports all architectures and can build multi-architecture images
2. **Podman**: Only supports single architecture builds. The build architecture depends on:
   - The specified `ARCH` parameter
   - If not specified, uses the default `linux-amd64`

### Cross-Platform Build Examples

```bash
# Build Linux AMD64 image on macOS (Apple Silicon)
ARCH=linux-amd64 make container

# Build Linux ARM64 image on Linux AMD64
ARCH=linux-arm64 make container

# Build multi-architecture image (Docker buildx only)
ARCH=linux-amd64,linux-arm64 make container
```

## Environment Variables

### Makefile Variables

| Variable | Default Value | Description |
|----------|---------------|-------------|
| `BIN` | `velero-plugin-alibabacloud` | Binary file name |
| `ARCH` | `linux-amd64` | Target architecture (format: GOOS-GOARCH) |
| `VERSION` | `main` | Image version tag |
| `REGISTRY` | `velero` | Image registry prefix |
| `TAG_LATEST` | `false` | Whether to also tag as latest |
| `CONTAINER_RUNTIME` | `docker` | Container runtime (docker or podman) |
| `GOPROXY` | `https://proxy.golang.org` | Go module proxy |
| `VELERO_DOCKERFILE` | `Dockerfile` | Dockerfile path |

### Usage Examples

```bash
# Customize all parameters
BIN=my-plugin \
ARCH=linux-arm64 \
VERSION=v2.0.0 \
REGISTRY=myregistry.com \
TAG_LATEST=true \
make container

# Use Chinese Go proxy for faster downloads
GOPROXY=https://goproxy.cn,direct make container
```

## Other Make Targets

### Run Tests

```bash
# Run all unit tests
make test

# Run tests and generate coverage report
make test
# View coverage report
go tool cover -html=coverage.out
```

### Code Quality Checks

```bash
# Tidy Go module dependencies
make modules

# Verify Go module files are up to date
make verify-modules

# CI build (includes module verification and tests)
make ci
```

### Clean Build Artifacts

```bash
# Clean all build artifacts
make clean
```

## Common Issues

### 1. Docker Buildx Not Enabled

**Error Message**:
```
buildx not enabled, refusing to run this recipe
```

**Solution**:
```bash
# Create and enable buildx builder
docker buildx create --name multiarch --use
docker buildx inspect --bootstrap
```

### 2. Podman Build Fails

**Issue**: Podman may not support some Docker features

**Solution**:
- Ensure you're using Podman 4.0+
- If issues persist, try using Docker instead
- Podman only supports single architecture builds, ensure the `ARCH` parameter is correct

### 3. Cross-Platform Build Fails

**Issue**: Building Linux images on non-Linux platforms fails

**Solution**:
- Use Docker buildx for cross-platform builds (recommended)
- Or explicitly specify target architecture with `ARCH=linux-amd64 make container`
- Ensure Dockerfile correctly uses `TARGETPLATFORM` and `BUILDPLATFORM`

### 4. Slow Go Module Downloads

**Issue**: Downloading Go modules is slow in mainland China

**Solution**:
```bash
# Use Chinese Go proxy
GOPROXY=https://goproxy.cn,direct make container

# Or set in Dockerfile
# Edit Dockerfile and modify GOPROXY default value
```

### 5. Build Path Error

**Issue**: Build fails with "package or file not found" error

**Solution**:
- Ensure you're running make command from the project root directory
- Check that `BIN` variable matches the directory name (default: `velero-plugin-alibabacloud`)
- Verify `PKG` variable is correct (default: `github.com/AliyunContainerService/velero-plugin`)

### 6. Permission Issues

**Issue**: Build fails with permission denied errors

**Solution**:
```bash
# Docker: Ensure user is in docker group
sudo usermod -aG docker $USER
# Log out and log back in for changes to take effect

# Podman: Usually doesn't require special permissions
```

## Build Process Overview

### Local Build Process

1. `make local` → Calls `hack/build.sh`
2. `hack/build.sh` → Compiles binary using `go build`
3. Outputs to `_output/bin/<GOOS>/<GOARCH>/<BIN>`

### Container Image Build Process

1. `make container` → Calls Docker/Podman build
2. Dockerfile multi-stage build:
   - **Builder stage**: Uses `golang:1.24-bookworm` to compile binary
   - **Runtime stage**: Uses `alpine:3.22` as runtime image
3. Final image contains the compiled binary

### Multi-Architecture Build Principles

- Docker Buildx uses QEMU emulator for cross-platform builds
- Dockerfile uses `--platform=$TARGETPLATFORM` to specify target platform
- Build arguments are passed to Dockerfile via `ARG`

## Best Practices

1. **Development Environment**: Use `make local` for quick builds and testing
2. **Production Environment**: Use `make container` to build images
3. **CI/CD**: Use `make ci` for complete build and test cycle
4. **Multi-Architecture**: Use Docker buildx to build multi-architecture images to support different platforms
5. **Version Management**: Use `VERSION` variable to explicitly specify version numbers

## Related Documentation

- [Velero Official Documentation](https://velero.io/docs/)
- [Docker Buildx Documentation](https://docs.docker.com/buildx/)
- [Podman Documentation](https://podman.io/docs/)

## Contributing

If you encounter build issues or have improvement suggestions, please submit an Issue or Pull Request.

