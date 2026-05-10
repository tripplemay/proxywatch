# cpa-stress — 设计文档

- **状态**：设计已批准，等待实现计划（writing-plans）
- **日期**：2026-05-10
- **作者**：与用户协作产出（brainstorming session）
- **目标系统**：CLIProxyAPI 部署在 `https://api.vpanel.cc`，路由 10 个 Codex OAuth 账号通过 miyaIP SOCKS5 代理出网

## 1. 背景与目标

CPA 部署稳定运行后，用户希望了解**当前部署的承载能力边界**——在多大并发 / 多少 RPS 之前 Codex 链路保持可用，何处开始失败。

**测试目标**（明确接受的代价）：
- 探到真实"边界"：连 OpenAI 账号级 rate limit、IP 被 ban、账号被挂起 / 销号都接受
- 单次跑完产出**完整测试报告**：每条请求的请求内容、响应内容、in/out token、出口 IP、响应延迟，加上聚合统计

**作用域**：
- 仅压 Codex/OpenAI 路径（`gpt-*` 模型）
- 不动 Gemini、Kimi 路径（保留日常可用性）
- 测试程序在生产服务器（`<server-ip>`）上跑，对外通过 `https://api.vpanel.cc`，**与外部用户体验完全一致的链路**

**非目标**：
- 不做"软压测"——不限主动加并发避开 rate limit
- 不做对比测试（如不同代理供应商）
- 不做长时连续负载（>1h），单次测试 ≤25 分钟

## 2. 总体架构

独立 Go 单二进制 `cpa-stress`，源码托管在 `proxywatch` 仓库下的 `tools/cpa-stress/` 子目录。一次性运维工具，与 proxywatch 主功能解耦。

```
┌────────────────────────────────────────────────────────────┐
│            cpa-stress 测试程序                              │
│                                                            │
│  ┌──────────┐    ┌────────────────┐                        │
│  │Profile   │───►│ Worker pool    │                        │
│  │Driver    │    │ (C 个 goroutine│                        │
│  │(stair-   │    │  并发触发请求) │                        │
│  │ step)    │    └───────┬────────┘                        │
│  └──────────┘            │                                 │
│                          ▼                                 │
│  ┌──────────┐    ┌────────────────┐                        │
│  │exit-IP   │───►│  Each request: │──┐                     │
│  │sampler   │    │  POST CPA →    │  │                     │
│  │(每 1s    │    │  collect resp  │  │                     │
│  │ ipify)   │    │  + tag exit IP │  │                     │
│  └──────────┘    └────────────────┘  │                     │
│                                       ▼                    │
│                              ┌──────────────┐              │
│                              │ JSONL writer │              │
│                              │ (每条请求一行│              │
│                              └──────┬───────┘              │
│                                     ▼                      │
│                              run-<ts>.jsonl 文件            │
│                              (实时落盘，崩了不丢)          │
│                                                            │
│  退出时：                                                  │
│  ┌──────────────────────────────────────┐                  │
│  │ Reporter: scan JSONL → build report  │                  │
│  └──────────────────────────────────────┘                  │
└────────────────────────────────────────────────────────────┘
                       │
                       ▼ HTTPS
              api.vpanel.cc/v1/chat/completions
                       │
                       ▼ (CPA round-robin)
              10 Codex OAuth × miyaIP SOCKS5
                       │
                       ▼
                api.openai.com
```

### 关键设计决策

1. **测试在服务器本地跑，但走公网** `https://api.vpanel.cc`（同机自反代）。原因：
   - 与真实外部客户端走完全一样的链路（nginx → CPA → SOCKS5 → upstream）
   - 服务器闲置内存够用（< 50 MB 占用），CPU 主要等 IO

2. **不进 proxywatch 主路径代码**——`tools/` 是工具子目录，独立 Go module（自己的 `go.mod`）。proxywatch 主二进制不依赖 cpa-stress。

3. **JSONL 实时落盘**——程序中途崩 / 网断 / kill 都还能回收已写部分，Reporter 直接 scan 即可。

## 3. 运行参数

### 3.1 并发剖面（stair-step ramp）

| Step | 并发 C | 持续 |
|------|--------|------|
| 0 | 1 | 3 min |
| 1 | 2 | 3 min |
| 2 | 4 | 3 min |
| 3 | 8 | 3 min |
| 4 | 16 | 3 min |
| 5 | 32 | 3 min |
| 6 | 64 | 3 min |

总最大时长 21 分钟（+ 4 分钟尾部缓冲 = 硬时间上限 25 分钟）。

每个 worker 是个 goroutine，只要前一条请求返回（成功或失败），立即从 task 队列取下一个并发出。**自然以 latency 调速**——没有人为节流。

### 3.2 模型轮换

每条请求按 round-robin 顺序选 `model`：

```
gpt-5.2 → gpt-5.4 → gpt-5.4-mini → gpt-5.5 → gpt-5.3-codex → (循环)
```

5 个模型负载近似均匀。

### 3.3 Prompt 池

20 个 task 变体，每条请求随机抽一个，组装成：

```
"Write a Python function that {task}. Include a brief docstring."
```

任务示例：
- reverses a string
- checks if a number is prime
- parses an ISO 8601 date
- merges two sorted lists
- counts word frequency in text
- ...

`max_tokens: 200` 控制单条响应大小。`temperature: 0.7`（避免完全确定性输出影响缓存）。

### 3.4 停止条件（OR 组合，任一触发即停）

| 条件 | 阈值 | 操作 |
|---|---|---|
| **完成全部 7 步** | C=64 步走完 | 正常结束 |
| **单步错误率超阈** | 一步内 ≥50% 请求失败（4xx + 5xx + transport 都算）| 立即停 + 标"边界步"|
| **连续无成功** | 连续 30 秒 0 成功响应 | 立即停（推断卡死或全员被 ban）|
| **总时间硬上限** | 25 分钟 | 兜底 |
| **手动 SIGINT (Ctrl+C)** | 任意时刻 | 优雅关闭 |

退出路径都一致：worker pool ctx cancel → 等在飞请求 ≤30s → close JSONL writer → run Reporter → 写 report.md → 进程退出。

### 3.5 出口 IP 抓取

旁路 sampler goroutine：每秒一次走 SOCKS5 调 `https://api.ipify.org`，记下 `(ts_ms, ip)`。

每条请求发出时，找最近一次 sample（取 ts ≤ 请求 ts 中最大那次）作为该请求的 `exit_ip` 标签，并记 `exit_ip_age_ms = req.ts - sample.ts`。

**精度约 1 秒**——高并发下同秒多请求会被打同一个 IP 标签，但 miyaIP 真实出口 IP 在那一秒内可能换。报告里**明确标注此精度限制**。

替代方案考虑过但放弃：
- 每请求前打一次 ipify：double 翻倍负载，且仍不保证两次走同一 SOCKS5 session（无法绑定）
- CPA 修改输出 reply 带 exit IP：太侵入，需要 fork CPA
- 读 CPA 日志：CPA 自己也不知道 exit IP（它只发 SOCKS5），日志里无此信息

## 4. 数据 schema

### 4.1 Per-request JSONL row

每条请求一行 JSON object，写到 `run-<ts>.jsonl`：

```json
{
  "ts_ms": 1778420017000,
  "step": 3,
  "concurrency": 8,
  "worker_id": 5,
  "model": "gpt-5.4-mini",
  "prompt": "Write a Python function that reverses a string. Include a brief docstring.",
  "response": {
    "id": "resp_...",
    "content": "```python\\ndef reverse_string(s):\\n  ...\\n```",
    "finish_reason": "stop"
  },
  "http_code": 200,
  "latency_ms": 4429,
  "in_tokens": 41,
  "out_tokens": 163,
  "total_tokens": 204,
  "exit_ip": "104.175.205.241",
  "exit_ip_age_ms": 320,
  "error": ""
}
```

字段说明：
- `ts_ms`：请求发出时刻（worker 写出第一字节前）
- `step`、`concurrency`、`worker_id`：可追溯 step / 在哪个 worker 上发
- `prompt`：完整 user message 文本（无 truncation）
- `response`：成功时填，失败时为 null。`content` 截到 4 KB 防止暴涨（多数 200-token 输出 ≤2 KB）
- `http_code`：实际 HTTP 状态码；transport 错误时为 0
- `latency_ms`：从 worker 写第一字节到读完响应 body 的总时长
- `in_tokens`、`out_tokens`、`total_tokens`：从响应 `usage` 字段抽
- `exit_ip`、`exit_ip_age_ms`：旁路 sampler 给出
- `error`：transport / parse 错误信息；HTTP 4xx/5xx 时此字段为空（但 `http_code` 非 200）

### 4.2 报告 Markdown

测试结束时写到 `cpa-stress-report-<ts>/report.md`，附原始 `run-<ts>.jsonl`：

```markdown
# CPA Stress Test Report — 2026-05-10 21:30:00

## Summary
- Total duration: 18m 32s
- Total requests: 4203
- Stopped reason: error_rate_exceeded at step 5 (C=32)
- Boundary identified: between C=16 (98% success) and C=32 (44% success)

## Per-step
| Step | C | Duration | Reqs | OK | 4xx | 5xx | err | RPS | p50 ms | p95 ms | tok in/out avg |
| ... |

## Per-model
| Model | Reqs | OK | 4xx | Avg latency | tok in/out avg |
| ... |

## Exit IP histogram
| Exit IP | Reqs | OK | 4xx | First seen step | Last seen step |
| ... |

## Errors detail
| Code | Count | Sample message |
| ... |

## Token usage estimate
- Total input tokens: ...
- Total output tokens: ...

## Caveats
- exit_ip 标签精度 ≈ 1 秒，高并发下同秒内可能跨多个真实 exit IP
- 测试期间使用了真实 ChatGPT Plus 订阅资源，可能影响日常使用直到 quota 重置
- ...

## Raw data
- run-<ts>.jsonl ({N} lines, {S} MB)
```

## 5. 错误处理与边界

| 场景 | 处理 |
|---|---|
| 请求 timeout（默认 60s）| 记 `error="timeout"`, `http_code=0`，计入失败 |
| Transport error（连不上 nginx / TLS 失败 / EOF）| 记 `error=<msg>`, `http_code=0` |
| HTTP 4xx | 写完整响应（含错误 message），计入失败 |
| HTTP 5xx | 同 4xx |
| ipify sampler 临时 fail | 跳过该秒；下条请求用前一秒的 sample；连续 5s 失败把 `exit_ip` 留空（"unknown"）|
| JSONL writer 写盘失败 | 不重试，stderr 警告，继续测试（不阻塞）|
| Worker panic | 记日志，退出该 worker；其他 worker 不受影响 |
| 主进程 SIGINT | ctx cancel → 等 ≤30s → reporter → 退出 |
| 时间硬上限 | 同 SIGINT |
| 单步 50% 错误率门限 | 该步走完后立刻 evaluate（不在中途打断该步），触发即标记并退 |

## 6. 测试策略

主程序逻辑会有一些有限的单元测试，但这是**运维工具**而非生产代码，重点放在端到端 dry-run：

### 单元
- Profile driver 状态推进逻辑（mock 时钟）
- 停止条件检测（构造 step 结果 → 期望 stopped_reason）
- JSONL row 序列化往返

### Dry-run
跑一个**短缩版**（每步 30s 而非 3 分钟，只到 C=4），实测以下：
- JSONL 真的实时写入
- exit-IP sampler 真的能拿到 IP
- 中途 Ctrl+C 优雅退出 + 仍出 report
- Reporter 生成的 markdown 表格无明显错乱

### 真测前
- 跑过 dry-run（≤2 分钟，约 60-100 个请求，烧掉很少 token）
- 检查 report.md 看着像样
- 跟用户确认 → 跑真版

## 7. 安全 / 风险

### 用户已确认接受的风险
- 测试期间触发的 OpenAI 账户级 rate limit 可能影响日常使用 1-24 小时
- miyaIP 出口 IP 可能被 OpenAI 拉黑（需等池轮换或后台手换）
- 极端情况账户被挂起或销号
- token quota / 月度 budget 被消耗

### 实施层面的安全
- API key (`5d378...`) 通过 CLI flag 传入，**不**编进二进制源码
- 仓库 `proxywatch` 是 public：cpa-stress 源码同样 public 但不含任何凭据
- 测试报告（含 prompts、responses、exit IPs）默认**不**提交到 git。`.gitignore` 排除 `tools/cpa-stress/run-*.jsonl` 和 `tools/cpa-stress/cpa-stress-report-*/`
- 测试报告完成后从服务器手动拉取（scp）回本地，由用户决定怎么保存

## 8. 部署与运行

### 构建

```bash
# 在服务器上
cd /opt/proxywatch-src
git pull
cd tools/cpa-stress
go build -o /opt/cpa-stress ./
```

### 运行

```bash
/opt/cpa-stress \
  -api-key 5d378b3ca96097d0e0a31b76965fc04bed3da0bc0de66d58 \
  -base-url https://api.vpanel.cc \
  -output-dir /tmp/cpa-stress-out
```

参数：
- `-api-key`（必填）：CPA 客户端 API key
- `-base-url`（默认 `https://api.vpanel.cc`）：CPA 入口
- `-output-dir`（默认当前目录）：报告 + JSONL 输出位置
- `-dry-run`（默认 false）：跑短版（每步 30s，到 C=4 即止）
- `-step-duration`（默认 `3m`）：自定义每步时长（dry-run 用）

### 取报告

```bash
# 本地
scp root@<server-ip>:/tmp/cpa-stress-out/cpa-stress-report-*/report.md ./
scp root@<server-ip>:/tmp/cpa-stress-out/cpa-stress-report-*/run-*.jsonl ./
```

## 9. 范围之外（明确排除）

- 不包装压测为 systemd service / docker container（一次性手动跑）
- 不做实时 dashboard（命令行偶尔打印 step 进度足够）
- 不做对比测试（不同代理 / 不同账号配置 等）
- 不自动判定"账号是否被销号"（这要求另外接 OpenAI 账户管理 API）
- 不做 token 美元成本核算（账户类型不同价格不一）
