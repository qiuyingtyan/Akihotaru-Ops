# 系统架构与模块约定

## 1. 架构总览
运维面板由 Go (Gin) 后端 + Vue3 前端组成，构建为单个二进制文件。

## 2. 核心模块与数据设计

### 业务服务与生产自愈 (App Services)
- 后端位置：`backend/internal/collect/app_service.go`
- 数据库表：`ops_app_services`
- 容灾持久化文件：`/workspace/opsweb/app_services.json`（断电重启未连上 PG 时自动降级生效）
- Systemd 命名：`ops-{name}.service`，位于 `/etc/systemd/system/`
- 特性：
  - 启动依赖编排（After/Wants，Level 1 基础 -> Level 2 核心 -> Level 3 应用）
  - 崩溃自动重启（Restart=always，RestartSec=5s）
  - 断电开机自愈（WantedBy=multi-user.target）
  - 状态探活双重保障：Systemd 属性 + 端口 TCP 连通 + /proc 进程 PID/CPU/RSS
  - 开机自检诊断（Uptime、服务就绪率、一键修复拉起）
  - 生产交付包一键打包导出（含单元文件、安装脚本、健康自检脚本）

### 存储防爆守卫 (Storage Guard)
- 后端位置：`backend/internal/collect/storage_guard.go`
- 路由前缀：`/api/storage`
- 特性：
  - 挂载点容量水位监控（预警水位 85%、临界水位 90%）
  - 大文件雷达 TOP 20（扫描超大日志和数据文件）
  - 安全截断（基于 os.Truncate / truncate -s 0 清空日志，不破坏正在运行的进程句柄，立即 100% 释放磁盘空间）
  - 历史归档安全清理（清理 7 天前已压缩的 *.gz / *.zip / *.bak 与 /tmp 临时文件）
  - Logrotate 规则自动纳管（生成 /etc/logrotate.d/opsweb-services，采用 copytruncate、50MB 自动分卷与压缩）
