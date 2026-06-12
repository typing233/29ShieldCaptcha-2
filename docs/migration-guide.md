# ShieldCaptcha v1 → v2 迁移指南

## 概述

v2 是 v1 的**超集**——所有 v1 接口和行为完全保留。新功能通过 Feature Flag 逐步开启，不影响现有客户端。

## 迁移步骤

### 第一步：准备基础设施

```bash
# 启动 Redis 和 PostgreSQL
docker compose -f deploy/docker-compose.yml up -d redis postgres
```

等待健康检查通过：
```bash
docker compose -f deploy/docker-compose.yml ps
```

### 第二步：部署新版本（Feature Flags 关闭）

```env
# .env - 初始部署，所有新功能关闭
CAPTCHA_ENABLE_REDIS=true
CAPTCHA_REDIS_URL=redis://redis:6379/0
CAPTCHA_ENABLE_POSTGRES=true
CAPTCHA_POSTGRES_URL=postgres://captcha:password@postgres:5432/shieldcaptcha?sslmode=disable
CAPTCHA_AUTO_MIGRATE=true

# 新功能全部关闭
CAPTCHA_FEATURE_RISK_ENGINE=false
CAPTCHA_FEATURE_BEHAVIOR=false
CAPTCHA_FEATURE_ADAPTIVE_RATE=false
CAPTCHA_FEATURE_REPLAY_DETECT=false
CAPTCHA_ADMIN_ENABLED=false
```

```bash
docker compose -f deploy/docker-compose.yml up -d shieldcaptcha
```

**验证**：`/api/challenge` 和 `/api/verify` 正常工作，`/health` 显示 Redis/Postgres 状态。

### 第三步：启用管理面板

```env
CAPTCHA_ADMIN_ENABLED=true
CAPTCHA_ADMIN_USER=admin
CAPTCHA_ADMIN_PASS=your-secure-password
CAPTCHA_JWT_SECRET=your-jwt-secret
```

重启服务后访问管理面板 API，创建管理员账户。

### 第四步：逐步开启风险引擎

建议顺序：

1. **行为采集** (`CAPTCHA_FEATURE_BEHAVIOR=true`)
   - 仅采集数据到 `verification_logs`，不影响验证结果
   - 观察 1-2 天后分析基线数据

2. **风险评分** (`CAPTCHA_FEATURE_RISK_ENGINE=true`)
   - 评分计算并记录，但初期只标记 `review`，不实际拦截
   - 通过管理面板观察评分分布，调整阈值

3. **重放检测** (`CAPTCHA_FEATURE_REPLAY_DETECT=true`)
   - 检测轨迹重放攻击

4. **自适应速率限制** (`CAPTCHA_FEATURE_ADAPTIVE_RATE=true`)
   - 根据风险评分动态调整限速

### 第五步：升级前端 SDK（可选）

旧版 `captcha.js` 仍然可用。新版 TypeScript SDK 提供：
- 行为生物特征采集（鼠标/触摸轨迹、加速度、停顿）
- 更好的 TypeScript 类型支持
- 主题定制

```html
<!-- 方式1：替换旧 captcha.js -->
<script src="/sdk/dist/shieldcaptcha.umd.js"></script>

<!-- 方式2：npm 安装 -->
<!-- npm install @shieldcaptcha/sdk -->
```

```javascript
import { render } from '@shieldcaptcha/sdk';
const captcha = render('#captcha-container', {
  apiBase: 'https://your-domain.com',
  enableBiometrics: true,
  theme: { primaryColor: '#1890ff', sliderShape: 'round' },
  onSuccess: (token) => { /* 提交 token */ },
});
```

## 回滚

如果遇到问题，可以随时关闭 Feature Flags 回退到 v1 行为：

```env
CAPTCHA_FEATURE_RISK_ENGINE=false
CAPTCHA_FEATURE_BEHAVIOR=false
CAPTCHA_FEATURE_ADAPTIVE_RATE=false
CAPTCHA_FEATURE_REPLAY_DETECT=false
```

重启即生效，无需回滚数据库。

## 数据库迁移

迁移在服务启动时自动执行（`CAPTCHA_AUTO_MIGRATE=true`）。

手动操作：
```bash
# 查看迁移状态
docker exec shieldcaptcha ./shieldcaptcha migrate status

# 回退最近一次迁移
docker exec shieldcaptcha ./shieldcaptcha migrate down 1
```

## API 兼容性

| 端点 | v1 | v2 | 变化 |
|------|----|----|------|
| `GET /api/challenge` | ✅ | ✅ | 无变化 |
| `POST /api/verify` | ✅ | ✅ | 新增可选 `behavior` 字段，响应新增可选 `risk_score`/`risk_reasons` |
| `GET /health` | ✅ | ✅ | 新增 Redis/Postgres 状态 |
| `GET /metrics` | ❌ | ✅ | 新增 Prometheus 指标 |
| `POST /admin/api/*` | ❌ | ✅ | 新增管理 API |

## 误杀回滚

如果出现误杀（合法用户被拦截）：

1. 管理面板 → 日志搜索 → 找到被误杀的 IP/指纹
2. 点击「解除封禁」或调用 `POST /admin/api/unblock`
3. 调整触发规则的阈值
4. 查看审计日志确认操作已记录
