# ShieldCaptcha v2 配置参考

## 环境变量

### 核心配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `CAPTCHA_HMAC_SECRET` | 自动生成(仅开发) | HMAC 签名密钥（hex 编码，32字节） |
| `CAPTCHA_PORT` | `8080` | 服务监听端口 |
| `CAPTCHA_BASE_DIFFICULTY` | `18` | 基础 PoW 难度（前导零比特数） |
| `CAPTCHA_MIN_DIFFICULTY` | `16` | 最小难度下限 |
| `CAPTCHA_MAX_DIFFICULTY` | `24` | 最大难度上限 |
| `CAPTCHA_CHALLENGE_TTL` | `120s` | Challenge 有效期 |
| `CAPTCHA_RATE_LIMIT` | `10` | 每秒请求限制（每IP） |
| `CAPTCHA_RATE_BURST` | `20` | 突发容量 |
| `CAPTCHA_NONCE_EXPIRY` | `300s` | 已用 Nonce 保留时间 |
| `CAPTCHA_LOG_LEVEL` | `info` | 日志级别 (debug/info/warn/error) |

### Redis 配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `CAPTCHA_ENABLE_REDIS` | `false` | 启用 Redis 存储 |
| `CAPTCHA_REDIS_URL` | `redis://localhost:6379/0` | Redis 连接地址 |
| `CAPTCHA_REDIS_PREFIX` | `sc:` | Redis Key 前缀 |

### PostgreSQL 配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `CAPTCHA_ENABLE_POSTGRES` | `false` | 启用 PostgreSQL |
| `CAPTCHA_POSTGRES_URL` | `postgres://captcha:captcha@localhost:5432/shieldcaptcha?sslmode=disable` | PostgreSQL 连接串 |
| `CAPTCHA_AUTO_MIGRATE` | `true` | 启动时自动运行数据库迁移 |

### 管理面板配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `CAPTCHA_ADMIN_ENABLED` | `false` | 启用管理面板 API |
| `CAPTCHA_ADMIN_USER` | `admin` | 初始管理员用户名（无PG时使用） |
| `CAPTCHA_ADMIN_PASS` | _(空)_ | 初始管理员密码 |
| `CAPTCHA_JWT_SECRET` | 同 HMAC_SECRET | JWT 签名密钥 |

### Feature Flags

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `CAPTCHA_FEATURE_RISK_ENGINE` | `false` | 启用风险评分引擎 |
| `CAPTCHA_FEATURE_BEHAVIOR` | `false` | 启用行为生物特征分析 |
| `CAPTCHA_FEATURE_ADAPTIVE_RATE` | `false` | 启用自适应速率限制 |
| `CAPTCHA_FEATURE_REPLAY_DETECT` | `false` | 启用轨迹重放检测 |

### 监控

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `CAPTCHA_METRICS_ENABLED` | `true` | 启用 Prometheus 指标导出（`/metrics`） |

## 降级策略

| 组件故障 | 行为 |
|---------|------|
| Redis 不可达 | 自动切换为内存存储（熔断器 5次失败后打开，10秒后半开尝试恢复） |
| PostgreSQL 不可达 | 跳过行为日志记录和规则加载，验证流程不受影响 |
| 风险引擎超时 | 5秒超时后放行（fail-open），记录告警日志 |

## Prometheus 指标

| 指标 | 类型 | 说明 |
|------|------|------|
| `shieldcaptcha_challenges_issued_total` | Counter | 发放的 Challenge 总数 |
| `shieldcaptcha_verifications_total{result}` | Counter | 验证结果（pass/fail/block） |
| `shieldcaptcha_verification_duration_seconds` | Histogram | 验证请求延迟 |
| `shieldcaptcha_risk_score` | Histogram | 风险评分分布 |
| `shieldcaptcha_active_sessions` | Gauge | 活跃会话数 |
| `shieldcaptcha_rules_loaded` | Gauge | 当前加载规则数 |
| `shieldcaptcha_rate_limit_hits_total{dimension}` | Counter | 速率限制命中 |
| `shieldcaptcha_http_requests_total{method,path,status}` | Counter | HTTP 请求计数 |
| `shieldcaptcha_http_request_duration_seconds{method,path}` | Histogram | HTTP 请求延迟 |

## 告警建议

```yaml
# Prometheus alerting rules
groups:
  - name: shieldcaptcha
    rules:
      - alert: HighBlockRate
        expr: rate(shieldcaptcha_verifications_total{result="block"}[5m]) / rate(shieldcaptcha_verifications_total[5m]) > 0.3
        for: 5m
        annotations:
          summary: "拦截率超过30%，可能存在误杀"

      - alert: HighLatency
        expr: histogram_quantile(0.99, rate(shieldcaptcha_verification_duration_seconds_bucket[5m])) > 0.5
        for: 2m
        annotations:
          summary: "验证P99延迟超过500ms"

      - alert: RedisDown
        expr: up{job="shieldcaptcha"} == 1 and shieldcaptcha_redis_ops_total == 0
        for: 1m
        annotations:
          summary: "Redis 连接断开，已降级为内存模式"
```
