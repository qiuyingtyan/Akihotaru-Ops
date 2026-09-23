# 秋萤云台 · AkiHotaru

> **秋夜流萤，微光守候，静默自愈。**
> 
> *AkiHotaru* 是专为 Linux 生产集群、云原生实例与边缘物理节点打造的一站式轻量化智能运维治理中心与自愈工作台。

---

## 🌟 核心特性

- 🖥️ **多服务器纳管工作台（像 FinalShell 一样自由切换）**
  - 支持多主机资产一键添加、标签分组（生产 / 测试 / 集群 / 边缘）与毫秒级探活感知；
  - 自由切换管理目标，支持卡片与列表视图，配置本地加密持久化。
- 🛡️ **生产服务自愈治理与依赖编排 (App Services)**
  - 三级启动依赖拓扑（Level 1 基础存储 ➜ Level 2 核心骨架 ➜ Level 3 上层业务）；
  - 原生 Linux Systemd 纳管，支持断电开机自保与崩溃毫秒级拉起；
  - 生产交付包（Unit 单元文件 + 安装脚本 + 开机自检）一键打包导出。
- 🚨 **存储防爆守卫 (Storage Guard)**
  - 挂载点多级容量水位雷达（85% 预警 / 90% 紧急防爆）；
  - 全局 TOP 20 膨胀大文件扫描雷达；
  - 基于句柄无损的 `truncate` 安全截断，不重启、不杀进程立即 100% 释放磁盘空间；
  - 自动化 `logrotate` 规则纳管与历史压缩日志无害清理。
- 📄 **语义分词着色日志平台 (LogViewer)**
  - 全自动智能分词：时间戳冰蓝、异常栈帧与 Caused by、状态码与日志级别语义着色；
  - 排版模式一键切换：智能自动换行 vs. 单行横向极客流；
  - 内存防溢出环形缓冲区与关键词高亮追溯。
- 🌸 **秋萤暗夜霓虹桌面客户端 (`desktop/`)**
  - 基于 Wails v2 (Go + Vue 3) 深度打造，12MB 纯净体积，常驻内存仅约 26MB；
  - 沉浸式无边框微光自绘顶栏、呼吸状态灯、胶囊式节点快切与系统级窗口平滑微动效；
  - 原生桌面单实例互斥锁与托盘守护。
- 🤖 **安全受控 AI 智能运维助手**
  - 对接任意兼容 OpenAI / DeepSeek / 通义千问等大语言模型；
  - 三级安全管控：只读查询自动执行、写操作交互确认卡片、危险 Shell 命令严格黑名单硬拦截。

---

## 📂 仓库架构

```
.
├── backend/            Go 服务端 (Gin 核心 / 采集 / 自愈 / AI / 存储守卫)
│   ├── cmd/server/     服务端启动入口
│   ├── internal/api/   路由、鉴权、多主机探活接口与操作审计
│   ├── internal/ai/    AI 助手与命令安全分类引擎
│   ├── internal/collect/ 服务依赖自愈、存储防爆、Docker 与 CI/CD 日志清洗
│   └── internal/web/   内嵌前端产物 (go:embed)
├── frontend/           Vue 3 + Vite 运维控制台前端源码
└── desktop/            秋萤云台桌面端 (Wails v2 + Vue 3 桌面壳层工程)
```

---

## 🚀 快速开始

### 1. 服务端构建 (Linux)

```bash
# 1. 构建前端产物
cd frontend && npm install && npm run build

# 2. 构建独立单二进制可执行文件
cd ../backend && GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o ../akihotaru-server ./cmd/server
```

在目标 Linux 服务器执行：
```bash
./akihotaru-server
```
默认监听 `9800` 端口，首次启动将自动初始化管理员账户及数据库表结构。

### 2. 桌面客户端构建 (Windows / macOS / Linux)

```bash
cd desktop
wails build
```
产物将输出至 `desktop/build/bin/akihotaru.exe`，双击即可启动沉浸式控制台。

---

## 📜 开源协议

本项目遵循 MIT 协议开源。
