# pf3090 服务器运维面板

Go/Gin + Vue3 单二进制运维面板，部署在 pf3090（10.8.0.2，VPN 内网地址）。

## 访问
- 地址: http://10.8.0.2:9800
- 登录: 账号密码登录，账号存储在 PostgreSQL（pgsql-baq 容器，127.0.0.1:5433/opsweb 库，ops_users 表 bcrypt 哈希）。首次启动自动建表并播种 admin/123456（取自 `OPSWEB_USER`/`OPSWEB_PASS`，仅表空时生效）。登录后签发随机会话 token（7 天有效，持久化在 ops_sessions 表，服务重启不掉登录态），所有 API 走 `Authorization: Bearer` 头。可在面板「账号」页增删用户/改密，操作审计存 ops_audit 表（保留 90 天）
- 路由: 总览 / 容器 / 项目 / CI/CD / 系统服务 / 日志 / 告警 / AI 助手

## 功能
- **总览**: CPU/内存/负载/磁盘/网卡速率（3 秒缓存防刷），采样曲线支持 1/3/7/30 天区间，重启不丢失
- **容器**: 全部 Docker 容器状态、启停/重启（有确认弹窗）、日志查看
- **项目**: 办案区/三中心/vocedu/VLM 容器组成、磁盘占用、最近备份；支持一键重新部署（含部署输出查看，同一时间只允许一个部署任务）
- **CI/CD**: GitLab/Runner/Nacos 健康状态、最近 30 条构建记录（成功/失败/耗时/完成时间，从 Runner 日志解析，60 秒缓存）、部署脚本、Runner 日志
- **系统服务**: 关键 systemd 服务状态与启停（stop/restart 有确认弹窗）
- **日志**: 白名单目录文件 tail（禁止路径穿越，最大读 8MB）
- **告警**: 磁盘>80%/90%、内存>80%/90%、load1>16、异常容器、failed 服务；活跃/历史事件展示；配置 `OPSWEB_WEBHOOK` 环境变量（企业微信/钉钉机器人）可启用 webhook 通知
- **审计**: 所有容器/服务/项目操作记录到 `/workspace/opsweb/audit.log`（时间、来源 IP、对象、动作、结果）
- **AI 助手**: 对接 OpenAI 兼容接口（DeepSeek/Qwen/GLM 等）的对话式运维。环境变量 `OPSWEB_AI_KEY`（必填，不配则功能隐藏）、`OPSWEB_AI_BASE_URL`（默认 `https://api.deepseek.com`）、`OPSWEB_AI_MODEL`（模型名）。安全机制：只读查询（状态/日志/负载等 15 个工具）自动执行；容器/服务/部署等写操作弹卡片需用户点击批准；shell 命令三级管控——纯只读白名单（docker ps、systemctl status 等）自动执行，命中黑名单（rm -rf、关机、清防火墙、覆写认证文件等 23 类）直接硬拒绝且不可批准，其余需批准且附带风险点提示；批准请求 5 分钟过期、一次性、只能本人处理；单轮最多 8 次工具调用；AI 全部调用（含被拒/被拦截）写入审计日志和 ops_audit 表

## 结构
```
backend/            Go 后端 (Gin)
  cmd/server/       入口（-version / -token / OPSWEB_TOKEN）
  internal/api/     路由、鉴权（仅 Authorization Header）、操作审计
  internal/ai/      AI 助手（OpenAI 兼容客户端、工具注册表、shell 安全分类、批准流）
  internal/collect/ 采集（/proc、docker、systemctl）、告警、流水线解析
  internal/web/     内嵌前端 dist (go:embed)
frontend/           Vue3 + Vite 前端源码（登录页、轮询随页面可见性暂停）
```

## 构建与部署
```
cd frontend && npm install && npm run build   # 输出到 backend/internal/web/dist
cd backend && GOOS=linux GOARCH=amd64 go build \
  -ldflags "-s -w -X main.version=x.y.z -X main.buildTime=$(date +%Y-%m-%d_%H:%M)" \
  -o ../opsweb-linux-amd64 ./cmd/server
scp opsweb-linux-amd64 pf3090@10.8.0.2:/tmp/
ssh pf3090@10.8.0.2 'sudo systemctl stop opsweb; cp /tmp/opsweb-linux-amd64 /workspace/opsweb/opsweb && chmod +x /workspace/opsweb/opsweb; sudo systemctl start opsweb'
```

服务器端 systemd 单元: `/etc/systemd/system/opsweb.service`（开机自启、崩溃自动重启）
