# Token Authentication System

A Telegram Bot based token authentication system with user registration, token generation, key management, API verification, online payment and IP binding features.

[中文文档](./docs/README_zh.md)

## 🚀 Features

### Core Features
- **Token Generation**: Users bind public IP to generate encrypted tokens
- **API Verification**: HTTP API for token validation and usage counting
- **Key Management**: Admins generate keys, users use keys to increase usage count
- **User Management**: Complete user information management and status tracking
- **Online Payment**: Integrated with EPay system, supporting WeChat Pay and Alipay
- **IP Rebinding**: Support for users to change bound IP address with payment

### Security Features
- **AES-GCM Encryption**: Using 256-bit AES-GCM encryption algorithm to protect tokens
- **IP Binding**: Tokens bound to user's public IP to prevent abuse
- **Deterministic Keys**: Deterministic encryption keys based on user ID and timestamp
- **Usage Limits**: Each token has usage limits
- **Payment Signature Verification**: Payment callback signature verification to prevent fraud

### User Experience
- **Button Interface**: Intuitive Telegram inline keyboard operations
- **Message Linking**: All operations on the same message for clean interface
- **Auto Timeout**: 5-minute inactivity auto-delete messages
- **Real-time Feedback**: Immediate operation result feedback
- **Payment Status Query**: Real-time order payment status checking

## 📋 System Architecture

### Data Structures

#### Config Structure
```go
type Config struct {
    Server struct {
        Port int    // HTTP server port
        Host string // HTTP server host
    }
    Bot struct {
        AdminIDs []int64 // Admin user ID list
        Token    string  // Telegram Bot Token
    }
    Database struct {
        Host     string // Database host
        Port     int    // Database port
        User     string // Database username
        Password string // Database password
        DBName   string // Database name
    }
    Limits struct {
        DefaultLimit int // Default usage count
        KeyAddLimit  int // Default key add count
    }
    Payment struct {
        BaseURL     string  // EPay API base URL
        MchID       string  // Merchant ID
        Secret      string  // Communication secret
        PricePerUse float64 // Price per use
        NotifyURL   string  // Async callback URL
        ReturnURL   string  // Sync callback URL
    }
}
```

#### User Record
```go
type UserRecord struct {
    UserID    string // User ID
    IP        string // Bound public IP
    Token     string // Encrypted token
    Limit     int    // Remaining usage count
    Timestamp int64  // Creation timestamp
    CreatedAt string // Creation time string
}
```

#### Key Record
```go
type KeyRecord struct {
    Key       string // Key string
    AddLimit  int    // Count to add
    Used      bool   // Whether used
    UsedBy    string // User ID who used
    CreatedBy string // Admin ID who created
    CreatedAt string // Creation time
    UsedAt    string // Usage time
}
```

#### Order Record
```go
type Order struct {
    PayID       string     // Merchant order number
    UserID      string     // User ID
    Count       int        // Purchase count
    GoodsName   string     // Product name
    Price       float64    // Order amount
    Status      string     // Order status
    CreateTime  time.Time  // Creation time
    PayTime     *time.Time // Payment time
    PayType     int        // Payment method
    ReallyPrice float64    // Actual payment amount
    OrderID     string     // EPay order number
    ChatID      int64      // Chat ID
    MessageID   int        // Message ID
}
```

### Core Modules

#### 1. Database Module
- **MySQL Connection**: Using MySQL to store user, key and order data
- **Transaction Processing**: Key usage and other critical operations use transactions to ensure data consistency
- **Connection Pool Management**: Set connection pool parameters to optimize performance

#### 2. Encryption Module
- **AES Key Generation**: `generateAESKey()` - Generate 256-bit random key
- **Deterministic Key**: `generateDeterministicKey()` - Generate based on user ID and timestamp
- **Token Encryption**: `encryptPayload()` - AES-GCM encrypt user data
- **Token Decryption**: `decryptToken()` - Decrypt and verify token

#### 3. Verification Module
- **IP Verification**: Check public IP validity, reject private addresses
- **Token Verification**: Complete token decryption and verification process
- **Usage Count Management**: Auto deduct and update usage count

#### 4. Bot Interface Module
- **State Management**: User operation state tracking
- **Message Timeout**: Auto cleanup of timeout messages
- **Keyboard Management**: Dynamic inline keyboard generation

#### 5. Payment Module
- **EPay Integration**: Support WeChat Pay and Alipay
- **Order Management**: Create, query and update orders
- **Payment Callback**: Handle payment success notifications
- **Signature Verification**: Verify payment callback signatures

## 🎮 User Operation Flow

### Regular User Features

#### 1. Get Token
```
User clicks "🐳 Get Token" → 
Enter public IP address → 
System validates IP → 
Generate encrypted token → 
Return token and initial usage count
```

#### 2. View Account Info
```
User clicks "🛳️ Account Info" → 
Display user ID, bound IP, remaining count, token, etc.
```

#### 3. Use Key
```
User clicks "💻 Use Key" → 
Enter 32-digit key → 
System validates key → 
Increase usage count → 
Update account info
```

#### 4. Recharge Count
```
User clicks "💰 Recharge Count" → 
Enter count to recharge → 
Confirm order info → 
Redirect to payment page → 
Complete payment → 
Auto increase usage count
```

#### 5. Rebind IP
```
User clicks "🔥 Rebind IP" → 
Enter new public IP → 
System validates IP → 
Create rebind order → 
Complete payment → 
Auto update IP and generate new token
```

### Admin Features

#### 1. Generate Key
```
Admin clicks "🛠️ Admin Features" → 
Click "🎉 Generate Key" → 
Enter count to add → 
Confirm generation → 
Return 32-digit key
```

## 🔌 API Interfaces

### POST /verify
Verify token validity and usage count

#### Request Format
```json
{
    "token": "Encrypted token string"
}
```

#### Response Format
```json
{
    "success": true/false,
    "message": "Response message",
    "user_id": "User ID",
    "limit": remaining count
}
```

#### Response Status Codes
- `200`: Verification successful
- `400`: Request format error or invalid IP
- `401`: Invalid token or IP mismatch
- `403`: Insufficient usage count
- `500`: System error

### GET/POST /notify
EPay async callback interface

### GET /return
EPay sync callback interface

## 🛠️ Technical Implementation

### Encryption Algorithm
- **Algorithm**: AES-256-GCM
- **Key Length**: 256-bit (32 bytes)
- **Nonce**: 96-bit (12 bytes)
- **Authentication**: GCM mode provides integrity verification

### Token Structure
```
[Timestamp(8 bytes)] + [UserID length(1 byte)] + [UserID] + [Nonce(12 bytes)] + [Ciphertext]
```

### Key Generation
- **Algorithm**: MD5 hash
- **Input**: Timestamp + Admin ID
- **Output**: 32-digit hex string

### Data Storage
- **Database**: MySQL
- **Tables**:
  - `users`: User information table
  - `card_keys`: Key information table
  - `orders`: Order information table

## 🔒 Security Mechanisms

### 1. IP Binding Verification
- Reject private addresses (10.x.x.x, 172.16-31.x.x, 192.168.x.x, 127.x.x.x)
- Validate IP format
- Strong token-IP binding

### 2. Usage Count Control
- Auto deduct count on each verification
- Reject verification when count insufficient
- Support increasing count via keys or online payment

### 3. Message Security
- Auto delete user input messages
- 5-minute timeout auto cleanup
- Prevent information leakage

### 4. Admin Permissions
- User ID based permission control
- Only configured admins can generate keys
- Operation log recording

### 5. Payment Security
- Signature verification for payment callbacks
- Real-time order status query
- Transaction processing ensures data consistency

## 📁 File Structure

```
token-auth-system/
├── main.go          # Main program file
├── config.toml      # Configuration file
├── README.md        # Project documentation (English)
├── docs/
│   └── README_zh.md # Chinese documentation
└── sql/
    ├── schema.sql   # Database table structure
    └── init.sql     # Initialization data
```

## 🗄️ Database Table Structure

### users table
```sql
CREATE TABLE `users` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` varchar(64) NOT NULL,
  `ip` varchar(64) NOT NULL,
  `token` text NOT NULL,
  `limit_count` int NOT NULL DEFAULT '0',
  `timestamp` bigint NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `user_id` (`user_id`),
  UNIQUE KEY `ip` (`ip`)
);
```

### card_keys table
```sql
CREATE TABLE `card_keys` (
  `id` int NOT NULL AUTO_INCREMENT,
  `key_code` varchar(32) NOT NULL,
  `add_limit` int NOT NULL,
  `used` tinyint(1) NOT NULL DEFAULT '0',
  `used_by` varchar(64) DEFAULT NULL,
  `created_by` varchar(64) NOT NULL,
  `created_at` datetime NOT NULL,
  `used_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `key_code` (`key_code`)
);
```

### orders table
```sql
CREATE TABLE `orders` (
  `id` int NOT NULL AUTO_INCREMENT,
  `pay_id` varchar(64) NOT NULL,
  `order_id` varchar(64) DEFAULT NULL,
  `user_id` varchar(64) NOT NULL,
  `count` int NOT NULL DEFAULT '0',
  `goods_name` varchar(255) NOT NULL,
  `price` decimal(10,2) NOT NULL,
  `really_price` decimal(10,2) DEFAULT NULL,
  `status` varchar(32) NOT NULL,
  `pay_type` int DEFAULT NULL,
  `pay_time` datetime DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime DEFAULT NULL,
  `chat_id` bigint DEFAULT NULL,
  `message_id` int DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `pay_id` (`pay_id`),
  KEY `user_id` (`user_id`),
  KEY `status` (`status`),
  KEY `order_id` (`order_id`)
);
```

## ⚙️ Configuration

### config.toml Example
```toml
[server]
port = 8080
host = "0.0.0.0"

[bot]
token = "YOUR_BOT_TOKEN_HERE"
admin_ids = [123456789, 987654321]

[database]
host = "localhost"
port = 3306
user = "token_auth"
password = "your_password"
db_name = "token_auth"

[limits]
default_limit = 10
key_add_limit = 5

[payment]
base_url = "https://epay.example.com"
mch_id = "your_merchant_id"
secret = "your_payment_secret"
price_per_use = 0.1
notify_url = "https://your-domain.com/notify"
return_url = "https://your-domain.com/return"
```

## 🚀 Deployment

### 1. Requirements
- Go 1.16+
- MySQL 5.7+
- Telegram Bot Token
- Public server
- EPay merchant account

### 2. Database Preparation
```bash
mysql -u root -p < sql/schema.sql
```

### 3. Install Dependencies
```bash
go mod tidy
```

### 4. Configuration
Edit `config.toml` to set Bot Token, database connection and payment parameters

### 5. Run Program
```bash
go run main.go
```

### 6. Verify Deployment
- Visit `http://your-server:8080/health` to check service status
- Send `/help` to the Telegram Bot to test functionality

## 📊 Monitoring and Logs

### Log Levels
- `[INFO]`: Normal operation information
- `[WARN]`: Warning information
- `[ERROR]`: Error information
- `[FATAL]`: Fatal error
- `[DEBUG]`: Debug information

### Key Metrics
- HTTP request response time
- Token verification success rate
- User registration count
- Key usage status
- Order payment conversion rate
- System error rate

## 🔄 Changelog

### v1.1.0
- ✅ Migrated database from JSON files to MySQL
- ✅ Integrated EPay system for online recharging
- ✅ Added IP rebinding feature
- ✅ Optimized message management and timeout mechanism
- ✅ Enhanced security and error handling

### v1.0.0
- ✅ Basic token generation and verification functionality
- ✅ Key system implementation
- ✅ Telegram Bot interface
- ✅ Message linking and timeout management
- ✅ Complete security mechanisms
- ✅ API interface implementation

## 📞 Technical Support

For questions or suggestions, please contact:
- Create GitHub Issue
- Contact system administrator

---

**Note**: Please keep Bot Token and admin permissions secure to ensure system security.
