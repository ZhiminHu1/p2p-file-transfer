# P2P 文件传输

[English](README.md) | [简体中文](README.zh-CN.md)

一个用 Go 实现的健壮的对等（P2P）文件传输系统。该系统实现了高效的文件共享和基于分块的对等数据传输，使用中央服务器进行节点发现，并支持 mDNS 自动服务发现。

## 特性

- **mDNS 服务发现**：自动发现局域网内的中央服务器
- **节点发现**：中央服务器管理动态节点注册和追踪
- **分块文件传输**：文件分块传输，支持高效并行下载
- **并发传输**：使用工作池模式优化下载速度
- **文件完整性验证**：基于哈希验证确保数据完整性
- **交互式 CLI**：用户友好的命令行界面，支持自动补全
- **心跳机制**：自动监控节点健康状态
- **可配置网络**：支持自定义绑定地址和端口

## 安装

### 环境要求

- Go 1.22.4 或更高版本

### 从源码构建

```bash
# 克隆仓库
git clone https://github.com/yourusername/p2p-file-transfer.git
cd p2p-file-transfer

# 构建可执行文件
go build -o p2p-transfer ./cmd/p2p-transfer

# 或使用 make
make build
```

### 使用 Docker

```bash
# 构建镜像
docker build -t p2p-transfer .

# 启动服务器和多个节点
docker-compose up -d
```

## 快速开始

### 1. 启动中央服务器

```bash
# 默认：监听 0.0.0.0:8000
./p2p-transfer server

# 自定义地址
./p2p-transfer server --addr 192.168.1.100:8000

# 交互式模式
./p2p-transfer server --addr 0.0.0.0:8000 --interactive
```

**服务器 CLI 命令**（交互模式）：

| 命令 | 描述 |
|:--------|:-----------|
| `status` | 显示服务器状态和已连接的节点 |
| `list peers` | 列出所有已连接的节点地址 |
| `exit` | 停止服务器并退出 |
| `help` | 显示可用命令 |

### 2. 启动节点

```bash
# 终端 1 - 使用默认设置启动节点
./p2p-transfer peer

# 终端 2 - 使用自定义地址启动另一个节点
./p2p-transfer peer --addr 127.0.0.1:8002

# 启用自动发现（通过 mDNS 查找中央服务器）
./p2p-transfer peer --auto-discover

# 指定中央服务器地址
./p2p-transfer peer --server 192.168.1.100:8000

# 交互模式
./p2p-transfer peer --interactive
```

**节点 CLI 命令**（交互模式）：

| 命令 | 描述 |
|:--------|:-----------|
| `status` | 显示节点状态和连接信息 |
| `register <file>` | 注册文件用于共享 |
| `download <file-id>` | 通过 ID 下载文件 |
| `exit` | 退出节点 |

### 3. 注册和传输文件

```bash
# 注册文件用于共享（在节点 CLI 中）
register /path/to/your/file.txt

# 通过哈希 ID 下载文件
download abc123def456
```

或使用命令行参数：

```bash
# 启动时注册文件
./p2p-transfer peer --register /path/to/file.txt

# 启动时下载文件
./p2p-transfer peer --download <file-id>
```

## 命令参考

### 全局选项

| 参数 | 简写 | 默认值 | 描述 |
|:-----|:-----|:-------|:-----------|
| `--help` | `-h` | - | 显示帮助信息 |

### Server 命令

```bash
p2p-transfer server [flags]
```

| 参数 | 简写 | 默认值 | 描述 |
|:-----|:-----|:-------|:-----------|
| `--addr` | `-a` | `0.0.0.0:8000` | 监听地址 |
| `--interactive` | `-i` | `false` | 交互模式 |

### Peer 命令

```bash
p2p-transfer peer [flags]
```

| 参数 | 简写 | 默认值 | 描述 |
|:-----|:-----|:-------|:-----------|
| `--addr` | `-a` | `127.0.0.1:8001` | 节点监听地址 |
| `--server` | `-s` | `127.0.0.1:8000` | 中央服务器地址 |
| `--auto-discover` | - | `false` | 通过 mDNS 自动发现中央服务器 |
| `--register` | `-r` | - | 启动时注册/分享的文件路径 |
| `--download` | `-d` | - | 启动时下载的文件哈希 ID |
| `--interactive` | `-i` | `false` | 交互模式 |

## 使用示例

### 场景 1：本地测试（单机）

```bash
# 终端 1：启动中央服务器
./p2p-transfer server

# 终端 2：启动第一个节点
./p2p-transfer peer --addr 127.0.0.1:8001

# 终端 3：启动第二个节点
./p2p-transfer peer --addr 127.0.0.1:8002

# 在终端 2 中，注册一个文件
register ~/testfile.txt
# 记下返回的 file-id

# 在终端 3 中，下载文件
download <file-id>
```

### 场景 2：局域网部署

```bash
# 机器 A (192.168.1.100)：启动中央服务器
./p2p-transfer server --addr 0.0.0.0:8000

# 机器 B (192.168.1.101)：启动节点，使用自动发现
./p2p-transfer peer --auto-discover

# 机器 C (192.168.1.102)：启动节点，手动指定服务器地址
./p2p-transfer peer --server 192.168.1.100:8000
```

### 场景 3：使用 Docker 模拟多节点

```bash
# 启动服务器和 3 个节点
docker-compose up -d

# 查看日志
docker-compose logs -f peer1

# 查看运行状态
docker-compose ps
```

### 场景 4：使用交互模式

```bash
# 启动交互模式
./p2p-transfer peer -i

# 在交互 shell 中：
> status
Peer Server Running on: 127.0.0.1:8001
Central Server: 127.0.0.1:8000 (Connected)
Connected Peers: 0

> register ./myvideo.mp4
[INFO] File registered successfully. File ID: a1b2c3d4e5f6...

> download a1b2c3d4e5f6
[INFO] Starting download: a1b2c3d4e5f6
```

## 工作原理

### 架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                         P2P 网络                                │
│                                                                  │
│   ┌──────────────┐      ┌──────────────┐      ┌──────────────┐ │
│   │    节点 A    │ ───> │  中央服务器  │ <─── │    节点 B    │ │
│   │  (分享者)    │ <─── │   (索引)     │ ───> │  (下载者)    │ │
│   └──────────────┘      └──────────────┘      └──────────────┘ │
│         │                                              │        │
│         └────────────── 直接 P2P 传输 ──────────────────┘        │
│                     (基于分块)                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 服务发现流程

1. **中央服务器** 启动并通过 mDNS 广播
2. **节点** 使用 `--auto-discover` 扫描中央服务器
3. **节点** 连接到中央服务器并注册自己
4. **节点** 每 5 秒发送心跳保持连接
5. **节点** 向中央服务器注册文件
6. **节点** 查询中央服务器获取文件位置
7. **节点** 直接从其他节点下载分块

### 文件传输流程

```
1. 分享者注册文件
   └→ 文件分割为分块（每块 4MB）
   └→ 每个分块计算哈希并存储
   └→ 元数据发送到中央服务器

2. 下载者请求文件
   └→ 向中央服务器查询文件 ID
   └→ 获取托管分块的节点列表
   └→ 使用工作池并行下载分块

3. 分块组装
   └→ 验证文件完整性
   └→ 重新组装原始文件
```

## 项目结构

```
p2p-file-transfer/
├── cmd/
│   └── p2p-transfer/          # 主 CLI 应用
│       ├── main.go            # 入口
│       ├── root.go            # 根命令
│       ├── server.go          # 服务器命令
│       └── peer.go            # 节点命令
├── central-server/            # 中央服务器实现
│   └── cserver.go             # 服务器逻辑与 mDNS 广播
├── peer/                      # 节点实现
│   ├── peer.go                # 节点服务器逻辑
│   ├── logic.go               # 文件处理逻辑
│   ├── workerpool.go          # 并发下载工作池
│   └── store.go               # 分块存储管理
├── pkg/
│   ├── discovery/             # mDNS 服务发现
│   ├── logger/                # 日志工具
│   ├── protocol/              # 消息定义
│   ├── storage/               # 文件存储和分块
│   └── transport/             # 网络传输层
├── Dockerfile                 # Docker 镜像构建
├── docker-compose.yaml        # Docker 编排配置
└── README.md
```

## 配置

### 默认端口

| 组件 | 默认端口 |
|:----------|:------------|
| 中央服务器 | 8000 |
| 节点 | 8001 |

### mDNS 配置

- **服务类型**：`_p2p-transfer._tcp`
- **域名**：`local.`
- **发现超时**：5 秒

## 故障排查

### 节点无法连接到中央服务器

1. 检查中央服务器是否运行：
   ```bash
   # 使用 netstat 检查端口是否监听
   netstat -an | grep 8000
   ```

2. 确认防火墙允许端口通信

3. 尝试使用 `--auto-discover` 参数通过 mDNS 查找服务器

### mDNS 发现不工作

- 确保所有设备在同一网段
- 检查路由器是否启用组播
- 使用手动 `--server` 地址作为后备方案

### 文件下载失败

1. 验证托管文件的节点在线
2. 检查可用磁盘空间
3. 查看日志获取错误信息

## 开发

### 运行测试

```bash
# 运行所有测试
go test ./...

# 运行测试并生成覆盖率报告
go test -cover ./...

# 运行特定包的测试
go test ./pkg/discovery/
```

### 跨平台构建

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o p2p-transfer-linux ./cmd/p2p-transfer

# macOS
GOOS=darwin GOARCH=amd64 go build -o p2p-transfer-mac ./cmd/p2p-transfer

# Windows
GOOS=windows GOARCH=amd64 go build -o p2p-transfer.exe ./cmd/p2p-transfer
```

## 路线图

- [ ] 节点间 mDNS 服务发现
- [ ] 中继服务器支持跨网络发现
- [ ] 断点续传
- [ ] 端到端加密
- [ ] 目录传输支持
- [ ] Web UI
- [ ] 移动应用

## 贡献

欢迎贡献！请按以下步骤操作：

1. Fork 本仓库
2. 创建新分支：`git checkout -b feature-branch-name`
3. 提交更改：`git commit -m 'Add some feature'`
4. 推送到分支：`git push origin feature-branch-name`
5. 提交 Pull Request

## 许可证

本项目采用 MIT 许可证。

## 致谢

本项目参考借鉴了 [tarunkavi/p2p-file-transfer](https://github.com/tarunkavi/p2p-file-transfer)。

感谢以下开源项目：

- [github.com/grandcat/zeroconf](https://github.com/grandcat/zeroconf) - mDNS 服务发现库
- [github.com/spf13/cobra](https://github.com/spf13/cobra) - CLI 框架
- [github.com/c-bata/go-prompt](https://github.com/c-bata/go-prompt) - 交互式提示库
