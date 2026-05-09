# proxywatch — 设计文档

- **状态**：设计已批准，等待实现计划（writing-plans）
- **日期**：2026-05-09
- **作者**：与用户协作产出（brainstorming session）
- **目标系统**：CLIProxyAPI（CPA）部署，主机 `<server-ip>`，对外 `https://api.example.com/`

## 1. 背景与目标

CPA 通过 SOCKS5 上游代理 `socks5h://...@us.miyaip.online:1111` 出网，访问 OpenAI / Google / Anthropic 等上游 AI 服务。代理供应商 miyaIP 提供的是"动态移动代理"（实测出口为 T-Mobile 4G/5G IP），但**当前账户配置为手动轮换** —— 出口 IP 在 miyaIP 后台手动点按钮才换。

**问题**：当上游服务（主要是 OpenAI / ChatGPT）开始返回 4xx（403/429）说明出口 IP 已被识别/限流，需要更换 IP 才能恢复服务。当前流程完全依赖人工发现 + 人工换 + 人工重测。

**目标**：构建一个独立微服务 `proxywatch`，做到：

1. **持续监控**代理出口 IP 与上游服务健康度
2. **自动判定**异常（基于上游 4xx 与主动探活）
3. **及时告警**通过 Telegram 通知用户去 miyaIP 后台换 IP
4. **自动验证**用户换 IP 后链路恢复，进入下一轮 HEALTHY 状态
5. 提供轻量 Web 面板查看状态、历史、调参

**非目标**：
- 不调用 miyaIP 的轮换 API（因为 miyaIP 后台无 API，已确认）
- 不修改 CPA 源码或 fork CPA-Manager
- 不在 IP 变化时重启 cli-proxy-api 容器（已确认下个 SOCKS5 请求自然走新 IP）
- 不做多代理供应商抽象（YAGNI；只服务当前 miyaIP 用例）

## 2. 总体架构

```
                ┌───────────────────────────────────────────────────┐
                │                 proxywatch                        │
                │  ┌────────┐    ┌─────────┐    ┌────────────┐      │
                │  │Watcher │ →  │Decision │ →  │Executor    │      │
                │  │(probe+ │    │Engine   │    │(alert+     │      │
                │  │ tail)  │    │(state   │    │ verify)    │      │
                │  │        │    │ machine)│    │            │      │
                │  └────┬───┘    └────┬────┘    └────┬───────┘      │
                │       │             │               │              │
                │       └────────┬────┴───────┬───────┘              │
                │                ▼            ▼                      │
                │           SQLite       Notifier                    │
                │           (state +    (Telegram + buffer)          │
                │            history)                                │
                │                ▲                                   │
                │                │                                   │
                │           HTTP API + 内嵌静态前端                  │
                │                ▲                                   │
                └────────────────┼───────────────────────────────────┘
                                 │
                          nginx → proxywatch.example.com
                                 │
                          (用户浏览器)

外部依赖（容器内可达）：
  - 主动探活：经 SOCKS5 → api.openai.com/v1/models, ipify
  - 被动监听：read-only mount /opt/cliproxyapi/logs/
  - 配置降级源：CPA management API /v0/management/logs（兜底）
  - 推送：api.telegram.org（直连，不走 SOCKS5）
```

### 部署形态

加进 `/opt/cliproxyapi/docker-compose.yml`，与 `cli-proxy-api`、`cpa-manager` 共享默认 docker network，但**不**共享 `docker.sock`（无重启需求 → 无此权限）。

```yaml
proxywatch:
  image: proxywatch:latest        # 自建镜像
  container_name: proxywatch
  ports:
    - "127.0.0.1:18318:18318"
  volumes:
    - proxywatch-data:/data       # SQLite 持久化
    - ./logs:/cpa-logs:ro         # CPA 日志只读 mount
    - ./proxywatch.yaml:/etc/proxywatch.yaml:ro  # 配置文件
  environment:
    - CPA_UPSTREAM_URL=http://cli-proxy-api:8317
    - CPA_PROXY_URL=socks5h://...@us.miyaip.online:1111  # 探活用，与 CPA config 同一份
    - CPA_MANAGEMENT_KEY=<bearer>
  restart: unless-stopped
  depends_on:
    - cli-proxy-api
```

nginx 反代复用 `api.example.com` / `cpa.example.com` 的模式，新增 `cpa.example.com` 风格的 `proxywatch.example.com` 站点 + Let's Encrypt。

### 实现语言：Go

理由：
- 与 CPA / CPA-Manager 同栈，运维一致
- 单二进制 + 静态资源 embed，镜像 ~20–30 MB
- 主机内存 3.8 GB / 0 swap，Go 比 Python/Node 内存占用低一档
- 标准库 net/http、database/sql、io/fs 即可覆盖；前端用单页 React+TypeScript（构建产物 embed 进二进制，参考 CPA-Manager 模式）

## 3. 组件细节

### 3.1 Watcher

两路探针并行运行，结果均写入 `probes` 表。

#### Active probe（主动）

- **频率**：默认 60 秒，可在面板调
- **目标**：`https://api.openai.com/v1/models`，经 SOCKS5
- **附带**：`https://api.ipify.org` 拿当前出口 IP（直连主代理，不需要鉴权）
- **超时**：连接 10s + 总 15s
- **降级**：ipify 失败时依次尝试 `ifconfig.me`、`api.myip.com`，三个都挂才记 `exit_ip=null`
- **输出**：`(ts, kind=active, target, http_code, latency_ms, exit_ip, ok)`

#### Passive log tail（被动）

- **数据源**：`/cpa-logs/main.log`（容器内只读 mount）+ `/cpa-logs/error-v1-chat-completions-*.log`（按文件创建事件捕获）
- **机制**：tail-follow `main.log`（用 inotify / fsnotify），从启动时记下的 offset 增量读
- **解析**：每行匹配 CPA 日志格式 `[ts] [reqid] [level] [file:line] message`，从 message 里抽 HTTP 状态码（CPA 在请求结束行会写状态码——**实现阶段需先抓样本验证 grep 模式**）
- **统计**：每 30 秒结算一次 5 分钟滚动窗口的 4xx 计数（403、429 计入；401 单独计入但不计入触发器）
- **降级**：如果 `main.log` 解析失败率 > 50%（CPA 日志格式变了），自动切到拉 `/v0/management/logs` 增量（按 `latest-timestamp` 取 delta）；面板显示 "passive-mode=api-fallback"

### 3.2 Decision Engine

#### 状态机

```
HEALTHY
  │
  ├──[5min 内 403/429 ≥ 3]──────► SUSPECT
  ├──[连续 active probe 失败 ≥ 3]─► SUSPECT
  │
SUSPECT
  │
  ├──[继续观察 60s 后仍异常]─────► ROTATING
  ├──[60s 内自愈]──────────────► HEALTHY
  │
ROTATING
  │
  ├──[发送 Telegram 告警]
  ├──[提高探活频率到每 5s 一次]
  ├──[探到 exit_ip 变化]────────► VERIFYING
  ├──[用户面板点"我换好了"]───► VERIFYING
  ├──[超时 10 分钟未变]─────────► ALERT_ONLY
  │
VERIFYING
  │
  ├──[active probe 200 OK]──────► COOLDOWN
  ├──[5 次内仍 4xx]────────────► ROTATING（认为换了一个还是不行）
  │
COOLDOWN（默认 120s）
  │
  └──[过期]────────────────────► HEALTHY

ALERT_ONLY
  │
  └──[面板点"恢复自动化"或自愈检测]─► HEALTHY
```

#### 关键参数（默认值，全部可在面板改）

| 参数 | 默认 | 含义 |
|---|---|---|
| `active_probe_interval` | 60s | 主动探活频率 |
| `active_probe_target` | `https://api.openai.com/v1/models` | 探活目标 |
| `passive_window` | 5m | 4xx 滚动窗口长度 |
| `passive_threshold` | 3 | 窗口内触发 SUSPECT 的 403/429 数 |
| `active_failure_threshold` | 3 | 连续 active 失败触发 SUSPECT |
| `suspect_observation` | 60s | SUSPECT 状态再观察时长 |
| `rotating_probe_interval` | 5s | ROTATING/VERIFYING 状态加密探活 |
| `rotating_timeout` | 10m | ROTATING 等多久换不到新 IP 就 ALERT_ONLY |
| `verifying_max_attempts` | 5 | VERIFYING 时最多探几次 |
| `cooldown` | 120s | 冷却时长 |
| `consecutive_rotation_failures_to_alert_only` | 2 | 连续两次轮换都没解决 → ALERT_ONLY |

### 3.3 Executor

仅"分支 B"（半自动）：

```
Trigger (state → ROTATING)
  ↓
Notifier.send(
  text = "⚠️ 代理异常\n当前 IP: <old_ip>\n触发原因: <reason>\n请去 miyaIP 后台换 IP，proxywatch 会自动检测",
  level = WARNING)
  ↓
[ROTATING 状态下 Watcher 提高探活频率到 5s/次]
  ↓
（用户在 miyaIP 后台手动换 IP，~30s-2min）
  ↓
[Watcher 探到 exit_ip != old_ip] OR [用户点面板"我换好了"]
  ↓
state → VERIFYING
  ↓
连续 active probe 200 OK
  ↓
state → COOLDOWN, 写 rotations 表
  ↓
Notifier.send(
  text = "✅ 已恢复\n旧 IP: <old>\n新 IP: <new>\n用时: <Xs>",
  level = INFO)
```

异常路径：
- ROTATING 超时未变 IP → 推 `❌ 10min 内未检测到 IP 变化，proxywatch 暂停自动化`，state → ALERT_ONLY
- VERIFYING 5 次仍 4xx → 推 `⚠️ 新 IP 仍然 4xx，再等你下一轮换`，state → ROTATING，并将 incident 的 `rotation_count += 1`
- 当 incident 的 `rotation_count >= 2` 且新一次 ROTATING 又失败 → state → ALERT_ONLY，停止自动化直到人工恢复

### 3.4 Notifier（Telegram）

- 配置：`telegram.bot_token`、`telegram.chat_id`，存 `config_kv` 表，**面板可改**
- 发送：直连 `https://api.telegram.org/bot<token>/sendMessage`，**不走 SOCKS5**（SOCKS5 挂了反而救不了告警）
- 重试：3 次指数退避（1s, 4s, 16s）；失败入 `notify_queue` 表，下次成功时批量补发（合并相邻同事件）
- 防风暴：单 incident 最多 2 条告警（开始 + 结束），中间状态变化只入库不推送
- 测试支持：`POST /api/test-notify` 触发测试消息（不影响状态机）

### 3.5 Storage（SQLite）

```sql
-- 探针历史
CREATE TABLE probes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ts          INTEGER NOT NULL,           -- unix epoch ms
    kind        TEXT NOT NULL,              -- 'active' | 'passive'
    target      TEXT,                       -- active: URL; passive: log file
    http_code   INTEGER,                    -- 0 表示连接错误
    latency_ms  INTEGER,
    exit_ip     TEXT,
    ok          INTEGER NOT NULL,           -- 0/1
    raw_error   TEXT
);
CREATE INDEX idx_probes_ts ON probes(ts);

-- incident（一次"健康→异常→恢复/告警"周期）
CREATE TABLE incidents (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at      INTEGER NOT NULL,
    ended_at        INTEGER,
    trigger_reason  TEXT NOT NULL,           -- 'passive_4xx' | 'active_failure'
    initial_state   TEXT,
    terminal_state  TEXT,                    -- 'recovered' | 'alert_only' | 'open'
    rotation_count  INTEGER DEFAULT 0
);

-- 单次轮换尝试
CREATE TABLE rotations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    incident_id     INTEGER NOT NULL,
    started_at      INTEGER NOT NULL,
    ended_at        INTEGER,
    old_ip          TEXT,
    new_ip          TEXT,
    detection_method TEXT,                   -- 'auto' | 'manual_button'
    ok              INTEGER,                  -- 0/1
    error           TEXT,
    FOREIGN KEY (incident_id) REFERENCES incidents(id)
);

-- 告警队列（已发 + 失败待重发）
CREATE TABLE notifications (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ts          INTEGER NOT NULL,
    incident_id INTEGER,
    level       TEXT NOT NULL,                -- 'info' | 'warning' | 'error'
    text        TEXT NOT NULL,
    sent_at     INTEGER,
    error       TEXT,
    retry_count INTEGER DEFAULT 0
);

-- 可调参数（面板改）
CREATE TABLE config_kv (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
```

容量：每分钟 ~1 条 active probe + 几十条 passive 行 → 一天 ~10万行。SQLite 处理 GB 级别没问题。**保留策略**：probes 表 30 天后归档（DELETE），incidents/rotations/notifications 永久保留。

### 3.6 HTTP API + 前端

#### API 端点

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/status` | 当前状态机、最新 active probe、当前 exit_ip、open incident |
| GET | `/api/probes?since=<ts>&kind=...&limit=...` | 探针历史（前端时间线用） |
| GET | `/api/incidents?limit=20` | 最近 incidents |
| GET | `/api/rotations?limit=20` | 最近轮换 |
| POST | `/api/confirm-rotation` | 用户点"我换好了"，强制进 VERIFYING |
| POST | `/api/resume-automation` | ALERT_ONLY 状态下手动恢复 |
| POST | `/api/test-notify` | 推一条测试消息 |
| GET | `/api/settings` | 当前可调参数 |
| PUT | `/api/settings` | 改可调参数（含 telegram） |

鉴权：与 CPA-Manager 同款 —— 部署时通过环境变量设置 `PROXYWATCH_KEY`；用户首次访问 `proxywatch.example.com` 时浏览器要求填同一个 key，存进浏览器 localStorage，后续请求带在 `Authorization: Bearer <key>` 头里；服务端字符串等值比较。**与 CPA 的 management key 完全隔离**——两个独立 key，两个独立面板。

#### UI 区块

1. **顶部状态条**：状态机当前状态（颜色：绿/黄/红）、当前出口 IP、最后 active probe 时间
2. **快速动作**：`我换好了，重测` 按钮（仅 ROTATING/SUSPECT 时高亮）
3. **Active probe 时间线**（最近 1h，~60 个点的 sparkline）
4. **Passive 4xx 滚动窗口**（5 分钟实时计数，与阈值对比）
5. **Incidents 列表**（表格：开始/结束/触发原因/轮换次数/最终状态）
6. **Rotations 列表**（表格：旧 IP / 新 IP / 检测方式 / 用时 / 成功）
7. **Settings 表单**（阈值、频率、Telegram bot、cooldown）

## 4. 错误处理与边界

| 场景 | 处理 |
|---|---|
| 401 vs 403/429 区分 | 默认 401 = api-key 无效，单独告警 `<api-key 无效>`，**不**计入触发器。注意启发式有边界：上游账号被风控也可能返回 401；如果短时间内 401 暴涨同时活跃账号没动，面板显示"401 spike"提示但仍不自动轮换 |
| ipify 全挂导致 exit_ip 拿不到 | exit_ip 记 null。在 ROTATING 时只能靠"用户点'我换好了'"按钮推进；自动检测路径暂不可用。面板顶部红条提示"exit_ip lookup down" |
| 代理网关 `us.miyaip.online:1111` 直接 TCP 不通 | 探活记 `proxy_down`，单独 Telegram 告警，**不**进入 ROTATING（换 IP 也救不了网关）；网关恢复后状态机自愈 |
| ipify / 备用 IP 服务全挂 | 见上一行 |
| CPA `main.log` 格式与预期不符 | 启动时跑 100 行解析正确率自检；< 50% 进 api-fallback 模式（拉 `/v0/management/logs` 增量），日志告警一次（不推 Telegram） |
| Telegram 不可达（token 错 / TG 被封 / 网络挂） | 重试 3 次 → 入 `notifications` 队列；面板顶部红条提示"Telegram 通道异常"；不阻塞状态机 |
| proxywatch 自身重启 | docker `restart: unless-stopped` 自动拉起；启动时读最近未关闭 incident，状态恢复为 `HEALTHY` 并继续观察（避免重启刷屏告警） |
| ROTATING 冷却期内又判异常 | COOLDOWN（120s）期间所有触发被吞掉，仅入 probes 表 |
| 连续 2 次 ROTATING 都没解决 | 进 ALERT_ONLY，停止自动化；要求手动恢复 |
| SQLite 文件损坏 | docker volume snapshot；启动时跑 `PRAGMA integrity_check`；坏了备份后重建 schema 继续跑 |

## 5. 测试策略

| 层级 | 内容 | 工具 |
|---|---|---|
| 单元 | 状态机所有合法/非法转换、滚动窗口、错误分类、配置序列化 | Go test，覆盖率目标 80%+ |
| 集成 | 起 mock 上游（可配置返回 200/403/429）、mock SOCKS5、mock telegram；驱动 watcher → decision → executor 全链路 | testcontainers-go 或自写 docker compose |
| 冒烟 | 部署到 staging（先用低阈值：active=10s、threshold=2）24h 观察，看是否有误报 | 部署后人工观察 |
| 演练 | `proxywatch drill` CLI 子命令一键模拟假故障，验证 Telegram + 面板告警端到端 | 内置；建议每周跑一次 |

## 6. 部署步骤（高层）

1. 实现 + 构建镜像，推到本机或私有 registry
2. 加 DNS：`proxywatch.example.com` A → `<server-ip>`
3. 写 `/opt/cliproxyapi/proxywatch.yaml`（初始配置）
4. 改 `/opt/cliproxyapi/docker-compose.yml` 加 `proxywatch` 服务
5. 写 `/etc/nginx/sites-available/proxywatch.example.com`，certbot 申请证书
6. `docker compose up -d proxywatch`，nginx reload
7. 浏览器访问 `https://proxywatch.example.com`，填 PROXYWATCH_KEY、Telegram bot_token + chat_id
8. 跑一次 `/api/test-notify` 验告警通道
9. 跑一次 `proxywatch drill incident` 验完整链路
10. 观察 24h 冒烟

## 7. 待实现阶段确认的细节

1. **CPA 主日志的 HTTP 状态码出现位置/格式**：需取样 `/opt/cliproxyapi/logs/main.log` 中含状态码的行，确定 grep 模式
2. **CPA-Manager 是否对外暴露同一份 management key**：如果是，需要权衡安全（多了一个用得上 key 的服务）
3. **proxywatch 自身的 PROXYWATCH_KEY 怎么管理**：建议生成新的，与 CPA 的 management key **隔离**，写在 `proxywatch.yaml` 里
4. **CPA 是否会因为出口 IP 改变而重置长连接**：和 OpenAI 客户端的连接是否需要 cli-proxy-api 主动断开重连。如果发现需要，再加 `force-restart` 开关

## 8. 范围之外（明确排除）

- 不做"多代理供应商"抽象（YAGNI）
- 不做"按客户/按 API key 分流路由"
- 不做"自动调 miyaIP API"（已确认 miyaIP 无 API）
- 不做"前端国际化"——中文 + 英文 hard-coded 即可
- 不替代 CPA-Manager 的功能（usage 分析仍由 CPA-Manager 负责）
