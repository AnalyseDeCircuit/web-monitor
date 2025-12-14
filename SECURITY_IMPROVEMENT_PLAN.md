# Web-Monitor 安全改进计划

## 概述
本文件记录了web-monitor项目的安全改进计划，旨在增强系统安全性同时保持现有功能完整性。

## 改进目标
1. 升级会话管理机制（SHA256自定义令牌 → JWT标准令牌）
2. 增强密码策略（复杂度要求、账户锁定）
3. 完善API安全头（CSP、严格CORS、SameSite cookie）
4. 强化输入验证（防止命令注入攻击）
5. 添加HTTPS/WSS支持（可选）

## 实施原则
- **不破坏现有功能**：所有监控、管理功能保持原样
- **API兼容性**：保持现有API接口和响应格式不变
- **渐进实施**：分阶段进行，每阶段完成后充分测试

## 详细实施步骤

### 第一阶段：会话管理与认证安全（预计2-3天）

#### 1.1 JWT会话管理升级
**目标**：用标准JWT替换自定义SHA256令牌

**具体任务**：
1. **添加JWT依赖**：修改`go.mod`，添加`github.com/golang-jwt/jwt/v5`
2. **创建JWT工具函数**：
   - `initJWT()`：初始化JWT密钥（环境变量 > 文件 > 随机生成）
   - `generateJWT(username, role)`：生成HS256签名令牌，24小时有效期
   - `validateJWT(tokenString)`：验证令牌签名和过期时间
   - `extractToken(r *http.Request)`：从请求中提取令牌（支持Authorization头、Cookie、查询参数）
3. **修改登录处理**：
   - 在`loginHandler`中用`generateJWT`替换SHA256令牌生成
   - 移除`sessions`内存存储和相关锁机制
4. **修改会话验证**：
   - 更新`getSessionInfo`使用JWT验证
   - 更新`isAuthenticated`和`getCurrentRole`函数
5. **保持API兼容**：登录响应格式保持不变
   ```json
   {
     "token": "jwt_token_here",
     "message": "Login successful",
     "username": "admin",
     "role": "admin"
   }
   ```

#### 1.2 密码策略增强
**目标**：添加密码复杂度要求和账户锁定机制

**具体任务**：
1. **密码复杂度验证函数**：`validatePasswordPolicy(password)`
   - 最少8个字符
   - 必须包含大小写字母和数字
   - 禁止常见弱密码（"password", "123456", "admin"等）
2. **账户锁定机制**：
   - `checkAccountLock(username)`：检查账户是否被锁定
   - `recordFailedLogin(username)`：记录失败尝试，5次后锁定15分钟
   - `recordSuccessfulLogin(username)`：重置失败计数
3. **集成到用户管理**：
   - 在`createUserHandler`中应用密码策略
   - 在`changePasswordHandler`中应用密码策略
   - 在`loginHandler`中检查账户锁定状态

#### 1.3 API安全头完整化
**目标**：添加缺失的安全HTTP头

**具体任务**：
1. **增强`securityHeadersMiddleware`**：
   ```go
   // 添加严格Content-Security-Policy
   csp := []string{
     "default-src 'self'",
     "script-src 'self' 'unsafe-inline'", // 现有前端需要内联脚本
     "style-src 'self' 'unsafe-inline'",  // 现有前端需要内联样式
     "img-src 'self' data:",
     "font-src 'self'",
     "connect-src 'self' ws://* wss://*", // 允许WebSocket连接
     "frame-ancestors 'none'",
     "base-uri 'self'",
     "form-action 'self'",
   }
   w.Header().Set("Content-Security-Policy", strings.Join(csp, "; "))
   ```
2. **安全Cookie设置**：
   ```go
   http.SetCookie(w, &http.Cookie{
     Name:     "auth_token",
     Value:    tokenStr,
     Path:     "/",
     HttpOnly: true,
     Secure:   true, // 仅在HTTPS下传输
     SameSite: http.SameSiteStrictMode,
     MaxAge:   86400, // 24小时
   })
   ```
3. **WebSocket CORS限制**：
   ```go
   var upgrader = websocket.Upgrader{
     CheckOrigin: func(r *http.Request) bool {
       origin := r.Header.Get("Origin")
       allowedOrigins := []string{
         "http://localhost:38080",
         "http://127.0.0.1:38080",
         // 添加实际生产域名
       }
       for _, allowed := range allowedOrigins {
         if origin == allowed {
           return true
         }
       }
       log.Printf("WebSocket connection from disallowed origin: %s", origin)
       return false // 生产环境返回false
     },
   }
   ```

### 第二阶段：输入验证与网络安全（预计1-2天）

#### 2.1 输入验证强化
**目标**：防止网络诊断工具的命令注入攻击

**具体任务**：
1. **创建严格验证函数**：`isValidTarget(target string) bool`
   ```go
   // 使用正则表达式验证：
   // 1. 域名：^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$
   // 2. IPv4：^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$
   // 3. IPv6：^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$
   // 额外检查：长度≤255，不含控制字符
   ```
2. **修改`networkTestHandler`**：
   - 用`isValidTarget`替换简单的字符白名单验证
   - 添加命令执行超时：`context.WithTimeout(context.Background(), 10*time.Second)`
3. **安全命令执行**：
   - 保持现有的参数化调用（已安全）
   - 添加资源限制和错误处理

#### 2.2 HTTPS/WSS支持（可选）
**目标**：添加TLS支持，启用安全传输

**具体任务**：
1. **证书管理脚本**：`generate_certs.sh`
   ```bash
   # 生成自签名证书（开发环境）
   openssl req -x509 -newkey rsa:4096 -keyout certs/key.pem -out certs/cert.pem \
     -days 365 -nodes -subj "/C=US/ST=State/L=City/O=Organization/CN=localhost"
   ```
2. **服务器修改**：
   ```go
   func startServer() error {
     certFile := os.Getenv("SSL_CERT_FILE")
     keyFile := os.Getenv("SSL_KEY_FILE")
     
     if certFile != "" && keyFile != "" {
       return http.ListenAndServeTLS(":"+port, certFile, keyFile, nil)
     } else {
       return http.ListenAndServe(":"+port, nil)
     }
   }
   ```
3. **Docker配置更新**：
   ```yaml
   environment:
     - SSL_CERT_FILE=/certs/cert.pem
     - SSL_KEY_FILE=/certs/key.pem
     - JWT_SECRET=${JWT_SECRET:-}
   volumes:
     - ./certs:/certs:ro
   ports:
     - "38080:38080"
     - "38443:38443"  # HTTPS端口
   ```

### 第三阶段：测试与部署（预计1-2天）

#### 3.1 测试计划
**单元测试**：
1. JWT令牌：生成、验证、过期测试
2. 密码策略：有效/无效密码验证
3. 输入验证：合法/非法目标验证
4. 账户锁定：失败尝试计数和锁定逻辑

**集成测试**：
1. 完整登录流程：使用JWT令牌访问受保护端点
2. 监控功能：验证所有监控数据正常收集
3. 管理功能：Docker、Systemd、Cron管理操作
4. 网络诊断：使用合法目标测试ping、trace等工具

**安全测试**：
1. 命令注入测试：尝试注入特殊字符和命令
2. 暴力破解测试：验证账户锁定机制
3. 令牌安全测试：尝试篡改和重用令牌
4. CSP/CORS测试：验证安全头是否生效

#### 3.2 部署策略
1. **分阶段部署**：
   - 阶段1：仅后端安全改进（JWT、密码策略、安全头）
   - 阶段2：前端适配（如有需要）
   - 阶段3：HTTPS部署（可选）

2. **回滚计划**：
   - 备份现有代码和配置
   - 准备快速回滚脚本
   - 监控关键指标（登录成功率、API响应时间）

## 兼容性保证

### API兼容性
- 登录接口响应格式不变
- 认证方式不变（支持三种令牌传递方式）
- WebSocket连接端点和工作方式不变

### 功能兼容性
- 所有系统监控指标收集和显示不受影响
- Docker、Systemd、Cron管理功能保持原样
- 现有前端HTML/JS/CSS无需修改

### 数据兼容性
- 用户数据格式不变（`users.json`）
- 配置文件格式和位置不变
- 会话数据：JWT替换后需要重新登录

## 风险评估与缓解

| 风险 | 描述 | 缓解措施 |
|------|------|----------|
| 会话中断 | JWT替换导致现有用户需要重新登录 | 在维护窗口实施，提前通知用户 |
| 密码策略影响 | 现有弱密码用户无法更改密码 | 仅对新密码和密码更改生效 |
| CSP破坏前端 | 严格CSP可能阻止前端脚本执行 | 使用较宽松策略，允许内联脚本 |
| 输入验证误报 | 合法网络目标被拒绝 | 广泛测试，确保常见格式通过 |
| 性能影响 | JWT验证比内存查找稍慢 | 性能测试，优化密钥管理 |

## 实施时间线

### 第1周：核心安全改进
- 周一：JWT会话管理实现
- 周二：密码策略和账户锁定
- 周三：API安全头完整化
- 周四：单元测试和集成测试
- 周五：修复问题和文档更新

### 第2周：输入验证与HTTPS
- 周一：输入验证强化
- 周二：HTTPS支持实现
- 周三：全面安全测试
- 周四：部署到测试环境
- 周五：生产环境部署准备

### 第3周：部署与监控
- 周一：生产环境部署（阶段1）
- 周二：监控和问题修复
- 周三：HTTPS部署（阶段2）
- 周四：最终安全审计
- 周五：项目总结和文档完善

## 成功标准

1. **安全性提升**：
   - 会话令牌使用标准JWT实现
   - 密码策略和账户锁定生效
   - 命令注入攻击被有效阻止
   - API安全头完整配置

2. **功能完整性**：
   - 所有监控功能正常工作
   - 管理操作不受影响
   - 用户界面无变化

3. **性能指标**：
   - 登录响应时间增加≤50ms
   - API吞吐量下降≤5%
   - 内存使用增加≤10MB

## 负责人与联系方式

- 项目负责人：[姓名]
- 安全工程师：[姓名]
- 开发工程师：[姓名]
- 测试工程师：[姓名]
- 运维工程师：[姓名]

## 修订历史

| 版本 | 日期 | 作者 | 说明 |
|------|------|------|------|
| 1.0 | 2025-12-14 | Cline | 初始版本，基于安全分析创建 |

---

**注意事项**：
1. 本计划不包含Docker配置安全加固，保持现有`docker-compose.yml`不变
2. 所有修改需确保向后兼容，不破坏现有功能
3. 每个阶段完成后需进行充分测试
4. 生产环境部署前需在测试环境验证

**下一步行动**：
1. 评审本计划
2. 切换到ACT模式开始实施
3. 按阶段顺序执行任务
