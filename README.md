# 中华文化教学后端

基于 OpenAPI 描述通过 [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) 生成 Gin 服务端代码，配套配置、日志、数据库、JWT、迁移等基础设施的 Go 后端服务。

## 技术栈

| 组件 | 用途 |
| --- | --- |
| [Gin](https://github.com/gin-gonic/gin) | HTTP 路由与中间件 |
| [pgx/v5](https://github.com/jackc/pgx) | PostgreSQL 驱动与连接池 |
| [scany/v2](https://github.com/georgysavva/scany) | pgx 查询结果扫描 |
| [koanf/v2](https://github.com/knadh/koanf) | 配置加载（YAML + 环境变量） |
| [zap](https://github.com/uber-go/zap) | 结构化日志 |
| [golang-jwt/v4](https://github.com/golang-jwt/jwt) | JWT 签发与校验 |
| [golang-migrate](https://github.com/golang-migrate/migrate) | 数据库迁移 |
| [air](https://github.com/air-verse/air) | 本地热重载 |
| [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) | 由 OpenAPI 生成 ServerInterface |

## 目录结构

```
.
├── cmd/server/              # 程序入口
├── internal/
│   ├── api/                 # oapi-codegen 生成的代码（勿手动修改）
│   ├── config/              # 配置加载
│   ├── handler/             # api.ServerInterface 实现
│   ├── middleware/          # HTTP 中间件（JWT 鉴权等）
│   ├── model/               # 数据模型
│   ├── repository/          # 数据访问层（注：拼写与原目录保持一致）
│   └── service/             # 业务逻辑层
├── migrations/              # SQL 迁移文件
├── pkg/
│   ├── db/                  # pgx 连接池 + 迁移辅助
│   ├── httpresp/            # 统一响应封装
│   ├── jwt/                 # JWT 生成与解析
│   └── logger/              # zap 日志构造
├── config.example.yaml      # 配置模板
├── .air.toml                # air 热重载配置
├── oapi-codegen.yaml        # 代码生成配置
└── 默认模块.openapi.json     # OpenAPI 接口描述
```

## 快速开始

### 环境要求

- Go 1.26+
- PostgreSQL 12+
- （可选）[air](https://github.com/air-verse/air) 用于热重载、[migrate CLI](https://github.com/golang-migrate/migrate) 用于手动迁移

### 1. 克隆并安装依赖

```bash
git clone <repo-url>
cd zhonghuawenhua_backend
go mod download
```

### 2. 准备配置

```bash
cp config.example.yaml config.yaml
# 按需修改 config.yaml：数据库连接、JWT secret 等
```

配置也可通过环境变量覆盖，使用双下划线 `__` 作为分隔符，键名小写。例如：

```bash
export DATABASE__HOST=db.local
export DATABASE__PASSWORD=secret
export JWT__SECRET=your-very-long-secret
```

如需使用 `.env` 文件，启动时会自动加载（存在时）。

### 3. 准备数据库

```bash
createdb zhonghuawenhua
# 把迁移文件放入 migrations/ 目录，程序启动时会自动执行
```

### 4. 运行

```bash
# 直接运行
go run ./cmd/server

# 或使用 air 热重载（开发推荐）
air
```

默认监听 `0.0.0.0:8080`，可通过 `SERVER__HOST` / `SERVER__PORT` 覆盖。

## 配置项说明

| 配置项 | 环境变量 | 说明 |
| --- | --- | --- |
| `server.host` | `SERVER__HOST` | 监听地址 |
| `server.port` | `SERVER__PORT` | 监听端口 |
| `database.host` | `DATABASE__HOST` | 数据库主机 |
| `database.port` | `DATABASE__PORT` | 数据库端口 |
| `database.user` | `DATABASE__USER` | 数据库用户 |
| `database.password` | `DATABASE__PASSWORD` | 数据库密码 |
| `database.name` | `DATABASE__NAME` | 数据库名 |
| `database.ssl_mode` | `DATABASE__SSL_MODE` | SSL 模式 |
| `database.max_conns` | `DATABASE__MAX_CONNS` | 最大连接数（默认 10） |
| `jwt.secret` | `JWT__SECRET` | JWT 签名密钥 |
| `jwt.expire_hours` | `JWT__EXPIRE_HOURS` | Token 过期小时数 |
| `log.level` | `LOG__LEVEL` | 日志级别：debug/info/warn/error |
| `log.encoding` | `LOG__ENCODING` | 日志编码：json/console |

可通过 `CONFIG_PATH` 环境变量指定配置文件路径，默认 `./config.yaml`。

## 鉴权

- 使用 JWT Bearer Token，请求头 `Authorization: Bearer <token>`
- JWT 中间件通过路由白名单放行公开接口，目前包括：
  - `POST /api/v1/accounts:login`
  - `GET /api/v1/accounts:status`
- 其余接口均需携带有效 Token，校验失败返回 `Base401Resp`
- 解析成功后 `user_id` 写入 `gin.Context`，业务层通过 `c.Get("user_id")` 读取

## 响应格式

所有接口统一返回 `Base` 响应结构（详见 `默认模块.openapi.json`）：

| HTTP 状态 | 结构 | 字段 |
| --- | --- | --- |
| 200 | `Base200Resp` | `code`(200) / `msg` / `data` |
| 400 | `Base400Resp` | `code`(4xx) / `tip` / `msg` / `data` |
| 401 | `Base401Resp` | `code`(401) / `tip`("") / `msg` / `data` |
| 500 | `Base500Resp` | `code`(5xx) / `tip` / `msg` / `data` |

`tip` 为前端展示的错误提示，`msg` 供内部日志使用。

## 代码生成

接口定义在 `默认模块.openapi.json`，生成配置见 `oapi-codegen.yaml`。修改接口后重新生成：

```bash
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config oapi-codegen.yaml 默认模块.openapi.json
```

生成产物 `internal/api/api.gen.go` **不要手动修改**，业务实现写在 `internal/handler` 中。

## 数据库迁移

- 迁移文件放入 `migrations/` 目录，命名遵循 golang-migrate 约定：`{N}_{name}.up.sql` / `{N}_{name}.down.sql`
- 程序启动时自动执行 `m.Up()`，无变更时忽略 `ErrNoChange`
- 也可使用 migrate CLI 手动操作：

```bash
migrate -path migrations -database "postgres://user:pass@host:port/dbname?sslmode=disable" up
```

## 开发约定

- 提交信息遵循 `<类型>[模块]: <描述>` 格式，类型见项目 Git 规范
- 业务代码分层：`handler`（HTTP）→ `service`（业务）→ `repository`（数据访问）→ `model`（结构体）
- 一个文件不超过 1000 行；同样逻辑立即抽取成函数；库已实现的功能直接使用
