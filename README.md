# ShieldCaptcha

可自托管的轻量级验证服务，通过 **PoW 质询响应 + 浏览器指纹 + 交互行为验证** 三重机制区分真人与机器人。

## 验证流程

```
┌──────────┐         ┌──────────────┐
│  浏览器   │         │  ShieldCaptcha│
│  (前端)   │         │   (后端)      │
└────┬─────┘         └──────┬───────┘
     │  1. GET /api/challenge        │
     │──────────────────────────────>│
     │     返回签名质询(ID+Nonce+    │
     │     Difficulty+Expires+Sig)   │
     │<──────────────────────────────│
     │                               │
     │  2. 用户拖动滑块 (采集轨迹)    │
     │  3. 采集 Canvas/WebGL/字体/   │
     │     硬件指纹 → SHA-256 脱敏   │
     │  4. WebWorker 计算 PoW        │
     │     (找到使 SHA-256(ID:Nonce: │
     │      Solution) 前N位为0的解)  │
     │                               │
     │  5. POST /api/verify          │
     │     {challenge, solution,     │
     │      fingerprint, interaction}│
     │──────────────────────────────>│
     │                               │ 6. 验证签名完整性
     │                               │ 7. 检查是否过期
     │                               │ 8. 防重放(Nonce唯一性)
     │                               │ 9. 验证 PoW 解
     │                               │ 10. 验证交互合理性
     │     {success, token}          │
     │<──────────────────────────────│
     │                               │
```

## 安全特性

| 特性 | 实现方式 |
|------|----------|
| 签名防篡改 | HMAC-SHA256 对质询参数签名，后端验证完整性 |
| 过期机制 | 质询携带 `expires_at`，过期后拒绝验证 |
| 防重放 | 内存 Nonce Store，已使用的 Nonce 不可重用 |
| 动态难度 | 根据并发负载对数增长，限制在 [min, max] 区间 |
| 限流 | 令牌桶算法 per-IP 限流，防止暴力攻击 |
| 指纹脱敏 | 前端仅提交 SHA-256 哈希，不上传原始硬件信息 |
| 行为验证 | 检查交互时长、轨迹点数、是否有真实移动 |

## 快速开始

### Docker 一键运行

```bash
# 生成密钥
export CAPTCHA_HMAC_SECRET=$(openssl rand -hex 32)

# 启动服务
docker compose up -d

# 访问 http://localhost:8080
```

### 本地开发

```bash
# 安装依赖
go mod download

# 运行服务
go run ./cmd/server/

# 运行测试
go test ./... -v
```

## 配置项

通过环境变量配置，或使用 `.env` 文件（参考 `.env.example`）：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `CAPTCHA_HMAC_SECRET` | 自动生成(不安全) | HMAC 签名密钥 (hex编码, 32字节) |
| `CAPTCHA_PORT` | `8080` | 服务监听端口 |
| `CAPTCHA_BASE_DIFFICULTY` | `18` | 基础 PoW 难度 (前导零 bit 数) |
| `CAPTCHA_MIN_DIFFICULTY` | `16` | 最小难度 |
| `CAPTCHA_MAX_DIFFICULTY` | `24` | 最大难度 |
| `CAPTCHA_CHALLENGE_TTL` | `120s` | 质询有效期 |
| `CAPTCHA_RATE_LIMIT` | `10` | 每秒请求数限制 |
| `CAPTCHA_RATE_BURST` | `20` | 突发请求容量 |
| `CAPTCHA_NONCE_EXPIRY` | `300s` | Nonce 保留时间 |
| `CAPTCHA_LOG_LEVEL` | `info` | 日志级别 |

## API 参考

### GET /api/challenge

获取验证质询。

**响应：**
```json
{
  "id": "a1b2c3...",
  "nonce": "d4e5f6...",
  "difficulty": 18,
  "timestamp": 1718100000,
  "expires_at": 1718100120,
  "signature": "hmac-sha256-hex..."
}
```

### POST /api/verify

提交验证结果。

**请求体：**
```json
{
  "challenge": { "id": "...", "nonce": "...", "difficulty": 18, "timestamp": ..., "expires_at": ..., "signature": "..." },
  "solution": "hex-counter-value",
  "fingerprint": "sha256-hash-of-browser-fingerprint",
  "interaction": {
    "type": "drag",
    "start_time": 1718100001000,
    "end_time": 1718100002500,
    "trajectory": [[0,5],[15,6],[30,4],...]
  }
}
```

**成功响应：**
```json
{ "success": true, "token": "challenge-id", "timestamp": 1718100003 }
```

**失败响应：**
```json
{ "success": false, "error": "reason", "timestamp": 1718100003 }
```

### GET /health

健康检查端点，返回 `{"status":"ok"}`。

## 项目结构

```
├── cmd/server/main.go       # 入口，组装依赖并启动 HTTP 服务
├── internal/
│   ├── config/              # 环境变量配置加载
│   ├── challenge/           # 质询生成、签名、PoW验证、动态难度
│   ├── store/               # 内存 Nonce Store (防重放)
│   ├── ratelimit/           # 令牌桶 per-IP 限流
│   ├── handler/             # HTTP handler (质询下发/验证)
│   └── middleware/          # CORS、日志、panic恢复
├── web/
│   ├── index.html           # 验证演示页面
│   ├── captcha.js           # 指纹采集 + 交互捕获 + 提交逻辑
│   └── worker.js            # WebWorker PoW 计算 (纯JS SHA-256)
├── tests/e2e/               # 端到端集成测试
├── Dockerfile               # 多阶段构建
├── docker-compose.yml       # 一键部署
└── .env.example             # 配置模板
```

## 测试

```bash
# 全部测试
go test ./... -v

# 仅单元测试
go test ./internal/... -v

# 仅E2E测试
go test ./tests/e2e/ -v

# 竞态检测
go test ./... -race
```

## 难度说明

难度值表示 PoW 解的 SHA-256 哈希需要有多少个前导零 bit：
- **16 bit**: ~65K 次哈希，现代浏览器约 0.1-0.5 秒
- **18 bit**: ~260K 次哈希，约 0.5-2 秒
- **20 bit**: ~1M 次哈希，约 1-4 秒
- **24 bit**: ~16M 次哈希，约 5-20 秒

服务在高并发时自动提高难度以减缓请求洪泛。

## 集成指引

在你的应用中集成 ShieldCaptcha：

1. 前端在表单提交前加载验证组件
2. 验证通过后获取 `token`
3. 将 `token` 随表单提交到你的后端
4. 你的后端可通过 token 值（即 challenge ID）确认该请求已通过验证

## License

MIT
