# Token 验证系统

一个基于 Telegram Bot 的 Token 验证系统，支持用户注册、Token 生成、卡密管理和 API 验证功能。

## 🚀 功能特性

### 核心功能
- **Token 生成**: 用户绑定公网IP生成专属加密Token
- **API 验证**: HTTP API接口验证Token有效性和使用次数
- **卡密系统**: 管理员生成卡密，用户使用卡密增加使用次数
- **用户管理**: 完整的用户信息管理和状态跟踪

### 安全特性
- **AES-GCM 加密**: 使用256位AES-GCM加密算法保护Token
- **IP 绑定**: Token与用户公网IP绑定，防止滥用
- **确定性密钥**: 基于用户ID和时间戳生成确定性加密密钥
- **使用次数限制**: 每个Token有使用次数限制

### 用户体验
- **按钮式界面**: 直观的Telegram内联键盘操作
- **消息链接**: 所有操作在同一消息上进行，界面整洁
- **自动超时**: 5分钟无操作自动删除消息
- **实时反馈**: 即时的操作结果反馈

## 📋 系统架构

### 数据结构

#### 配置结构 (Config)
```go
type Config struct {
    Server struct {
        Port int    // HTTP服务器端口
        Host string // HTTP服务器主机
    }
    Bot struct {
        AdminIDs []int64 // 管理员用户ID列表
        Token    string  // Telegram Bot Token
    }
    Database struct {
        File     string // 用户数据库文件路径
        KeysFile string // 卡密数据库文件路径
    }
    Limits struct {
        DefaultLimit int // 默认使用次数
        KeyAddLimit  int // 卡密默认增加次数
    }
}
```

#### 用户记录 (UserRecord)
```go
type UserRecord struct {
    UserID    string // 用户ID
    IP        string // 绑定的公网IP
    Token     string // 加密Token
    Limit     int    // 剩余使用次数
    Timestamp int64  // 创建时间戳
    CreatedAt string // 创建时间字符串
}
```

#### 卡密记录 (KeyRecord)
```go
type KeyRecord struct {
    Key       string // 卡密字符串
    AddLimit  int    // 可增加的次数
    Used      bool   // 是否已使用
    UsedBy    string // 使用者ID
    CreatedBy string // 创建者ID
    CreatedAt string // 创建时间
    UsedAt    string // 使用时间
}
```

### 核心模块

#### 1. 加密模块
- **AES密钥生成**: `generateAESKey()` - 生成256位随机密钥
- **确定性密钥**: `generateDeterministicKey()` - 基于用户ID和时间戳生成
- **Token加密**: `encryptPayload()` - AES-GCM加密用户数据
- **Token解密**: `decryptToken()` - 解密并验证Token

#### 2. 数据库模块
- **用户数据库**: JSON格式存储用户记录
- **卡密数据库**: JSON格式存储卡密记录
- **数据持久化**: 自动保存和加载数据

#### 3. 验证模块
- **IP验证**: 检查公网IP有效性，拒绝内网地址
- **Token验证**: 完整的Token解密和验证流程
- **使用次数管理**: 自动扣减和更新使用次数

#### 4. Bot界面模块
- **状态管理**: 用户操作状态跟踪
- **消息超时**: 自动清理超时消息
- **键盘管理**: 动态生成内联键盘

## 🎮 用户操作流程

### 普通用户功能

#### 1. 获取Token
```
用户点击"🐳 获取Token" → 
输入公网IP地址 → 
系统验证IP有效性 → 
生成加密Token → 
返回Token和初始使用次数
```

#### 2. 查看账户信息
```
用户点击"🛳️ 账户信息" → 
显示用户ID、绑定IP、剩余次数、Token等信息
```

#### 3. 使用卡密
```
用户点击"💻 使用卡密" → 
输入32位卡密 → 
系统验证卡密有效性 → 
增加使用次数 → 
更新账户信息
```

### 管理员功能

#### 1. 生成卡密
```
管理员点击"🛠️ 管理员功能" → 
点击"🎉 生成卡密" → 
输入可增加的次数 → 
确认生成 → 
返回32位卡密
```

## 🔌 API 接口

### POST /verify
验证Token有效性和使用次数

#### 请求格式
```json
{
    "token": "加密的Token字符串"
}
```

#### 响应格式
```json
{
    "success": true/false,
    "message": "响应消息",
    "user_id": "用户ID",
    "limit": 剩余次数
}
```

#### 响应状态码
- `200`: 验证成功
- `400`: 请求格式错误或IP无效
- `401`: Token无效或IP不匹配
- `403`: 使用次数不足
- `500`: 系统错误

## 🛠️ 技术实现

### 加密算法
- **算法**: AES-256-GCM
- **密钥长度**: 256位 (32字节)
- **随机数**: 96位 (12字节) Nonce
- **认证**: GCM模式提供完整性验证

### Token结构
```
[时间戳(8字节)] + [用户ID长度(1字节)] + [用户ID] + [Nonce(12字节)] + [密文]
```

### 卡密生成
- **算法**: MD5哈希
- **输入**: 时间戳 + 管理员ID
- **输出**: 32位十六进制字符串

### 数据存储
- **格式**: JSON
- **编码**: UTF-8
- **备份**: 自动保存机制

## 🔒 安全机制

### 1. IP绑定验证
- 拒绝内网地址 (10.x.x.x, 172.16-31.x.x, 192.168.x.x, 127.x.x.x)
- 验证IP格式有效性
- Token与IP强绑定

### 2. 使用次数控制
- 每次验证自动扣减次数
- 次数不足时拒绝验证
- 支持通过卡密增加次数

### 3. 消息安全
- 自动删除用户输入消息
- 5分钟超时自动清理
- 防止信息泄露

### 4. 管理员权限
- 基于用户ID的权限控制
- 只有配置的管理员可生成卡密
- 操作日志记录

## 📁 文件结构

```
go/
├── main.go          # 主程序文件
├── config.toml      # 配置文件
├── users.json       # 用户数据库
├── keys.json        # 卡密数据库
└── README.md        # 项目文档
```

## ⚙️ 配置说明

### config.toml 示例
```toml
[server]
port = 8080
host = "0.0.0.0"

[bot]
token = "YOUR_BOT_TOKEN_HERE"
admin_ids = [123456789, 987654321]

[database]
file = "users.json"
keys_file = "keys.json"

[limits]
default_limit = 10
key_add_limit = 5
```

## 🚀 部署运行

### 1. 环境要求
- Go 1.16+
- Telegram Bot Token
- 公网服务器

### 2. 安装依赖
```bash
go mod tidy
```

### 3. 配置文件
编辑 `config.toml` 设置Bot Token和管理员ID

### 4. 运行程序
```bash
go run main.go
```

### 5. 验证部署
- 访问 `http://your-server:8080/health` 检查服务状态
- 在Telegram中向Bot发送 `/start` 测试功能

## 📊 监控和日志

### 日志级别
- `[INFO]`: 正常操作信息
- `[WARN]`: 警告信息
- `[ERROR]`: 错误信息
- `[FATAL]`: 致命错误
- `[DEBUG]`: 调试信息

### 关键监控指标
- HTTP请求响应时间
- Token验证成功率
- 用户注册数量
- 卡密使用情况
- 系统错误率

## 🔄 更新日志

### v1.0.0
- ✅ 基础Token生成和验证功能
- ✅ 卡密系统实现
- ✅ Telegram Bot界面
- ✅ 消息链接和超时管理
- ✅ 完整的安全机制
- ✅ API接口实现

## 📞 技术支持

如有问题或建议，请通过以下方式联系：
- 创建 GitHub Issue
- 联系系统管理员

---

**注意**: 请妥善保管Bot Token和管理员权限，确保系统安全运行。
