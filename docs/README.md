# API文档自动生成

本项目使用 [swaggo/swag](https://github.com/swaggo/swag) 自动生成Swagger API文档。

## 📖 查看文档

启动服务后，访问以下URL查看交互式API文档:

```
http://localhost:8000/swagger/index.html
```

## 🛠️ 生成文档

### 使用Makefile (推荐)

```bash
# 生成Swagger文档
make docs

# 开发模式 (生成文档并运行服务)
make dev
```

### 手动生成

```bash
# 1. 安装swag工具 (首次使用)
go install github.com/swaggo/swag/cmd/swag@latest

# 2. 生成文档
swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal

# 3. 更新vendor (如果使用vendor)
go mod vendor
```

## ✍️ 编写API注释

### 主程序注释 (cmd/server/main.go)

```go
// @title Web Monitor API
// @version 2.0
// @description 轻量级系统监控API服务

// @contact.name API Support
// @contact.url https://github.com/AnalyseDeCircuit/web-monitor
// @contact.email support@example.com

// @license.name CC BY-NC 4.0
// @license.url https://creativecommons.org/licenses/by-nc/4.0/

// @host localhost:8000
// @BasePath /

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

// @securityDefinitions.apikey CookieAuth
// @in cookie
// @name auth_token
```

### Handler注释示例

```go
// LoginHandler 处理登录请求
// @Summary 用户登录
// @Description 使用用户名和密码登录，成功后返回JWT token
// @Tags Authentication
// @Accept json
// @Produce json
// @Param credentials body types.LoginRequest true "登录凭证"
// @Success 200 {object} types.LoginResponse "登录成功"
// @Failure 400 {object} map[string]string "请求格式错误"
// @Failure 401 {object} map[string]string "用户名或密码错误"
// @Router /api/login [post]
func LoginHandler(w http.ResponseWriter, r *http.Request) {
    // ... 实现代码
}
```

### 注释语法说明

| 注释 | 说明 | 示例 |
|------|------|------|
| `@Summary` | 简短摘要 | `@Summary 用户登录` |
| `@Description` | 详细描述 | `@Description 使用用户名和密码登录` |
| `@Tags` | API分组标签 | `@Tags Authentication` |
| `@Accept` | 接受的Content-Type | `@Accept json` |
| `@Produce` | 返回的Content-Type | `@Produce json` |
| `@Param` | 请求参数 | `@Param id path int true "用户ID"` |
| `@Success` | 成功响应 | `@Success 200 {object} User "成功"` |
| `@Failure` | 失败响应 | `@Failure 400 {object} Error "错误"` |
| `@Router` | 路由定义 | `@Router /api/users [get]` |
| `@Security` | 安全认证 | `@Security BearerAuth` |

## 🎯 常用命令

```bash
# 查看所有Makefile命令
make help

# 生成并查看文档
make docs
# 然后访问 http://localhost:8000/swagger/index.html

# 格式化Swagger注释
make docs-fmt

# 完整构建 (清理+文档+编译)
make all

# 运行项目
make run

# 开发模式 (自动生成文档并运行)
make dev
```

## 📋 API分组

当前项目的API按以下标签分组:

- **Authentication** - 用户认证相关接口
- **Monitoring** - 系统监控数据接口
- **Docker** - Docker容器管理接口 (需要admin权限)
- **Systemd** - Systemd服务管理接口 (需要admin权限)
- **Users** - 用户管理接口 (需要admin权限)
- **Cron** - Cron任务管理接口 (需要admin权限)
- **WebSocket** - WebSocket实时推送接口

## 🔒 认证方式

API支持两种认证方式:

### 1. Bearer Token (Header)
```http
Authorization: Bearer <jwt_token>
```

### 2. Cookie (推荐)
```http
Cookie: auth_token=<jwt_token>
```

后端优先使用HttpOnly Cookie进行认证，更安全。

## 📦 依赖

```go
require (
    github.com/swaggo/swag v1.16.6
    github.com/swaggo/http-swagger/v2 v2.0.2
    github.com/swaggo/files/v2 v2.0.2
)
```

## 🔄 工作流程

1. 在handler函数上方添加Swagger注释
2. 运行 `make docs` 生成文档
3. 启动服务 `make run`
4. 访问 `http://localhost:8000/swagger/index.html`
5. 在Swagger UI中测试API

## 🎨 Swagger UI特性

- ✅ 交互式API测试
- ✅ 请求/响应示例
- ✅ 模型定义查看
- ✅ 认证配置
- ✅ Try it out功能
- ✅ 响应格式化显示
- ✅ API分组展示

## 📝 注意事项

1. **修改注释后必须重新生成文档** - 运行 `make docs`
2. **类型定义需在pkg/types中** - Swagger会自动解析
3. **复杂类型使用object内联定义** - 如 `@Param request body object{username=string,password=string} true "请求体"`
4. **Vendor模式** - 使用 `GOFLAGS="-mod=mod"` 生成文档
5. **文档目录不要手动修改** - docs/下的文件由swag自动生成

## 🚀 最佳实践

1. **保持注释与代码同步** - 修改handler时同步更新注释
2. **使用有意义的Summary** - 简短明确的功能描述
3. **完整的错误码说明** - 列出所有可能的错误响应
4. **请求参数详细说明** - 包含类型、是否必需、示例值
5. **分组合理** - 使用Tags组织相关API
6. **安全定义清晰** - 明确哪些API需要认证

## 📚 相关链接

- [Swag官方文档](https://github.com/swaggo/swag)
- [Swagger规范](https://swagger.io/specification/)
- [API示例](http://localhost:8000/swagger/index.html)
