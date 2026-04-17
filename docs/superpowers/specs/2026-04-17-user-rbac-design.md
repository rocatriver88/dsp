# User System + RBAC Design Spec

**Date:** 2026-04-17
**Branch:** feat-user-rbac
**Status:** APPROVED
**Review history:** office-hours design → Claude spec review → Codex cold read → eng review → Codex outside voice (4 rounds, 12 issues addressed)

---

## Problem

DSP 平台准备商用。当前认证：API Key（广告主）+ ADMIN_TOKEN 环境变量（管理员）。无用户概念，无密码登录，无审计追踪到人。已有广告主等着从竞品切过来。

## Constraints

- 一个广告主 = 一个用户（MVP）
- 两个角色：platform_admin、advertiser
- 广告主账号由管理员创建（无自助注册）
- API Key 保留给程序化接入
- ADMIN_TOKEN 保留给服务间调用（bidder → API）

## Architecture

### 三层认证并存

```
请求进入
  │
  ├─ Tenant 路由 (/api/v1/campaigns, /billing, /reports, etc.)
  │   ├── Authorization: Bearer <JWT> (role=advertiser)
  │   │   → 验证 JWT → WithAdvertiser(ctx, {ID: claims.aid})
  │   │                 + WithUser(ctx, {ID: claims.uid, ...})
  │   ├── X-API-Key: <key> (程序化接入，保持不变)
  │   │   → DB lookup → WithAdvertiser(ctx, {ID, Name, Email})
  │   │                  UserFromContext = nil
  │   ├── JWT role=platform_admin → 403 (admin 不能访问 tenant 路由)
  │   └── 无凭证 → 401
  │
  ├─ Admin 路由 (/api/v1/admin/*, /internal/*)
  │   ├── Authorization: Bearer <JWT> (role=platform_admin)
  │   │   → 验证 JWT → WithUser(ctx, {Role: platform_admin})
  │   ├── X-Admin-Token (服务间调用，bidder/consumer)
  │   │   → 与 ADMIN_TOKEN env 比对 → 通过
  │   └── 其他 → 401
  │
  └─ 公开路由 (免认证)
      ├── POST /api/v1/auth/login
      ├── POST /api/v1/auth/refresh
      ├── GET /health, /health/live, /health/ready
      ├── /uploads/*
      └── POST /api/v1/register (保留但前端不暴露)
```

### Data Model

```sql
CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    email           TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    name            TEXT NOT NULL,
    role            TEXT NOT NULL CHECK (role IN ('platform_admin', 'advertiser')),
    advertiser_id   BIGINT REFERENCES advertisers(id),
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    refresh_token_hash TEXT,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- advertiser 必须关联 advertiser_id, platform_admin 不关联
ALTER TABLE users ADD CONSTRAINT users_role_advertiser_check
    CHECK ((role = 'advertiser' AND advertiser_id IS NOT NULL) OR
           (role = 'platform_admin' AND advertiser_id IS NULL));

-- audit_log 加 user_id 列
ALTER TABLE audit_log ADD COLUMN user_id BIGINT REFERENCES users(id);
```

### JWT Lifecycle

- **Access Token:** 15 min, HS256, claims: uid, email, role, aid
- **Refresh Token:** 7 days, hash 存 users.refresh_token_hash
- **刷新时检查:** user.status != 'suspended'
- **停用用户:** 清 refresh_token_hash → 15 min 内强制下线
- **JWTSecret:** 独立环境变量，不复用 APIHMACSecret/BidderHMACSecret

### Context Bridging (向后兼容)

**关键设计：20+ 个现有 handler 零改动。**

- JWT advertiser 路径：注入 AdvertiserFromContext + UserFromContext
- API Key 路径：只注入 AdvertiserFromContext（UserFromContext = nil）
- 所有现有 handler 继续用 `auth.AdvertiserIDFromContext(ctx)`
- 新 handler 可以额外用 `auth.UserFromContext(ctx)`

### Admin 中间件

```go
// AdminAuthMiddleware 同时接受 JWT 和 X-Admin-Token
func AdminAuthMiddleware(jwtSecret []byte, adminToken string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 1. 试 JWT
            if bearer := extractBearer(r); bearer != "" {
                claims, err := ValidateJWT(bearer, jwtSecret)
                if err == nil && claims.Role == "platform_admin" {
                    ctx := WithUser(r.Context(), &User{...})
                    next.ServeHTTP(w, r.WithContext(ctx))
                    return
                }
            }
            // 2. 试 X-Admin-Token (服务间调用)
            if r.Header.Get("X-Admin-Token") == adminToken && adminToken != "" {
                next.ServeHTTP(w, r)
                return
            }
            // 3. 都没有
            WriteError(w, 401, "authentication required")
        })
    }
}
```

## API Changes

### New Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | /api/v1/auth/login | none | 邮箱+密码 → access + refresh token |
| POST | /api/v1/auth/refresh | refresh token | 刷新 access token |
| GET | /api/v1/auth/me | JWT | 当前用户信息 |
| POST | /api/v1/auth/change-password | JWT | 修改密码 |
| GET | /api/v1/admin/users | admin | 列出所有用户 |
| POST | /api/v1/admin/users | admin | 创建用户（含创建广告主） |
| PUT | /api/v1/admin/users/{id} | admin | 修改用户 |

### Unchanged

- 所有 /api/v1/campaigns, /creatives, /billing, /reports 端点
- API Key 认证
- POST /api/v1/admin/topup（已有）
- POST /api/v1/register（保留不暴露）
- X-Admin-Token 服务间认证

## File Changes

### New Files
| File | Purpose |
|------|---------|
| `migrations/011_users.sql` | users 表 + audit_log.user_id |
| `internal/user/store.go` | User CRUD |
| `internal/user/model.go` | User struct + UserResponse DTO |
| `internal/auth/password.go` | bcrypt hash/verify |
| `internal/auth/jwt.go` | JWT 签发/验证/Claims |
| `internal/auth/jwt_middleware.go` | JWT 中间件 + API Key fallback |
| `internal/handler/auth_handlers.go` | login, refresh, me, change-password |
| `internal/handler/user_handlers.go` | admin 用户管理 |
| `web/app/admin/users/page.tsx` | 用户管理页面 |

### Modified Files
| File | Change |
|------|--------|
| `internal/config/config.go` | +JWTSecret field, Validate() 检查 |
| `internal/handler/handler.go` | Deps +UserStore, +JWTSecret |
| `internal/handler/routes.go` | 新路由 + auth 豁免 |
| `internal/handler/admin_auth.go` | JWT + Token 双认证 |
| `internal/handler/middleware.go` | WithAuthExemption 加 /auth/login, /auth/refresh |
| `cmd/api/main.go` | 初始化 UserStore, JWT |
| `web/app/_components/ApiKeyGate.tsx` | 邮箱+密码登录表单 |
| `web/app/admin/layout.tsx` | JWT admin 认证 |
| `web/lib/api.ts` | login/refresh/me API + 401 自动 refresh |
| `web/lib/admin-api.ts` | JWT Bearer auth |

## Security

- bcrypt cost 10
- JWT HS256, JWTSecret >= 32 bytes (production validated)
- 登录锁定: 5 次失败 → 15 min 锁（Redis, fail-open if unavailable）
- CSRF immune: Bearer header, no cookies
- 种子 admin: ADMIN_EMAIL + ADMIN_INITIAL_PASSWORD env vars
- 审计: admin 操作写 audit_log, actor = user:<id> 或 apikey:<advertiser_id>

## Admin Workflow

```
管理员登录 → 管理后台 → 创建广告主
  → 输入：公司名、联系邮箱、初始密码
  → 系统创建：advertisers 行 + users 行 + 生成 API Key
  → 返回：登录信息 + API Key（一次性展示）

管理员充值：
  → POST /api/v1/admin/topup (已有端点)
  → body: { advertiser_id, amount_cents, description }
  → 系统：更新 balance_cents + 记录 transaction + 写 audit_log
```

## Frontend Token Refresh

```typescript
// web/lib/api.ts — request 函数
async function request<T>(path: string, options?: RequestInit): Promise<T> {
  let res = await fetch(url, { headers: { Authorization: `Bearer ${accessToken}` }, ...options });
  if (res.status === 401 && refreshToken) {
    const refreshRes = await fetch('/api/v1/auth/refresh', { body: refreshToken });
    if (refreshRes.ok) {
      accessToken = refreshRes.json().access_token;
      res = await fetch(url, { headers: { Authorization: `Bearer ${accessToken}` }, ...options });
    } else {
      logout(); // refresh 也失败，跳回登录页
    }
  }
  return res.json();
}
```

## Test Coverage (28+ paths)

All auth code requires 100% test coverage.

**Unit tests (24 paths):**
- JWT: issue/validate/expired/wrong-key/malformed (5)
- Password: hash/check-correct/check-wrong (3)
- Middleware: JWT-advertiser/JWT-admin/API-Key-fallback/expired-fallthrough/admin-on-tenant-403/both-headers (6)
- Login: valid/wrong-password/unknown-email/suspended/lockout (5)
- Refresh: valid/expired/suspended (3)
- User CRUD: create-advertiser/create-admin/duplicate-email (3)

**E2E tests (4 paths):**
- Frontend login form → dashboard
- Admin login → admin dashboard
- Token refresh on 401
- Logout clears tokens

## NOT in scope

- 多用户/多角色 per advertiser（MVP 一人一账号）
- 忘记密码（管理员手动重置）
- 管理员代操作 tenant 路由
- API Key 哈希存储（记为 TODO）
- sessions 表替代 refresh_token_hash（记为 TODO）
- 前端注册页面（管理员创建）

## Dependencies

- `golang.org/x/crypto/bcrypt`
- `github.com/golang-jwt/jwt/v5`
