# LANShare P2P

一款去中心化的局域网即时通信工具，支持命令行和Web界面两种模式。

## 🌟 特性

- **P2P架构**: 无需中央服务器，点对点直接通信
- **自动发现**: 自动发现局域网中的其他客户端
- **双模式支持**: 
  - 命令行模式：轻量级，适合服务器环境
  - Web界面模式：现代化界面，自动打开浏览器
- **实时聊天**: 支持公聊和私聊功能
- **网卡选择**: 多网卡环境下可选择使用的网络接口
- **跨平台**: 支持 macOS、Linux、Windows
- **便携性**: 单一可执行文件，无需额外依赖

## 🚀 快速开始

### 构建

```bash
# 构建程序
go build -o build/lanshare lanshare.go

# 或者使用构建脚本
./build.sh
```

### 运行

```bash
# 交互式启动（推荐）
./build/lanshare

# Web模式
./build/lanshare -web

# 命令行模式
./build/lanshare -cli

# 指定用户名
./build/lanshare -name 张三

# Web模式 + 指定用户名
./build/lanshare -web -name 李四

# 查看帮助
./build/lanshare -help
```

## 📖 使用说明

### Web界面模式

1. 启动程序后会自动打开浏览器
2. 在Web界面中可以：
   - 查看在线用户列表
   - 发送公聊消息
   - 点击用户名快速私聊
   - 使用命令进行高级操作

### 命令行模式

启动后可以使用以下命令：

- 直接输入消息 - 发送公聊消息
- `/to <用户名> <消息>` - 发送私聊消息
- `/list` - 查看在线用户
- `/name <新名称>` - 更改用户名
- `/web` - 打开Web界面（仅Web模式下可用）
- `/quit` - 退出程序

## 🔧 命令行参数

```
用法: ./build/lanshare [选项]

选项:
  -cli          仅使用命令行模式
  -web          启用Web界面模式 (默认)
  -name string  指定用户名
  -h            显示帮助信息
```

## 🌐 网络配置

### 端口使用

- **P2P通信端口**: 8888 (TCP)
- **服务发现端口**: 9999 (UDP)
- **Web界面端口**: 8080 (HTTP，仅Web模式)

### 网卡选择

程序会自动检测可用的网络接口：

- 单网卡：自动使用
- 多网卡：提示用户选择

## 📁 项目结构

```
LANShare/
├── lanshare.go          # 主程序源码
├── build.sh             # 构建脚本
├── README.md            # 项目说明
├── go.mod               # Go模块文件
├── go.sum               # Go依赖校验
├── web/                 # Web界面文件
│   ├── index.html       # HTML模板
│   ├── style.css        # CSS样式文件
│   └── app.js           # JavaScript代码
├── build/               # 构建输出目录
│   └── lanshare         # 可执行文件
└── examples/            # 示例目录
```

## 🔨 开发

### 构建所有平台版本

使用构建脚本一次性构建所有平台：

```bash
# 构建所有平台版本
./build.sh
```

构建脚本会自动生成以下平台的可执行文件：

- **macOS**: Intel (amd64) 和 Apple Silicon (arm64)
- **Linux**: x86_64, ARM64, x86, ARM
- **Windows**: x86_64, x86, ARM64
- **FreeBSD**: x86_64, ARM64

### 手动构建特定平台

```bash
# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o build/lanshare-macos-arm64 lanshare.go

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o build/lanshare-macos-amd64 lanshare.go

# Linux (x86_64)
GOOS=linux GOARCH=amd64 go build -o build/lanshare-linux-amd64 lanshare.go

# Linux (ARM64)
GOOS=linux GOARCH=arm64 go build -o build/lanshare-linux-arm64 lanshare.go

# Windows (x86_64)
GOOS=windows GOARCH=amd64 go build -o build/lanshare-windows-amd64.exe lanshare.go

# Windows (ARM64)
GOOS=windows GOARCH=arm64 go build -o build/lanshare-windows-arm64.exe lanshare.go
```

### 技术架构

- **语言**: Go 1.19+
- **网络**: TCP (P2P通信) + UDP (服务发现)
- **Web框架**: Go标准库 `net/http`
- **前端**: 原生HTML/CSS/JavaScript
- **并发**: Goroutines + Channels

### 核心功能模块

#### 1. 网络发现模块
- `getLocalIP()`: 获取本地IP地址，支持多网卡选择
- `startDiscovery()`: 启动服务发现
- `listenBroadcast()`: 监听UDP广播
- `sendDiscoveryBroadcast()`: 发送服务发现广播

#### 2. 连接管理模块
- `connectToPeer()`: 连接到对等节点
- `acceptConnections()`: 接受传入连接
- `handlePeerConnection()`: 处理对等节点连接

#### 3. 消息处理模块
- `handleMessages()`: 处理接收到的消息
- `broadcastMessage()`: 广播消息到所有节点
- `sendMessageToPeer()`: 发送消息到特定节点

#### 4. 用户界面模块
- `startCLI()`: 命令行用户界面
- `handleCommand()`: 处理用户命令
- `startWebGUI()`: Web界面服务器

### 开发指南

#### 添加新功能

在 `lanshare.go` 中添加新的消息类型和处理逻辑：

1. **定义新消息类型**: 在 `Message` 结构体中添加新的类型
2. **添加处理逻辑**: 在 `handleMessages()` 函数中添加处理代码
3. **用户界面**: 在 `handleCommand()` 函数中添加新命令

#### 代码组织原则

- **单一职责**: 每个函数负责特定功能
- **模块化**: 功能按模块分离
- **清晰命名**: 函数和变量名称清晰表达用途
- **文档完整**: 重要功能都有相应注释

### 网络架构

#### 通信流程
1. **启动阶段**: 节点启动，选择网卡，开始广播
2. **发现阶段**: 监听广播，发现其他节点
3. **连接阶段**: 建立TCP连接，交换握手信息
4. **通信阶段**: 进行消息传输和用户交互

#### 技术特点

**优势**:
- **去中心化**: 无需中央服务器
- **自动发现**: 零配置网络发现
- **实时通信**: 低延迟P2P通信
- **简单部署**: 单文件部署

**限制**:
- **局域网限制**: 仅支持同一局域网
- **安全性**: 当前版本未加密
- **扩展性**: 大量节点时性能可能下降

### 故障排除

#### 常见问题
1. **无法发现节点**: 检查防火墙和网络配置
2. **连接失败**: 确认端口未被占用
3. **消息丢失**: 检查网络稳定性
4. **Web界面无法访问**: 确认8080端口未被占用

#### 调试方法
- 查看控制台输出
- 检查网络连接状态
- 使用网络工具测试端口连通性
- 检查Web文件是否存在（web/目录）

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

GPLv3 License

## 🔗 相关链接

- [项目仓库](https://github.com/ByteMini/LANShare)
- [问题反馈](https://github.com/ByteMini/LANShare/issues)

---

**LANShare P2P** - 让局域网通信更简单！
