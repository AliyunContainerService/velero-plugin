# Velero Plugin for Alibaba Cloud - 构建指南

> [English Version](BUILD_EN.md) | 中文版

本文档详细说明如何从源码构建 Velero Plugin for Alibaba Cloud。

## 目录

- [前置要求](#前置要求)
- [构建方式](#构建方式)
  - [本地构建](#本地构建)
  - [容器镜像构建](#容器镜像构建)
- [架构支持](#架构支持)
- [环境变量配置](#环境变量配置)
- [常见问题](#常见问题)

## 前置要求

### 必需工具

1. **Go 1.24+** - 用于编译 Go 代码
   ```bash
   go version
   ```

2. **Docker 或 Podman** - 用于构建容器镜像
   - Docker: 需要启用 buildx 支持（用于多架构构建）
   - Podman: 4.0+ 版本（仅支持单架构构建）

### Docker Buildx 设置（仅 Docker 需要）

如果使用 Docker 进行多架构构建，需要先设置 buildx：

```bash
# 创建并启用 buildx builder
docker buildx create --name multiarch --use

# 验证 buildx 状态
docker buildx inspect --bootstrap
```

## 构建方式

### 本地构建

本地构建会在当前机器上直接编译二进制文件，不生成容器镜像。

#### 基本用法

```bash
# 构建当前平台的二进制文件
make local

# 构建指定架构的二进制文件
ARCH=linux-amd64 make local
ARCH=linux-arm64 make local
```

#### 构建输出

构建完成后，二进制文件位于：
```
_output/bin/<GOOS>/<GOARCH>/velero-plugin-alibabacloud
```

例如：
- Linux AMD64: `_output/bin/linux/amd64/velero-plugin-alibabacloud`
- Linux ARM64: `_output/bin/linux/arm64/velero-plugin-alibabacloud`
- Darwin ARM64: `_output/bin/darwin/arm64/velero-plugin-alibabacloud`

### 容器镜像构建

容器镜像构建会生成可用于 Kubernetes 部署的 Docker/Podman 镜像。

#### 使用 Docker 构建（推荐，支持多架构）

```bash
# 构建默认架构（linux-amd64）的镜像
make container

# 构建指定架构的镜像
ARCH=linux-arm64 make container

# 构建多架构镜像（需要 Docker buildx）
ARCH=linux-amd64,linux-arm64 make container

# 指定版本和镜像仓库
VERSION=v1.0.0 REGISTRY=myregistry.com make container

# 同时打 latest 标签
TAG_LATEST=true VERSION=v1.0.0 make container
```

#### 使用 Podman 构建（单架构）

```bash
# 使用 Podman 构建（仅支持单架构）
CONTAINER_RUNTIME=podman make container

# 指定架构
CONTAINER_RUNTIME=podman ARCH=linux-arm64 make container
```

#### 构建输出

构建完成后，镜像标签格式为：
```
<REGISTRY>/<BIN>:<VERSION>
```

例如：
- `velero/velero-plugin-alibabacloud:main`
- `myregistry.com/velero-plugin-alibabacloud:v1.0.0`

## 架构支持

### 支持的架构

| 架构 | GOOS | GOARCH | 说明 |
|------|------|--------|------|
| Linux AMD64 | linux | amd64 | 默认架构，适用于大多数服务器 |
| Linux ARM64 | linux | arm64 | ARM 服务器（如 AWS Graviton） |
| Linux ARM | linux | arm | ARM 32位设备 |
| Darwin AMD64 | darwin | amd64 | Intel Mac |
| Darwin ARM64 | darwin | arm64 | Apple Silicon Mac |
| Windows AMD64 | windows | amd64 | Windows 服务器 |
| Linux PPC64LE | linux | ppc64le | PowerPC 服务器 |

### 架构兼容性说明

1. **Docker Buildx**: 支持所有架构，可以构建多架构镜像
2. **Podman**: 仅支持单架构构建，构建的架构取决于：
   - 指定的 `ARCH` 参数
   - 如果未指定，使用默认的 `linux-amd64`

### 跨平台构建示例

```bash
# 在 macOS (Apple Silicon) 上构建 Linux AMD64 镜像
ARCH=linux-amd64 make container

# 在 Linux AMD64 上构建 Linux ARM64 镜像
ARCH=linux-arm64 make container

# 构建多架构镜像（仅 Docker buildx）
ARCH=linux-amd64,linux-arm64 make container
```

## 环境变量配置

### Makefile 变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `BIN` | `velero-plugin-alibabacloud` | 二进制文件名 |
| `ARCH` | `linux-amd64` | 目标架构（格式：GOOS-GOARCH） |
| `VERSION` | `main` | 镜像版本标签 |
| `REGISTRY` | `velero` | 镜像仓库前缀 |
| `TAG_LATEST` | `false` | 是否同时打 latest 标签 |
| `CONTAINER_RUNTIME` | `docker` | 容器运行时（docker 或 podman） |
| `GOPROXY` | `https://proxy.golang.org` | Go 模块代理 |
| `VELERO_DOCKERFILE` | `Dockerfile` | Dockerfile 路径 |

### 使用示例

```bash
# 自定义所有参数
BIN=my-plugin \
ARCH=linux-arm64 \
VERSION=v2.0.0 \
REGISTRY=myregistry.com \
TAG_LATEST=true \
make container

# 使用中国 Go 代理加速
GOPROXY=https://goproxy.cn,direct make container
```

## 其他 Make 目标

### 运行测试

```bash
# 运行所有单元测试
make test

# 运行测试并生成覆盖率报告
make test
# 查看覆盖率报告
go tool cover -html=coverage.out
```

### 代码质量检查

```bash
# 整理 Go 模块依赖
make modules

# 验证 Go 模块文件是否最新
make verify-modules

# CI 构建（包含模块验证和测试）
make ci
```

### 清理构建产物

```bash
# 清理所有构建产物
make clean
```

## 常见问题

### 1. Docker Buildx 未启用

**错误信息**:
```
buildx not enabled, refusing to run this recipe
```

**解决方法**:
```bash
# 创建并启用 buildx builder
docker buildx create --name multiarch --use
docker buildx inspect --bootstrap
```

### 2. Podman 构建失败

**问题**: Podman 可能不支持某些 Docker 特性

**解决方法**:
- 确保使用 Podman 4.0+ 版本
- 如果遇到问题，可以尝试使用 Docker
- Podman 仅支持单架构构建，确保 `ARCH` 参数正确

### 3. 跨平台构建失败

**问题**: 在非 Linux 平台上构建 Linux 镜像时失败

**解决方法**:
- 使用 Docker buildx 进行跨平台构建（推荐）
- 或者使用 `ARCH=linux-amd64 make container` 明确指定目标架构
- 确保 Dockerfile 中正确使用了 `TARGETPLATFORM` 和 `BUILDPLATFORM`

### 4. Go 模块下载慢

**问题**: 在中国大陆下载 Go 模块很慢

**解决方法**:
```bash
# 使用中国 Go 代理
GOPROXY=https://goproxy.cn,direct make container

# 或者在 Dockerfile 中设置
# 编辑 Dockerfile，修改 GOPROXY 默认值
```

### 5. 构建路径错误

**问题**: 构建时提示找不到包或文件

**解决方法**:
- 确保在项目根目录执行 make 命令
- 检查 `BIN` 变量是否与目录名匹配（默认：`velero-plugin-alibabacloud`）
- 验证 `PKG` 变量是否正确（默认：`github.com/AliyunContainerService/velero-plugin`）

### 6. 权限问题

**问题**: 构建镜像时提示权限不足

**解决方法**:
```bash
# Docker: 确保用户在 docker 组中
sudo usermod -aG docker $USER
# 重新登录后生效

# Podman: 通常不需要特殊权限
```

## 构建流程说明

### 本地构建流程

1. `make local` → 调用 `hack/build.sh`
2. `hack/build.sh` → 使用 `go build` 编译二进制
3. 输出到 `_output/bin/<GOOS>/<GOARCH>/<BIN>`

### 容器镜像构建流程

1. `make container` → 调用 Docker/Podman 构建
2. Dockerfile 多阶段构建：
   - **Builder 阶段**: 使用 `golang:1.24-bookworm` 编译二进制
   - **Runtime 阶段**: 使用 `alpine:3.22` 作为运行时镜像
3. 最终镜像包含编译好的二进制文件

### 多架构构建原理

- Docker Buildx 使用 QEMU 模拟器进行跨平台构建
- Dockerfile 使用 `--platform=$TARGETPLATFORM` 指定目标平台
- 构建参数通过 `ARG` 传递到 Dockerfile

## 最佳实践

1. **开发环境**: 使用 `make local` 快速构建和测试
2. **生产环境**: 使用 `make container` 构建镜像
3. **CI/CD**: 使用 `make ci` 进行完整的构建和测试
4. **多架构**: 使用 Docker buildx 构建多架构镜像以支持不同平台
5. **版本管理**: 使用 `VERSION` 变量明确指定版本号

## 相关文档

- [Velero 官方文档](https://velero.io/docs/)
- [Docker Buildx 文档](https://docs.docker.com/buildx/)
- [Podman 文档](https://podman.io/docs/)

## 贡献

如果遇到构建问题或有改进建议，欢迎提交 Issue 或 Pull Request。

