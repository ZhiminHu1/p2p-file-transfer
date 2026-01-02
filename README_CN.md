# P2P 文件传输系统

一个用 Go 语言实现的健壮的点对点（P2P）文件传输系统。该系统支持高效的文件共享和基于分块的数据传输，利用中心服务器进行节点发现，并通过 mDNS 实现自动服务发现。


## 功能特性

- **mDNS 服务发现**：自动发现局域网内的中心服务器
- **节点发现**：中心服务器管理动态节点注册和追踪
- **分块文件传输**：文件分块处理，支持高效并行传输
- **并发传输**：工作池模式优化下载速度
- **文件完整性校验**：基于哈希的验证确保数据完整性
- **交互式 CLI**：用户友好的命令行界面，支持自动补全
- **心跳机制**：自动节点健康监控
- **可配置网络**：支持自定义绑定地址和端口

## 安装

### 环境要求

- Go 1.22.4 或更高版本

### 从源码构建

```bash
# 克隆仓库
git clone https://github.com/yourusername/p2p-file-transfer.git
cd p2p-file-transfer

# 编译可执行文件
go build -o p2p-transfer ./cmd/p2p-transfer

# 或使用 make
make build
```

## 快速开始

### 1. 启动中心服务器

```bash
# 默认配置：监听 0.0.0.0:8000
./p2p-transfer server

# 自定义地址
./p2p-transfer server --addr 192.168.1.100:8000

# 交互式模式（带命令行 Shell）
./p2p-transfer server --addr 0.0.0.0:8000 --interactive
```

**服务器 CLI 命令**（交互模式）：

| 命令 | 说明 |
|:-----|:-----|
| `status` | 显示服务器状态和已连接的节点 |
| `list peers` | 列出所有已连接的节点地址 |
| `exit` | 停止服务器并退出 |
| `help` | 显示可用命令 |

### 2. 启动 Peer 节点

```bash
# 终端 1 - 使用默认配置的节点
./p2p-transfer peer

# 终端 2 - 使用自定义地址的另一个节点
./p2p-transfer peer --addr 127.0.0.1:8002

# 启用自动发现功能（通过 mDNS 查找中心服务器）
./p2p-transfer peer --auto-discover

# 手动指定中心服务器地址
./p2p-transfer peer --server 192.168.1.100:8000

# 交互式模式
./p2p-transfer peer --interactive
```

**Peer CLI 命令**（交互模式）：

| 命令 | 说明 |
|:-----|:-----|
| `status` | 显示节点状态和连接信息 |
| `list files` | 列出所有已注册的文件 |
| `register <file>` | 注册文件以供共享 |
| `download <file-id>` | 根据文件 ID 下载文件 |
| `exit` | 退出节点 |

### 3. 注册和传输文件

```bash
# 注册文件以供共享（在 peer CLI 中）
register /path/to/your/file.txt

# 根据哈希 ID 下载文件
download abc123def456 /path/to/output/
```

或使用命令行参数：

```bash
# 启动时注册文件
./p2p-transfer peer --register /path/to/file.txt

# 启动时下载文件
./p2p-transfer peer --download <file-id>
```

## 命令参考

### 服务器命令

```bash
p2p-transfer server [flags]
```

| 参数 | 简写 | 默认值 | 说明 |
|:-----|:-----|:-------|:-----|
| `--addr` | `-p` | `0.0.0.0:8000` | 监听地址 |
| `--interactive` | `-i` | `false` | 以交互模式启动 |

### Peer 命令

```bash
p2p-transfer peer [flags]
```

| 参数 | 简写 | 默认值 | 说明 |
|:-----|:-----|:-------|:-----|
| `--addr` | `-a` | `127.0.0.1:8001` | 本节点监听地址 |
| `--server` | `-s` | `127.0.0.1:8000` | 中心服务器地址 |
| `--auto-discover` | - | `false` | 通过 mDNS 自动发现中心服务器 |
| `--register` | `-r` | - | 启动时立即注册/播种的文件路径 |
| `--download` | `-d` | - | 启动时立即下载的文件哈希 ID |
| `--interactive` | `-i` | `false` | 以交互模式启动 |

## 使用示例

### 场景 1：本地测试（单机）

```bash
# 终端 1：启动中心服务器
./p2p-transfer server

# 终端 2：启动第一个节点
./p2p-transfer peer --addr 127.0.0.1:8001

# 终端 3：启动第二个节点
./p2p-transfer peer --addr 127.0.0.1:8002

# 在终端 2 中注册文件
register ~/testfile.txt
# 记住返回的 file-id

# 在终端 3 中下载文件
download <file-id> ~/downloads/
```

### 场景 2：局域网部署

```bash
# 机器 A (192.168.1.100)：启动中心服务器
./p2p-transfer server --addr 0.0.0.0:8000

# 机器 B (192.168.1.101)：使用自动发现的节点
./p2p-transfer peer --auto-discover

# 机器 C (192.168.1.102)：手动指定服务器地址的节点
./p2p-transfer peer --server 192.168.1.100:8000
```

### 场景 3：使用交互模式

```bash
# 以交互模式启动节点
./p2p-transfer peer -i

# 在交互 Shell 中：
> status
Peer Server Running on: 127.0.0.1:8001
Central Server: 127.0.0.1:8000 (Connected)
Connected Peers: 0

> register ./myvideo.mp4
[INFO] File registered successfully. File ID: a1b2c3d4e5f6...

> download a1b2c3d4e5f6 /home/user/downloads/
[INFO] Starting download: a1b2c3d4e5f6
```

## 工作原理

### 架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                         P2P 网络                                 │
│                                                                  │
│   ┌──────────────┐      ┌──────────────┐      ┌──────────────┐ │
│   │   节点 A     │ ───> │  中心服务器  │ <─── │   节点 B     │ │
│   │  (种子节点)  │ <─── │   (索引)     │ ───> │  (下载节点)  │ │
│   └──────────────┘      └──────────────┘      └──────────────┘ │
│         │                                              │        │
│         └────────────── 直接 P2P 传输 ──────────────────┘        │
│                     (基于分块)                                   │
└─────────────────────────────────────────────────────────────────┘
```

### 服务发现流程

1. **中心服务器** 启动并通过 mDNS 广播服务
2. **节点** 使用 `--auto-discover` 扫描中心服务器
3. **节点** 连接到中心服务器并注册自己
4. **节点** 每 5 秒发送心跳保持连接
5. **节点** 向中心服务器注册文件
6. **节点** 向中心服务器查询文件位置
7. **节点** 直接从其他节点下载分块

### 文件传输过程

```
1. 种子节点注册文件
   └→ 文件分割成块（每块 4MB）
   └→ 每个块计算哈希并存储
   └→ 元数据发送到中心服务器

2. 下载节点请求文件
   └→ 向中心服务器查询文件 ID
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
│   └── p2p-transfer/          # 主 CLI 应用程序
│       ├── main.go            # 入口文件
│       ├── root.go            # 根命令
│       ├── server.go          # 服务器命令
│       └── peer.go            # 节点命令
├── central-server/            # 中心服务器实现
│   └── cserver.go             # 服务器逻辑（含 mDNS 广播）
├── peer/                      # 节点实现
│   ├── peer.go                # 节点服务器逻辑
│   ├── logic.go               # 文件处理逻辑
│   ├── workerpool.go          # 并发下载工作池
│   └── store.go               # 分块存储管理
├── pkg/
│   ├── discovery/             # mDNS 服务发现
│   │   ├── discovery.go       # 广播器和解析器
│   │   └── discovery_test.go  # 单元测试
│   ├── logger/                # 日志工具
│   ├── protocol/              # 消息定义
│   ├── storage/               # 文件存储和分块
│   └── transport/             # 网络传输层
│       ├── tcp_transport.go   # TCP 实现
│       └── transport.go       # 传输接口
└── README.md
```

## 配置

### 默认端口

| 组件 | 默认端口 |
|:-----|:-------|
| 中心服务器 | 8000 |
| 节点 | 8001 |

### mDNS 配置

- **服务类型**：`_p2p-transfer._tcp`
- **域名**：`local.`
- **发现超时**：5 秒

## 故障排除

### 节点无法连接到中心服务器

1. 检查中心服务器是否运行：
   ```bash
   # 使用 netstat 或 lsof 检查端口是否监听
   netstat -an | grep 8000
   ```

2. 验证防火墙设置允许端口通信

3. 尝试使用 `--auto-discover` 参数通过 mDNS 查找服务器

### mDNS 发现不工作

- 确保所有设备在同一网段
- 检查路由器是否启组播功能
- 使用手动 `--server` 地址作为备用方案

### 文件下载失败

1. 验证托管文件的节点在线
2. 检查可用磁盘空间
3. 查看日志中的错误信息

## 开发

### 运行测试

```bash
# 运行所有测试
go test ./...

# 运行测试并显示覆盖率
go test -cover ./...

# 运行特定包的测试
go test ./pkg/discovery/
```

### 跨平台编译

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o p2p-transfer-linux ./cmd/p2p-transfer

# macOS
GOOS=darwin GOARCH=amd64 go build -o p2p-transfer-mac ./cmd/p2p-transfer

# Windows
GOOS=windows GOARCH=amd64 go build -o p2p-transfer.exe ./cmd/p2p-transfer
```

## 路线图

- [ ] 节点间服务发现（Peer 间 mDNS）
- [ ] 跨网段中继服务器
- [ ] 断点续传功能
- [ ] 端到端加密
- [ ] 目录传输支持
- [ ] Web 界面
- [ ] 移动端应用

## 贡献

欢迎贡献！请遵循以下步骤：

1. Fork 本仓库
2. 创建新分支：`git checkout -b feature-branch-name`
3. 提交更改：`git commit -m 'Add some feature'`
4. 推送到分支：`git push origin feature-branch-name`
5. 提交 Pull Request

## 许可证

本项目采用 MIT 许可证。

## 致谢

- [github.com/grandcat/zeroconf](https://github.com/grandcat/zeroconf) - mDNS 服务发现库
- [github.com/spf13/cobra](https://github.com/spf13/cobra) - CLI 框架
- [github.com/c-bata/go-prompt](https://github.com/c-bata/go-prompt) - 交互式提示库
