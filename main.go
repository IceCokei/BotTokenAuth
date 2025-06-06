package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	cryptorand "crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Config struct {
	Server struct {
		Port int    `toml:"port"`
		Host string `toml:"host"`
	} `toml:"server"`
	Bot struct {
		AdminIDs []int64 `toml:"admin_ids"`
		Token    string  `toml:"token"`
	} `toml:"bot"`
	Database struct {
		Host     string `toml:"host"`
		Port     int    `toml:"port"`
		User     string `toml:"user"`
		Password string `toml:"password"`
		DBName   string `toml:"db_name"`
	} `toml:"database"`
	Limits struct {
		DefaultLimit int `toml:"default_limit"`
		KeyAddLimit  int `toml:"key_add_limit"`
	} `toml:"limits"`
	Payment struct {
		BaseURL     string  `toml:"base_url"`
		MchID       string  `toml:"mch_id"`
		Secret      string  `toml:"secret"`
		PricePerUse float64 `toml:"price_per_use"`
		NotifyURL   string  `toml:"notify_url"`
		ReturnURL   string  `toml:"return_url"`
	} `toml:"payment"`
}

type Payload struct {
	UserID    string `json:"user_id"`
	IP        string `json:"ip"`
	Timestamp int64  `json:"timestamp"`
}

type UserRecord struct {
	UserID    string `json:"user_id"`
	IP        string `json:"ip"`
	Token     string `json:"token"`
	Limit     int    `json:"limit"`
	Timestamp int64  `json:"timestamp"`
	CreatedAt string `json:"created_at"`
}

type UserDatabase struct {
	Records []UserRecord `json:"records"`
}

type KeyRecord struct {
	Key       string `json:"key"`
	AddLimit  int    `json:"add_limit"`
	Used      bool   `json:"used"`
	UsedBy    string `json:"used_by"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
	UsedAt    string `json:"used_at"`
}

type KeyDatabase struct {
	Keys []KeyRecord `json:"keys"`
}

type VerifyRequest struct {
	Token string `json:"token"`
}

type VerifyResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	UserID  string `json:"user_id,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// 简化的IP信息结构体
type IPInfoResponse struct {
	IP string `json:"ip"`
}

// 用户状态管理
type UserState struct {
	State     string
	Data      map[string]interface{}
	MessageID int         // 添加消息ID跟踪
	Timer     *time.Timer // 添加定时器
}

// 消息超时管理
type MessageTimeout struct {
	UserID    int64
	ChatID    int64
	MessageID int
	Timer     *time.Timer
}

// EpayClient 易支付客户端
type EpayClient struct {
	BaseURL string // 易支付API基础地址
	MchID   string // 商户ID
	Secret  string // 通讯密钥
}

// CreateOrderRequest 创建订单请求参数
type CreateOrderRequest struct {
	MchID     string  `json:"mchId"`     // 商户ID
	PayID     string  `json:"payId"`     // 商户支付单号
	Type      int     `json:"type"`      // 支付方式 1:微信 2:支付宝
	Price     float64 `json:"price"`     // 订单金额
	GoodsName string  `json:"goodsName"` // 商品名称
	Param     string  `json:"param"`     // 传输参数(可选)
	IsHTML    int     `json:"isHtml"`    // 0:返回json 1:跳转支付页面
	NotifyURL string  `json:"notifyUrl"` // 异步回调地址(可选)
	ReturnURL string  `json:"returnUrl"` // 同步回调地址(可选)
	Sign      string  `json:"sign"`      // 签名
}

// CreateOrderResponse 创建订单响应
type CreateOrderResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data *struct {
		PayID       string  `json:"payId"`       // 商户支付单号
		OrderID     string  `json:"orderId"`     // 云端订单号
		PayType     int     `json:"payType"`     // 支付方式
		Price       float64 `json:"price"`       // 订单金额
		ReallyPrice float64 `json:"reallyPrice"` // 实际支付金额
		PayURL      string  `json:"payUrl"`      // 支付二维码URL
		IsAuto      int     `json:"isAuto"`      // 是否自动输入金额
		State       int     `json:"state"`       // 订单状态
		TimeOut     int     `json:"timeOut"`     // 有效时间(分钟)
		Date        int64   `json:"date"`        // 创建时间戳
	} `json:"data"`
}

// GetOrderResponse 查询订单响应
type GetOrderResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data *struct {
		PayID       string  `json:"payId"`       // 商户订单号
		OrderID     string  `json:"orderId"`     // 云端订单号
		PayType     int     `json:"payType"`     // 支付方式
		Price       float64 `json:"price"`       // 订单金额
		ReallyPrice float64 `json:"reallyPrice"` // 实际支付金额
		PayURL      string  `json:"payUrl"`      // 支付二维码内容
		IsAuto      int     `json:"isAuto"`      // 是否自动输入金额
		State       int     `json:"state"`       // 订单状态
		TimeOut     int     `json:"timeOut"`     // 有效时间
		Date        int64   `json:"date"`        // 创建时间戳
	} `json:"data"`
}

// CheckOrderResponse 检查订单状态响应
type CheckOrderResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data string `json:"data"` // 跳转地址或null
}

// CallbackParams 回调参数
type CallbackParams struct {
	MchID       string  `json:"mchId"`       // 商户ID
	OrderID     string  `json:"orderId"`     // 云端订单号
	Param       string  `json:"param"`       // 传输参数
	Type        int     `json:"type"`        // 支付方式
	Price       float64 `json:"price"`       // 订单金额
	ReallyPrice float64 `json:"reallyPrice"` // 实际支付金额
	Sign        string  `json:"sign"`        // 校验签名
}

// Order 订单信息
type Order struct {
	PayID       string     `json:"payId"`
	UserID      string     `json:"userId"` // 添加用户ID
	Count       int        `json:"count"`  // 购买次数
	GoodsName   string     `json:"goodsName"`
	Price       float64    `json:"price"`
	Status      string     `json:"status"`
	CreateTime  time.Time  `json:"createTime"`
	PayTime     *time.Time `json:"payTime,omitempty"`
	PayType     int        `json:"payType,omitempty"`
	ReallyPrice float64    `json:"reallyPrice,omitempty"`
	OrderID     string     `json:"orderId,omitempty"`   // 易支付订单号
	ChatID      int64      `json:"chatId,omitempty"`    // 聊天ID
	MessageID   int        `json:"messageId,omitempty"` // 消息ID
}

var (
	config          Config
	userKeys        = make(map[int64][]byte)     // 缓存用户 AES 密钥
	userStates      = make(map[int64]*UserState) // 用户状态管理
	chinaLocation   *time.Location
	messageTimeouts = make(map[string]*MessageTimeout) // 消息超时管理
	epayClient      *EpayClient                        // 易支付客户端
	orderDB         = make(map[string]*Order)          // 订单数据库 (临时，将迁移到MySQL)
	db              *sql.DB                            // MySQL数据库连接
)

func init() {
	var err error
	chinaLocation, err = time.LoadLocation("Asia/Shanghai")
	if err != nil {
		log.Fatal("[FATAL] 加载时区失败:", err)
	}
}

// 初始化MySQL数据库连接
func initDatabase() error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		config.Database.User,
		config.Database.Password,
		config.Database.Host,
		config.Database.Port,
		config.Database.DBName)

	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("连接数据库失败: %v", err)
	}

	// 测试连接
	if err = db.Ping(); err != nil {
		return fmt.Errorf("数据库连接测试失败: %v", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	log.Printf("[INFO] MySQL数据库连接成功: %s:%d/%s", config.Database.Host, config.Database.Port, config.Database.DBName)

	return nil
}

// 加载用户数据库 - 替换为MySQL版本
func loadDatabase() (*UserDatabase, error) {
	query := "SELECT user_id, ip, token, limit_count, timestamp, created_at FROM users"
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("查询用户数据失败: %v", err)
	}
	defer rows.Close()

	userDB := &UserDatabase{Records: []UserRecord{}}

	for rows.Next() {
		var record UserRecord
		var createdAt time.Time

		err := rows.Scan(&record.UserID, &record.IP, &record.Token,
			&record.Limit, &record.Timestamp, &createdAt)
		if err != nil {
			log.Printf("[WARN] 扫描用户记录失败: %v", err)
			continue
		}

		record.CreatedAt = createdAt.In(chinaLocation).Format("2006-01-02 15:04:05 CST")
		userDB.Records = append(userDB.Records, record)
	}

	return userDB, nil
}

// 检查用户是否存在 - MySQL版本
func userExists(userID string) (bool, error) {
	query := "SELECT COUNT(*) FROM users WHERE user_id = ?"
	var count int
	err := db.QueryRow(query, userID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// 获取用户信息 - MySQL版本
func getUserInfo(userID string) (*UserRecord, error) {
	query := "SELECT user_id, ip, token, limit_count, timestamp, created_at FROM users WHERE user_id = ?"
	var record UserRecord
	var createdAt time.Time

	err := db.QueryRow(query, userID).Scan(&record.UserID, &record.IP, &record.Token,
		&record.Limit, &record.Timestamp, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	record.CreatedAt = createdAt.In(chinaLocation).Format("2006-01-02 15:04:05 CST")
	return &record, nil
}

// 检查IP是否已被使用 - MySQL版本
func ipExists(ip string) (bool, string, error) {
	query := "SELECT user_id FROM users WHERE ip = ?"
	var userID string
	err := db.QueryRow(query, ip).Scan(&userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, "", nil
		}
		return false, "", err
	}
	return true, userID, nil
}

// 添加用户记录 - MySQL版本
func addUserRecord(userID, ip, token string, limit int, timestamp int64) error {
	query := `INSERT INTO users (user_id, ip, token, limit_count, timestamp, created_at) 
			  VALUES (?, ?, ?, ?, ?, ?)`

	createdAt := time.Now().In(chinaLocation)
	_, err := db.Exec(query, userID, ip, token, limit, timestamp, createdAt)
	if err != nil {
		return fmt.Errorf("插入用户记录失败: %v", err)
	}

	log.Printf("[INFO] 用户记录已保存到MySQL: %s", userID)
	return nil
}

// 更新用户次数 - MySQL版本
func updateUserLimit(userID string, addLimit int) error {
	query := "UPDATE users SET limit_count = limit_count + ?, updated_at = ? WHERE user_id = ?"
	result, err := db.Exec(query, addLimit, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("更新用户次数失败: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %v", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}

	log.Printf("[INFO] 用户 %s 次数已更新: %+d", userID, addLimit)
	return nil
}

// 加载卡密数据库 - MySQL版本
func loadKeyDatabase() (*KeyDatabase, error) {
	query := `SELECT key_code, add_limit, used, COALESCE(used_by, ''), created_by, 
			  created_at, COALESCE(used_at, '1970-01-01 00:00:00') 
			  FROM card_keys ORDER BY created_at DESC`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("查询卡密数据失败: %v", err)
	}
	defer rows.Close()

	keyDB := &KeyDatabase{Keys: []KeyRecord{}}

	for rows.Next() {
		var record KeyRecord
		var createdAt, usedAt time.Time

		err := rows.Scan(&record.Key, &record.AddLimit, &record.Used,
			&record.UsedBy, &record.CreatedBy, &createdAt, &usedAt)
		if err != nil {
			log.Printf("[WARN] 扫描卡密记录失败: %v", err)
			continue
		}

		record.CreatedAt = createdAt.In(chinaLocation).Format("2006-01-02 15:04:05 CST")
		if !usedAt.IsZero() && usedAt.Year() > 1970 {
			record.UsedAt = usedAt.In(chinaLocation).Format("2006-01-02 15:04:05 CST")
		}

		keyDB.Keys = append(keyDB.Keys, record)
	}

	return keyDB, nil
}

// 添加卡密 - MySQL版本
func addKey(addLimit int, adminID int64) (string, error) {
	key := generateKey(adminID)
	query := `INSERT INTO card_keys (key_code, add_limit, created_by, created_at) 
			  VALUES (?, ?, ?, ?)`

	createdAt := time.Now().In(chinaLocation)
	_, err := db.Exec(query, key, addLimit, fmt.Sprintf("%d", adminID), createdAt)
	if err != nil {
		return "", fmt.Errorf("插入卡密失败: %v", err)
	}

	log.Printf("[INFO] 卡密已保存到MySQL: %s", key)
	return key, nil
}

// 使用卡密 - MySQL版本
func useKey(key, userID string) (int, error) {
	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("开始事务失败: %v", err)
	}
	defer tx.Rollback()

	// 查询卡密
	var addLimit int
	var used bool
	query := "SELECT add_limit, used FROM card_keys WHERE key_code = ? FOR UPDATE"
	err = tx.QueryRow(query, key).Scan(&addLimit, &used)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("卡密不存在")
		}
		return 0, fmt.Errorf("查询卡密失败: %v", err)
	}

	if used {
		return 0, fmt.Errorf("卡密已被使用")
	}

	// 更新卡密状态
	updateQuery := "UPDATE card_keys SET used = TRUE, used_by = ?, used_at = ? WHERE key_code = ?"
	usedAt := time.Now().In(chinaLocation)
	_, err = tx.Exec(updateQuery, userID, usedAt, key)
	if err != nil {
		return 0, fmt.Errorf("更新卡密状态失败: %v", err)
	}

	// 提交事务
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交事务失败: %v", err)
	}

	log.Printf("[INFO] 卡密使用成功: %s -> 用户 %s", key, userID)
	return addLimit, nil
}

// 保存订单到数据库
func saveOrderToDB(order *Order) error {
	query := `INSERT INTO orders (pay_id, order_id, user_id, count, goods_name, price, 
			  really_price, status, pay_type, pay_time, created_at, chat_id, message_id) 
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	var payTime *time.Time
	if order.PayTime != nil {
		payTime = order.PayTime
	}

	_, err := db.Exec(query, order.PayID, order.OrderID, order.UserID, order.Count,
		order.GoodsName, order.Price, order.ReallyPrice, order.Status, order.PayType,
		payTime, order.CreateTime, order.ChatID, order.MessageID)

	if err != nil {
		return fmt.Errorf("保存订单失败: %v", err)
	}

	log.Printf("[INFO] 订单已保存到MySQL: %s", order.PayID)
	return nil
}

// 更新订单状态
func updateOrderStatus(payID string, status string, reallyPrice float64, payType int) error {
	query := `UPDATE orders SET status = ?, really_price = ?, pay_type = ?, 
			  pay_time = ?, updated_at = ? WHERE pay_id = ?`

	payTime := time.Now()
	_, err := db.Exec(query, status, reallyPrice, payType, payTime, payTime, payID)
	if err != nil {
		return fmt.Errorf("更新订单状态失败: %v", err)
	}

	log.Printf("[INFO] 订单状态已更新: %s -> %s", payID, status)
	return nil
}

// 更新订单的易支付信息
func updateOrderWithEpayInfo(payID string, epayOrderID string, reallyPrice float64, payType int) error {
	query := `UPDATE orders SET order_id = ?, really_price = ?, pay_type = ?, 
			  updated_at = ? WHERE pay_id = ?`

	_, err := db.Exec(query, epayOrderID, reallyPrice, payType, time.Now(), payID)
	if err != nil {
		return fmt.Errorf("更新订单易支付信息失败: %v", err)
	}

	log.Printf("[INFO] 订单易支付信息已更新: %s -> OrderID: %s", payID, epayOrderID)
	return nil
}

// 根据PayID获取订单
func getOrderByPayID(payID string) (*Order, error) {
	query := `SELECT pay_id, COALESCE(order_id, ''), user_id, count, goods_name, 
			  price, COALESCE(really_price, 0), status, COALESCE(pay_type, 0), 
			  created_at, pay_time, COALESCE(chat_id, 0), COALESCE(message_id, 0) 
			  FROM orders WHERE pay_id = ?`

	var order Order
	var payTime sql.NullTime

	err := db.QueryRow(query, payID).Scan(&order.PayID, &order.OrderID, &order.UserID,
		&order.Count, &order.GoodsName, &order.Price, &order.ReallyPrice, &order.Status,
		&order.PayType, &order.CreateTime, &payTime, &order.ChatID, &order.MessageID)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("查询订单失败: %v", err)
	}

	if payTime.Valid {
		order.PayTime = &payTime.Time
	}

	return &order, nil
}

// 根据易支付OrderID获取订单
func getOrderByEpayOrderID(orderID string) (*Order, error) {
	query := `SELECT pay_id, COALESCE(order_id, ''), user_id, count, goods_name, 
			  price, COALESCE(really_price, 0), status, COALESCE(pay_type, 0), 
			  created_at, pay_time, COALESCE(chat_id, 0), COALESCE(message_id, 0) 
			  FROM orders WHERE order_id = ?`

	var order Order
	var payTime sql.NullTime

	err := db.QueryRow(query, orderID).Scan(&order.PayID, &order.OrderID, &order.UserID,
		&order.Count, &order.GoodsName, &order.Price, &order.ReallyPrice, &order.Status,
		&order.PayType, &order.CreateTime, &payTime, &order.ChatID, &order.MessageID)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("查询订单失败: %v", err)
	}

	if payTime.Valid {
		order.PayTime = &payTime.Time
	}

	return &order, nil
}

// 根据用户ID获取最新的待支付订单
func getLatestPendingOrderByUserID(userID string) (*Order, error) {
	query := `SELECT pay_id, COALESCE(order_id, ''), user_id, count, goods_name, 
			  price, COALESCE(really_price, 0), status, COALESCE(pay_type, 0), 
			  created_at, pay_time, COALESCE(chat_id, 0), COALESCE(message_id, 0) 
			  FROM orders 
			  WHERE user_id = ? AND status = 'pending' 
			  ORDER BY created_at DESC LIMIT 1`

	var order Order
	var payTime sql.NullTime

	err := db.QueryRow(query, userID).Scan(&order.PayID, &order.OrderID, &order.UserID,
		&order.Count, &order.GoodsName, &order.Price, &order.ReallyPrice, &order.Status,
		&order.PayType, &order.CreateTime, &payTime, &order.ChatID, &order.MessageID)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("查询用户最新待支付订单失败: %v", err)
	}

	if payTime.Valid {
		order.PayTime = &payTime.Time
	}

	return &order, nil
}

func loadConfig() error {
	if _, err := toml.DecodeFile("config.toml", &config); err != nil {
		return err
	}
	log.Printf("[INFO] 配置加载成功: 端口=%d, 管理员数量=%d", config.Server.Port, len(config.Bot.AdminIDs))
	return nil
}

func generateAESKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := cryptorand.Read(key)
	return key, err
}

func generateNonce() ([]byte, error) {
	nonce := make([]byte, 12)
	_, err := io.ReadFull(cryptorand.Reader, nonce)
	return nonce, err
}

func encryptPayload(payload Payload, key []byte) (string, error) {
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	nonce, err := generateNonce()
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)

	// 在Token前面加上时间戳（8字节）和用户ID长度信息
	timestampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timestampBytes, uint64(payload.Timestamp))

	userIDBytes := []byte(payload.UserID)
	userIDLen := byte(len(userIDBytes))

	// 最终格式: [timestamp(8)] + [userID_len(1)] + [userID] + [nonce(12)] + [ciphertext]
	final := append(timestampBytes, userIDLen)
	final = append(final, userIDBytes...)
	final = append(final, nonce...)
	final = append(final, ciphertext...)

	return hex.EncodeToString(final), nil
}

func decryptToken(tokenHex string, key []byte) (*Payload, error) {
	data, err := hex.DecodeString(tokenHex)
	if err != nil {
		return nil, err
	}

	if len(data) < 12 {
		return nil, fmt.Errorf("token太短")
	}

	nonce := data[:12]
	ciphertext := data[12:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	var payload Payload
	err = json.Unmarshal(plaintext, &payload)
	return &payload, err
}

// 检查是否为局域网地址
func isPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// 检查常见的私有网络地址段
	privateRanges := []string{
		"10.0.0.0/8",     // 10.0.0.0 - 10.255.255.255
		"172.16.0.0/12",  // 172.16.0.0 - 172.31.255.255
		"192.168.0.0/16", // 192.168.0.0 - 192.168.255.255
		"127.0.0.0/8",    // 127.0.0.0 - 127.255.255.255 (localhost)
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(parsedIP) {
			return true
		}
	}
	return false
}

// 验证IP地址（必须是有效的公网IP）
func isValidPublicIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	return !isPrivateIP(ip)
}

// 获取客户端真实IP
func getRealIP(c *gin.Context) string {
	// 优先从 X-Forwarded-For 获取
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			log.Printf("[DEBUG] 从X-Forwarded-For获取IP: %s", clientIP)
			return clientIP
		}
	}

	// 从 X-Real-IP 获取
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		clientIP := strings.TrimSpace(xri)
		log.Printf("[DEBUG] 从X-Real-IP获取IP: %s", clientIP)
		return clientIP
	}

	// 从 CF-Connecting-IP 获取（Cloudflare）
	if cfIP := c.GetHeader("CF-Connecting-IP"); cfIP != "" {
		clientIP := strings.TrimSpace(cfIP)
		log.Printf("[DEBUG] 从CF-Connecting-IP获取IP: %s", clientIP)
		return clientIP
	}

	// 最后从 RemoteAddr 获取
	if ip, _, err := net.SplitHostPort(c.Request.RemoteAddr); err == nil {
		log.Printf("[DEBUG] 从RemoteAddr获取IP: %s", ip)
		return ip
	}

	clientIP := c.ClientIP()
	log.Printf("[DEBUG] 从ClientIP()获取IP: %s", clientIP)
	return clientIP
}

// 修改验证处理函数，确保剩余次数为0时也正确返回
func verifyHandler(c *gin.Context) {
	log.Printf("[DEBUG] 验证接口被调用: %s %s", c.Request.Method, c.Request.URL.Path)

	var req VerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[WARN] 验证请求格式错误: %v", err)
		c.JSON(http.StatusBadRequest, VerifyResponse{
			Success: false,
			Message: "请求格式错误: " + err.Error(),
		})
		return
	}

	log.Printf("[DEBUG] 解析请求成功，Token长度: %d", len(req.Token))

	// 获取请求者真实IP
	clientIP := getRealIP(c)
	log.Printf("[INFO] 收到验证请求: 客户端IP=%s", clientIP)

	// 验证IP是否为有效的公网IP
	if !isValidPublicIP(clientIP) {
		log.Printf("[WARN] 客户端IP无效或为内网IP: %s", clientIP)
		c.JSON(http.StatusBadRequest, VerifyResponse{
			Success: false,
			Message: "无法获取有效的公网IP",
		})
		return
	}

	// 验证Token格式
	if _, err := hex.DecodeString(req.Token); err != nil {
		log.Printf("[WARN] Token格式无效: %v", err)
		c.JSON(http.StatusBadRequest, VerifyResponse{
			Success: false,
			Message: "Token格式无效",
		})
		return
	}

	// 解密和验证Token
	payload, matchedRecord, err := decryptAndValidateToken(req.Token, clientIP)
	if err != nil {
		log.Printf("[WARN] Token验证失败: %v", err)
		c.JSON(http.StatusUnauthorized, VerifyResponse{
			Success: false,
			Message: "Token无效或IP不匹配",
		})
		return
	}

	log.Printf("[INFO] Token验证成功: 用户ID=%s, IP匹配", payload.UserID)

	// 检查用户剩余次数
	if matchedRecord.Limit <= 0 {
		log.Printf("[WARN] 用户 %s 次数不足，剩余: %d", matchedRecord.UserID, matchedRecord.Limit)
		c.JSON(http.StatusForbidden, VerifyResponse{
			Success: false,
			Message: "使用次数不足",
			UserID:  matchedRecord.UserID,
			Limit:   0, // 明确设置为0，而不是使用matchedRecord.Limit
		})
		return
	}

	// 验证成功，扣除一次使用次数
	err = updateUserLimit(matchedRecord.UserID, -1)
	if err != nil {
		log.Printf("[ERROR] 更新用户次数失败: %v", err)
		c.JSON(http.StatusInternalServerError, VerifyResponse{
			Success: false,
			Message: "系统错误",
		})
		return
	}

	// 计算扣费后的剩余次数
	newLimit := matchedRecord.Limit - 1

	log.Printf("[INFO] 验证完全成功: 用户=%s, 解密IP=%s, 请求IP=%s, 剩余次数=%d",
		matchedRecord.UserID, payload.IP, clientIP, newLimit)

	c.JSON(http.StatusOK, VerifyResponse{
		Success: true,
		Message: "验证成功",
		UserID:  matchedRecord.UserID,
		Limit:   newLimit,
	})
}

// 新的解密和验证函数
func decryptAndValidateToken(tokenHex string, clientIP string) (*Payload, *UserRecord, error) {
	// 解码十六进制
	data, err := hex.DecodeString(tokenHex)
	if err != nil {
		return nil, nil, fmt.Errorf("十六进制解码失败: %v", err)
	}

	if len(data) < 21 { // timestamp(8) + userID_len(1) + userID(>=1) + nonce(12)
		return nil, nil, fmt.Errorf("token太短")
	}

	// 解析Token结构: [timestamp(8)] + [userID_len(1)] + [userID] + [nonce(12)] + [ciphertext]
	timestamp := int64(binary.BigEndian.Uint64(data[0:8]))
	userIDLen := int(data[8])

	if len(data) < 9+userIDLen+12 {
		return nil, nil, fmt.Errorf("token格式无效")
	}

	userID := string(data[9 : 9+userIDLen])
	nonce := data[9+userIDLen : 9+userIDLen+12]
	ciphertext := data[9+userIDLen+12:]

	log.Printf("[DEBUG] 从Token解析: 用户ID=%s, 时间戳=%d", userID, timestamp)

	// 使用解析出的用户ID和时间戳生成密钥
	key, err := generateDeterministicKey(userID, timestamp)
	if err != nil {
		return nil, nil, fmt.Errorf("生成密钥失败: %v", err)
	}

	// 解密数据
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("创建密码块失败: %v", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("创建GCM失败: %v", err)
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("解密失败: %v", err)
	}

	var payload Payload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, nil, fmt.Errorf("解析Payload失败: %v", err)
	}

	log.Printf("[DEBUG] 解密成功: 用户ID=%s, IP=%s, 时间戳=%d", payload.UserID, payload.IP, payload.Timestamp)

	// 验证IP是否匹配
	if payload.IP != clientIP {
		return nil, nil, fmt.Errorf("IP不匹配: Token中IP=%s, 请求IP=%s", payload.IP, clientIP)
	}

	// 从数据库获取用户记录（用于检查剩余次数）
	db, err := loadDatabase()
	if err != nil {
		return nil, nil, fmt.Errorf("加载数据库失败: %v", err)
	}

	for _, record := range db.Records {
		if record.UserID == userID && record.Timestamp == timestamp {
			log.Printf("[DEBUG] 找到匹配的数据库记录")
			return &payload, &record, nil
		}
	}

	return nil, nil, fmt.Errorf("数据库中未找到匹配的记录")
}

// 生成确定性密钥（基于用户ID和时间戳）
func generateDeterministicKey(userID string, timestamp int64) ([]byte, error) {
	// 使用用户ID和时间戳生成确定性密钥
	data := fmt.Sprintf("%s_%d", userID, timestamp)

	hash := md5.Sum([]byte(data))
	// 扩展到32字节
	key := make([]byte, 32)
	copy(key, hash[:])
	copy(key[16:], hash[:])

	return key, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// 生成卡密（基于时间戳和管理员ID的MD5）
func generateKey(adminID int64) string {
	now := time.Now().In(chinaLocation)
	timestamp := now.Unix()

	// 组合时间戳和管理员ID
	data := fmt.Sprintf("%d_%d", timestamp, adminID)

	// 计算MD5
	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:])
}

// 检查是否为管理员
func isAdmin(userID int64) bool {
	for _, adminID := range config.Bot.AdminIDs {
		if adminID == userID {
			return true
		}
	}
	return false
}

// 创建主菜单键盘（移除取消按钮）
func createMainMenuKeyboard(userID int64) tgbotapi.InlineKeyboardMarkup {
	var keyboard [][]tgbotapi.InlineKeyboardButton

	// 普通用户按钮
	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🐳 获取Token", "get_token"),
		tgbotapi.NewInlineKeyboardButtonData("🛳️ 账户信息", "account_info"),
	))

	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("💻 使用卡密", "use_key"),
		tgbotapi.NewInlineKeyboardButtonData("💰 充值次数", "recharge"),
	))

	keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔥 换绑IP", "change_ip"),
	))

	// 管理员按钮
	if isAdmin(userID) {
		keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🛠️ 管理员功能", "admin_menu"),
		))
	}

	return tgbotapi.NewInlineKeyboardMarkup(keyboard...)
}

// 创建管理员菜单键盘（移除取消按钮）
func createAdminMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	keyboard := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🎉 生成卡密", "gen_key"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
		),
	}

	return tgbotapi.NewInlineKeyboardMarkup(keyboard...)
}

// 创建确认键盘（移除取消按钮）
func createConfirmKeyboard(action string) tgbotapi.InlineKeyboardMarkup {
	keyboard := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ 确认", "confirm_"+action),
		),
	}

	return tgbotapi.NewInlineKeyboardMarkup(keyboard...)
}

// 设置消息超时
func setMessageTimeout(bot *tgbotapi.BotAPI, userID int64, chatID int64, messageID int) {
	// 清除之前的超时
	clearMessageTimeout(userID, chatID, messageID)

	timeoutKey := fmt.Sprintf("%d_%d_%d", userID, chatID, messageID)

	timer := time.AfterFunc(5*time.Minute, func() {
		// 5分钟后删除消息
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
		bot.Request(deleteMsg)

		// 清理状态
		clearUserState(userID)
		delete(messageTimeouts, timeoutKey)

		log.Printf("[INFO] 用户 %d 的消息 %d 因超时被删除", userID, messageID)
	})

	messageTimeouts[timeoutKey] = &MessageTimeout{
		UserID:    userID,
		ChatID:    chatID,
		MessageID: messageID,
		Timer:     timer,
	}
}

// 清除消息超时
func clearMessageTimeout(userID int64, chatID int64, messageID int) {
	timeoutKey := fmt.Sprintf("%d_%d_%d", userID, chatID, messageID)
	if timeout, exists := messageTimeouts[timeoutKey]; exists {
		timeout.Timer.Stop()
		delete(messageTimeouts, timeoutKey)
	}
}

// 重置消息超时（用户有操作时调用）
func resetMessageTimeout(bot *tgbotapi.BotAPI, userID int64, chatID int64, messageID int) {
	setMessageTimeout(bot, userID, chatID, messageID)
}

// 设置用户状态（更新为包含消息ID）
func setUserState(userID int64, state string, data map[string]interface{}, messageID int) {
	if data == nil {
		data = make(map[string]interface{})
	}

	// 清除旧的定时器
	if oldState := userStates[userID]; oldState != nil && oldState.Timer != nil {
		oldState.Timer.Stop()
	}

	userStates[userID] = &UserState{
		State:     state,
		Data:      data,
		MessageID: messageID,
	}
}

// 清除用户状态（更新）
func clearUserState(userID int64) {
	if state := userStates[userID]; state != nil && state.Timer != nil {
		state.Timer.Stop()
	}
	delete(userStates, userID)
	delete(userKeys, userID)
}

// 处理用户状态输入
func handleUserStateInput(bot *tgbotapi.BotAPI, userID int64, chatID int64, text string, userState *UserState, userMessageID int) {
	// 删除用户的输入消息
	deleteMsg := tgbotapi.NewDeleteMessage(chatID, userMessageID)
	bot.Request(deleteMsg)

	// 重置消息超时
	resetMessageTimeout(bot, userID, chatID, userState.MessageID)

	switch userState.State {
	case "waiting_ip":
		handleIPInput(bot, userID, chatID, text)
	case "waiting_key":
		handleKeyInput(bot, userID, chatID, text)
	case "waiting_key_limit":
		handleKeyLimitInput(bot, userID, chatID, text)
	case "waiting_recharge_count":
		handleRechargeCountInput(bot, userID, chatID, text)
	case "waiting_change_ip":
		handleChangeIPInput(bot, userID, chatID, text)
	}
}

// 获取用户状态
func getUserState(userID int64) *UserState {
	return userStates[userID]
}

// 处理IP输入
func handleIPInput(bot *tgbotapi.BotAPI, userID int64, chatID int64, ip string) {
	userState := getUserState(userID)
	if userState == nil {
		return
	}

	messageID := userState.MessageID

	if isValidPublicIP(ip) {
		ipUsed, _, err := ipExists(ip)
		if err != nil {
			log.Printf("[ERROR] 检查IP存在性失败: %v", err)
			editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 系统错误，请稍后再试")
			keyboard := createMainMenuKeyboard(userID)
			editMsg.ReplyMarkup = &keyboard
			bot.Send(editMsg)
			clearUserState(userID)
			return
		}

		if ipUsed {
			clearUserState(userID)
			editMsg := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf("❌ IP地址 %s 已被其他用户绑定！\n\n请使用其他IP地址。", ip))
			keyboard := createMainMenuKeyboard(userID)
			editMsg.ReplyMarkup = &keyboard
			bot.Send(editMsg)
			return
		}

		// 生成Token
		generateTokenForUser(bot, userID, chatID, ip, messageID)

	} else if net.ParseIP(ip) != nil && isPrivateIP(ip) {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 禁止使用局域网地址！\n\n📥 请重新输入你的公网 IP 地址：")
		keyboard := [][]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
			),
		}
		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
		bot.Send(editMsg)

	} else {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 输入的不是有效的 IP 地址格式！\n\n📥 请重新输入你的公网 IP 地址：")
		keyboard := [][]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
			),
		}
		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
		bot.Send(editMsg)
	}
}

// 处理卡密输入
func handleKeyInput(bot *tgbotapi.BotAPI, userID int64, chatID int64, key string) {
	userState := getUserState(userID)
	if userState == nil {
		return
	}

	messageID := userState.MessageID

	userInfo, err := getUserInfo(fmt.Sprintf("%d", userID))
	if err != nil {
		log.Printf("[ERROR] 获取用户信息失败: %v", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 系统错误，请稍后再试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	if userInfo == nil {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 你还没有获取过 Token\n\n💡 请先获取 Token")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	addLimit, err := useKey(key, fmt.Sprintf("%d", userID))
	if err != nil {
		log.Printf("[WARN] 用户 %d 使用卡密失败: %v", userID, err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf("❌ %s\n\n🎉 请重新输入你的卡密：", err.Error()))
		keyboard := [][]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
			),
		}
		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
		bot.Send(editMsg)
		return
	}

	err = updateUserLimit(fmt.Sprintf("%d", userID), addLimit)
	if err != nil {
		log.Printf("[ERROR] 更新用户次数失败: %v", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 系统错误，请稍后再试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	newLimit := userInfo.Limit + addLimit
	msgText := fmt.Sprintf("✅ 卡密使用成功！\n\n⚡ 增加次数: %d\n💫 当前总次数: %d", addLimit, newLimit)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, msgText)
	keyboard := createMainMenuKeyboard(userID)
	editMsg.ReplyMarkup = &keyboard
	bot.Send(editMsg)
	clearUserState(userID)
	log.Printf("[INFO] 用户 %d 使用卡密成功: %s, 增加次数: %d", userID, key, addLimit)
}

// 处理卡密次数输入
func handleKeyLimitInput(bot *tgbotapi.BotAPI, userID int64, chatID int64, text string) {
	userState := getUserState(userID)
	if userState == nil {
		return
	}

	messageID := userState.MessageID

	limit, err := strconv.Atoi(text)
	if err != nil || limit <= 0 {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 请输入有效的正整数\n\n🎉 生成卡密\n\n请输入卡密可增加的次数：")
		keyboard := [][]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔙 返回管理员菜单", "admin_menu"),
			),
		}
		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
		bot.Send(editMsg)
		return
	}

	userState.Data["limit"] = limit

	confirmMsg := fmt.Sprintf("📋 确认生成卡密信息：\n\n⚡ 次数: %d\n\n确认生成吗？", limit)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, confirmMsg)
	keyboard := createConfirmKeyboard("gen_key")
	editMsg.ReplyMarkup = &keyboard
	bot.Send(editMsg)
}

// 处理充值次数输入
func handleRechargeCountInput(bot *tgbotapi.BotAPI, userID int64, chatID int64, text string) {
	userState := getUserState(userID)
	if userState == nil {
		return
	}

	messageID := userState.MessageID

	count, err := strconv.Atoi(text)
	if err != nil || count <= 0 {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 请输入有效的整数\n\n💰 请输入要充值的次数：\n\n💡 按次计费：每次 0.1 ¥")
		keyboard := [][]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
			),
		}
		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
		bot.Send(editMsg)
		return
	}

	totalPrice := float64(count) * config.Payment.PricePerUse

	userState.Data["count"] = count
	userState.Data["price"] = totalPrice

	confirmMsg := fmt.Sprintf("📋 确认充值信息：\n\n⚡ 次数: %d\n💰 金额: %.2f 元\n💳 支付方式: 微信支付\n\n确认创建订单吗？", count, totalPrice)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, confirmMsg)
	keyboard := createConfirmKeyboard("recharge")
	editMsg.ReplyMarkup = &keyboard
	bot.Send(editMsg)
}

// 处理充值按钮
func handleRechargeButton(bot *tgbotapi.BotAPI, userID int64, chatID int64, messageID int) {
	userInfo, err := getUserInfo(fmt.Sprintf("%d", userID))
	if err != nil {
		log.Printf("[ERROR] 获取用户信息失败: %v", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 系统错误，请稍后再试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		return
	}

	if userInfo == nil {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 你还没有获取过 Token\n\n💡 请先获取你的专属 Token")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		return
	}

	setUserState(userID, "waiting_recharge_count", make(map[string]interface{}), messageID)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf("💰 请输入要充值的次数：\n\n💡 每次 %.2f 元\n🎯 当前剩余次数: %d", config.Payment.PricePerUse, userInfo.Limit))
	keyboard := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
		),
	}
	editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
	bot.Send(editMsg)
}

// 处理确认充值
func handleConfirmRecharge(bot *tgbotapi.BotAPI, userID int64, chatID int64, messageID int) {
	userState := getUserState(userID)
	if userState == nil || userState.Data["count"] == nil || userState.Data["price"] == nil {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 操作超时，请重新开始")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	count := userState.Data["count"].(int)
	price := userState.Data["price"].(float64)

	payID := fmt.Sprintf("RECHARGE_%d_%d", userID, time.Now().UnixNano())

	order := &Order{
		PayID:      payID,
		UserID:     fmt.Sprintf("%d", userID),
		Count:      count,
		GoodsName:  fmt.Sprintf("充值%d次使用次数", count),
		Price:      price,
		Status:     "pending",
		CreateTime: time.Now(),
		ChatID:     chatID,
		MessageID:  messageID,
	}

	err := saveOrderToDB(order)
	if err != nil {
		log.Printf("[ERROR] 保存订单失败: %v", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 创建订单失败，请稍后再试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	req := &CreateOrderRequest{
		PayID:     payID,
		Type:      1, // 微信支付
		Price:     price,
		GoodsName: order.GoodsName,
		Param:     fmt.Sprintf("%d", userID),
		IsHTML:    0,
		NotifyURL: config.Payment.NotifyURL,
		ReturnURL: config.Payment.ReturnURL,
	}

	result, err := epayClient.CreateOrder(req)
	if err != nil {
		log.Printf("[ERROR] 创建订单失败: %v", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 创建订单失败，请稍后再试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	if result.Code != 1 {
		log.Printf("[ERROR] 创建订单失败: %s", result.Msg)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf("❌ 创建订单失败: %s", result.Msg))
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	// 保存易支付订单号到数据库
	err = updateOrderWithEpayInfo(payID, result.Data.OrderID, result.Data.ReallyPrice, result.Data.PayType)
	if err != nil {
		log.Printf("[ERROR] 更新订单信息失败: %v", err)
	}

	msgText := fmt.Sprintf("🎉 订单创建成功！\n\n"+
		"📦 商品: %s\n"+
		"💰 金额: %.2f 元\n"+
		"📋 订单号: %s\n\n"+
		"🔗 请点击下方链接完成支付：\n%s\n\n"+
		"⏰ 订单有效期: %d 分钟\n"+
		"💡 支付完成后次数将自动到账",
		order.GoodsName,
		result.Data.Price,
		result.Data.OrderID,
		result.Data.PayURL,
		result.Data.TimeOut)

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, msgText)
	keyboard := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("💳 去支付", result.Data.PayURL),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 查询订单状态", "check_order_"+result.Data.OrderID),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
		),
	}
	editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
	bot.Send(editMsg)

	clearUserState(userID)
	log.Printf("[INFO] 用户 %d 创建充值订单: %s, 次数: %d, 金额: %.2f", userID, payID, count, price)
}

// 处理查询订单状态
func handleCheckOrderStatus(bot *tgbotapi.BotAPI, userID int64, chatID int64, messageID int, orderID string) {
	result, err := epayClient.GetOrder(orderID)
	if err != nil {
		log.Printf("[ERROR] 查询订单失败: %v", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 查询订单失败，请稍后再试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		return
	}

	if result.Code != 1 {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf("❌ 查询订单失败: %s", result.Msg))
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		return
	}

	var statusText string
	var keyboard [][]tgbotapi.InlineKeyboardButton

	order, err := getOrderByPayID(result.Data.PayID)
	if err == nil && order != nil && order.Status == "paid" {
		statusText = "✅ 已支付完成"
		keyboard = [][]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
			),
		}
	} else {
		switch result.Data.State {
		case 0:
			statusText = "⏳ 等待支付"
			keyboard = [][]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonURL("💳 去支付", result.Data.PayURL),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔄 刷新状态", "check_order_"+orderID),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
				),
			}
		case 1:
			statusText = "✅ 已支付完成"
			keyboard = [][]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
				),
			}
		case 2:
			statusText = "❌ 支付失败"
			keyboard = [][]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
				),
			}
		default:
			statusText = "❓ 未知状态"
			keyboard = [][]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔄 刷新状态", "check_order_"+orderID),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
				),
			}
		}
	}

	msgText := fmt.Sprintf("📋 订单状态查询\n\n"+
		"📦 商品: %s\n"+
		"💰 金额: %.2f 元\n"+
		"📋 订单号: %s\n"+
		"📊 状态: %s",
		result.Data.PayID,
		result.Data.Price,
		result.Data.OrderID,
		statusText)

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, msgText)
	editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
	bot.Send(editMsg)
}

// 生成Token
func generateTokenForUser(bot *tgbotapi.BotAPI, userID int64, chatID int64, ip string, messageID int) {
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)

	deterministicKey, err := generateDeterministicKey(fmt.Sprintf("%d", userID), timestamp)
	if err != nil {
		log.Printf("[ERROR] 生成确定性密钥失败: %v", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 生成 Token 出错，请重试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	payload := Payload{
		UserID:    fmt.Sprintf("%d", userID),
		IP:        ip,
		Timestamp: timestamp,
	}

	token, err := encryptPayload(payload, deterministicKey)
	if err != nil {
		log.Printf("[ERROR] 为用户 %d 生成 Token 失败: %v", userID, err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 生成 Token 出错，请重试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	err = addUserRecord(fmt.Sprintf("%d", userID), ip, token, config.Limits.DefaultLimit, timestamp)
	if err != nil {
		log.Printf("[ERROR] 保存用户记录失败: %v", err)
	}

	result := fmt.Sprintf("🎉 你的 Token 生成成功！\n\n```\n%s\n```\n\n📌 请妥善保存，用于身份验证\n⚡ 初始额度: %d 次\n\n💡 使用账户信息按钮查看详情", token, config.Limits.DefaultLimit)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, result)
	editMsg.ParseMode = "Markdown"
	keyboard := createMainMenuKeyboard(userID)
	editMsg.ReplyMarkup = &keyboard
	bot.Send(editMsg)

	clearUserState(userID)
	log.Printf("[INFO] ✅ 为用户 %d 生成 Token 成功，IP: %s, 时间戳: %d", userID, ip, timestamp)
}

// 处理回调查询
func handleCallbackQuery(bot *tgbotapi.BotAPI, query *tgbotapi.CallbackQuery) {
	userID := query.From.ID
	chatID := query.Message.Chat.ID
	messageID := query.Message.MessageID
	data := query.Data

	resetMessageTimeout(bot, userID, chatID, messageID)

	callback := tgbotapi.NewCallback(query.ID, "")
	bot.Request(callback)

	log.Printf("[INFO] 用户 %d 点击按钮: %s", userID, data)

	switch {
	case data == "main_menu":
		clearUserState(userID)
		welcomeMsg := "🎉 欢迎使用 Token 验证系统！\n\n请选择你需要的功能："
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, welcomeMsg)
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)

	case data == "get_token":
		handleGetTokenButton(bot, userID, chatID, messageID)

	case data == "account_info":
		handleAccountInfoButton(bot, userID, chatID, messageID)

	case data == "use_key":
		handleUseKeyButton(bot, userID, chatID, messageID)

	case data == "recharge":
		handleRechargeButton(bot, userID, chatID, messageID)

	case data == "change_ip":
		handleChangeIPButton(bot, userID, chatID, messageID)

	case data == "confirm_recharge":
		handleConfirmRecharge(bot, userID, chatID, messageID)

	case data == "confirm_change_ip":
		handleConfirmChangeIP(bot, userID, chatID, messageID)

	case strings.HasPrefix(data, "check_order_"):
		orderID := strings.TrimPrefix(data, "check_order_")
		handleCheckOrderStatus(bot, userID, chatID, messageID, orderID)

	case data == "admin_menu":
		if !isAdmin(userID) {
			editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 你没有管理员权限")
			keyboard := createMainMenuKeyboard(userID)
			editMsg.ReplyMarkup = &keyboard
			bot.Send(editMsg)
			return
		}
		adminMsg := "🛠️ 管理员功能面板\n\n请选择操作："
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, adminMsg)
		keyboard := createAdminMenuKeyboard()
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)

	case data == "gen_key":
		handleGenKeyButton(bot, userID, chatID, messageID)

	case data == "confirm_gen_key":
		handleConfirmGenKey(bot, userID, chatID, messageID)

	default:
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 未知操作")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
	}
}

// 处理获取Token按钮
func handleGetTokenButton(bot *tgbotapi.BotAPI, userID int64, chatID int64, messageID int) {
	exists, err := userExists(fmt.Sprintf("%d", userID))
	if err != nil {
		log.Printf("[ERROR] 检查用户存在性失败: %v", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 系统错误，请稍后再试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		return
	}

	if exists {
		log.Printf("[WARN] 用户 %d 尝试重复获取token", userID)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 你已经获取过 Token，每个用户只能获取一次\n\n💡 使用账户信息按钮查看你的信息")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		return
	}

	key, err := generateAESKey()
	if err != nil {
		log.Printf("[ERROR] 为用户 %d 生成密钥失败: %v", userID, err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 无法生成密钥，请稍后再试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		return
	}

	userKeys[userID] = key
	setUserState(userID, "waiting_ip", nil, messageID)

	log.Printf("[INFO] 为用户 %d 生成密钥成功", userID)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "🧩 已为你生成 AES 密钥\n\n📥 请输入你的公网 IP 地址以生成专属 Token：")
	keyboard := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
		),
	}
	editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
	bot.Send(editMsg)
}

// 处理账户信息按钮
func handleAccountInfoButton(bot *tgbotapi.BotAPI, userID int64, chatID int64, messageID int) {
	userInfo, err := getUserInfo(fmt.Sprintf("%d", userID))
	if err != nil {
		log.Printf("[ERROR] 获取用户信息失败: %v", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 系统错误，请稍后再试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		return
	}

	if userInfo == nil {
		log.Printf("[INFO] 用户 %d 查询信息但未注册", userID)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 你还没有获取过 Token\n\n💡 请先获取你的专属 Token")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		return
	}

	infoMsg := fmt.Sprintf("🌸 你的账户信息：\n\n"+
		"💭 用户ID: %s\n"+
		"🌐 绑定IP: %s\n"+
		"⚡ 剩余次数: %d\n"+
		"📅 创建时间: %s\n\n"+
		"👑 Token: ```\n%s\n```",
		userInfo.UserID,
		userInfo.IP,
		userInfo.Limit,
		userInfo.CreatedAt,
		userInfo.Token)

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, infoMsg)
	editMsg.ParseMode = "Markdown"
	keyboard := createMainMenuKeyboard(userID)
	editMsg.ReplyMarkup = &keyboard
	bot.Send(editMsg)
	log.Printf("[INFO] 用户 %d 查询账户信息成功", userID)
}

// 处理使用卡密按钮
func handleUseKeyButton(bot *tgbotapi.BotAPI, userID int64, chatID int64, messageID int) {
	userInfo, err := getUserInfo(fmt.Sprintf("%d", userID))
	if err != nil {
		log.Printf("[ERROR] 获取用户信息失败: %v", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 系统错误，请稍后再试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		return
	}

	if userInfo == nil {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 你还没有获取过 Token\n\n💡 请先获取你的专属 Token")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		return
	}

	setUserState(userID, "waiting_key", nil, messageID)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "🎉 请输入你的卡密：")
	keyboard := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
		),
	}
	editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
	bot.Send(editMsg)
}

// 处理生成卡密按钮
func handleGenKeyButton(bot *tgbotapi.BotAPI, userID int64, chatID int64, messageID int) {
	if !isAdmin(userID) {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 你没有管理员权限")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		return
	}

	setUserState(userID, "waiting_key_limit", nil, messageID)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf("🎉 生成卡密\n\n请输入卡密可增加的次数：\n\n💡 默认次数: %d", config.Limits.KeyAddLimit))
	keyboard := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 返回管理员菜单", "admin_menu"),
		),
	}
	editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
	bot.Send(editMsg)
}

// 处理确认生成卡密
func handleConfirmGenKey(bot *tgbotapi.BotAPI, userID int64, chatID int64, messageID int) {
	userState := getUserState(userID)
	if userState == nil || userState.Data["limit"] == nil {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 操作超时，请重新开始")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	addLimit := userState.Data["limit"].(int)

	key, err := addKey(addLimit, userID)
	if err != nil {
		log.Printf("[ERROR] 生成卡密失败: %v", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 生成卡密失败")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	msgText := fmt.Sprintf("🎉 卡密生成成功：\n\n```\n%s\n```\n\n⚡ 可增加次数: %d\n\n📌 请妥善保存此卡密", key, addLimit)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, msgText)
	editMsg.ParseMode = "Markdown"
	keyboard := createMainMenuKeyboard(userID)
	editMsg.ReplyMarkup = &keyboard
	bot.Send(editMsg)
	clearUserState(userID)
	log.Printf("[INFO] 管理员 %d 生成卡密: %s, 次数: %d", userID, key, addLimit)
}

// returnHandler 同步回调处理器
func returnHandler(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, `
		<!DOCTYPE html>
		<html>
		<head>
			<meta charset="utf-8">
			<title>支付成功</title>
			<meta name="viewport" content="width=device-width, initial-scale=1.0">
			<style>
				body { font-family: Arial, sans-serif; text-align: center; padding: 50px; background: #f5f5f5; }
				.container { background: white; padding: 30px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); max-width: 400px; margin: 0 auto; }
				.success-icon { font-size: 60px; color: #4CAF50; margin-bottom: 20px; }
				h1 { color: #333; margin-bottom: 10px; }
				p { color: #666; line-height: 1.6; }
			</style>
		</head>
		<body>
			<div class="container">
				<div class="success-icon">✅</div>
				<h1>支付成功！</h1>
				<p>感谢您的购买，使用次数已自动到账。</p>
				<p>请返回 Telegram 查看您的账户信息。</p>
			</div>
		</body>
		</html>
	`)
}

// NewEpayClient 创建新的易支付客户端
func NewEpayClient(baseURL, mchID, secret string) *EpayClient {
	return &EpayClient{
		BaseURL: baseURL,
		MchID:   mchID,
		Secret:  secret,
	}
}

// generateMD5 生成MD5哈希
func generateMD5(data string) string {
	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:])
}

// generateSign 生成签名
func (c *EpayClient) generateSign(params map[string]interface{}) string {
	// 排序参数
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}

	// 对键进行排序
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	// 构建签名字符串
	var signData string
	for _, key := range keys {
		if params[key] != nil && params[key] != "" {
			signData += fmt.Sprintf("%s=%v&", key, params[key])
		}
	}
	signData += c.Secret

	return generateMD5(signData)
}

// CreateOrder 创建订单
func (c *EpayClient) CreateOrder(req *CreateOrderRequest) (*CreateOrderResponse, error) {
	// 设置商户ID
	req.MchID = c.MchID

	// 根据易支付API文档，签名计算方式为: md5(payId+param+type+price+通讯密钥)
	signString := fmt.Sprintf("%s%s%d%.2f%s", req.PayID, req.Param, req.Type, req.Price, c.Secret)
	req.Sign = generateMD5(signString)

	log.Printf("[DEBUG] 易支付签名字符串: %s", signString)
	log.Printf("[DEBUG] 易支付签名结果: %s", req.Sign)

	// 构建请求体
	data := url.Values{}
	data.Set("mchId", req.MchID)
	data.Set("payId", req.PayID)
	data.Set("type", fmt.Sprintf("%d", req.Type))
	data.Set("price", fmt.Sprintf("%.2f", req.Price))
	data.Set("goodsName", req.GoodsName)
	data.Set("param", req.Param)
	data.Set("isHtml", fmt.Sprintf("%d", req.IsHTML))
	data.Set("notifyUrl", req.NotifyURL)
	data.Set("returnUrl", req.ReturnURL)
	data.Set("sign", req.Sign)

	log.Printf("[DEBUG] 易支付请求URL: %s", c.BaseURL+"/api/createOrder")
	log.Printf("[DEBUG] 易支付请求参数: %s", data.Encode())

	// 发送请求到正确的API端点
	resp, err := http.PostForm(c.BaseURL+"/api/createOrder", data)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	log.Printf("[DEBUG] 易支付响应状态: %d", resp.StatusCode)
	log.Printf("[DEBUG] 易支付响应内容: %s", string(body))

	// 检查响应是否为HTML（错误页面）
	if strings.Contains(string(body), "<html>") || strings.Contains(string(body), "<!DOCTYPE") {
		return nil, fmt.Errorf("API返回HTML页面而非JSON，可能是请求URL或参数错误")
	}

	var result CreateOrderResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v, 响应内容: %s", err, string(body))
	}

	return &result, nil
}

// GetOrder 查询订单
func (c *EpayClient) GetOrder(orderID string) (*GetOrderResponse, error) {
	data := url.Values{}
	data.Set("mchId", c.MchID)
	data.Set("orderId", orderID)

	log.Printf("[DEBUG] 查询订单请求URL: %s", c.BaseURL+"/api/getOrder")
	log.Printf("[DEBUG] 查询订单请求参数: %s", data.Encode())

	resp, err := http.PostForm(c.BaseURL+"/api/getOrder", data)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	log.Printf("[DEBUG] 查询订单响应状态: %d", resp.StatusCode)
	log.Printf("[DEBUG] 查询订单响应内容: %s", string(body))

	var result GetOrderResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v, 响应内容: %s", err, string(body))
	}

	return &result, nil
}

// 支付成功通知函数（修改以支持换绑IP）
func notifyPaymentSuccess(order *Order, reallyPrice float64, payType int) {
	go func() {
		// 解析用户ID
		userIDInt, err := strconv.ParseInt(order.UserID, 10, 64)
		if err != nil {
			log.Printf("[ERROR] 解析用户ID失败: %v", err)
			return
		}

		// 创建临时Bot实例
		if config.Bot.Token == "" {
			log.Printf("[ERROR] Bot Token未配置，无法发送通知")
			return
		}

		bot, err := tgbotapi.NewBotAPI(config.Bot.Token)
		if err != nil {
			log.Printf("[ERROR] 创建Bot实例失败: %v", err)
			return
		}

		// 获取用户信息
		userInfo, err := getUserInfo(order.UserID)
		if err != nil {
			log.Printf("[ERROR] 获取用户信息失败: %v", err)
			return
		}

		// 格式化支付方式
		payTypeStr := "未知"
		switch payType {
		case 1:
			payTypeStr = "微信支付"
		case 2:
			payTypeStr = "支付宝"
		}

		var message string

		// 检查是否是换绑IP订单
		if strings.HasPrefix(order.PayID, "CHANGE_IP_") {
			message = fmt.Sprintf("🔥 换绑IP成功通知\n\n"+
				"🎁 服务名称: %s\n"+
				"💵 支付金额: %.2f 元\n"+
				"💳 支付方式: %s\n"+
				"📦 订单号: %s\n"+
				"🌐 新IP地址: %s\n\n"+
				"✅ IP换绑成功，新Token已生成！\n"+
				"💡 请使用账户信息查看新Token\n\n"+
				"感谢您的使用！",
				order.GoodsName, reallyPrice, payTypeStr, order.PayID, userInfo.IP)
		} else {
			// 普通充值订单
			message = fmt.Sprintf("💰 支付成功通知\n\n"+
				"🎁 商品名称: %s\n"+
				"💵 支付金额: %.2f 元\n"+
				"💳 支付方式: %s\n"+
				"📦 订单号: %s\n"+
				"✅ 增加次数: %d\n"+
				"🔢 当前总次数: %d\n\n"+
				"感谢您的购买！",
				order.GoodsName, reallyPrice, payTypeStr, order.PayID, order.Count, userInfo.Limit)
		}

		msg := tgbotapi.NewMessage(userIDInt, message)
		_, err = bot.Send(msg)
		if err != nil {
			log.Printf("[ERROR] 发送支付成功通知失败: %v", err)
		} else {
			log.Printf("[INFO] 支付成功通知已发送给用户 %s", order.UserID)
		}
	}()
}

// notifyHandler 异步回调处理器
func notifyHandler(c *gin.Context) {
	log.Printf("[INFO] 收到支付回调通知，方法: %s", c.Request.Method)

	// 获取所有参数
	params := make(map[string]string)

	// GET参数（优先，因为易支付使用GET请求）
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}

	// POST参数（备用）
	for key, values := range c.Request.PostForm {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}

	log.Printf("[DEBUG] 回调参数: %+v", params)

	// 验证必要参数
	requiredParams := []string{"mchId", "orderId", "type", "price", "reallyPrice", "sign"}
	for _, param := range requiredParams {
		if _, exists := params[param]; !exists {
			log.Printf("[WARN] 缺少必要参数: %s", param)
			c.String(http.StatusBadRequest, "fail")
			return
		}
	}

	// 验证商户ID
	if params["mchId"] != config.Payment.MchID {
		log.Printf("[WARN] 商户ID不匹配: 期望 %s, 收到 %s", config.Payment.MchID, params["mchId"])
		c.String(http.StatusBadRequest, "fail")
		return
	}

	// 验证签名
	receivedSign := params["sign"]

	// 根据易支付API文档，回调签名计算方式为: md5(orderId + param + type + price + reallyPrice + 通讯密钥)
	signString := fmt.Sprintf("%s%s%s%s%s%s",
		params["orderId"],
		params["param"],
		params["type"],
		params["price"],
		params["reallyPrice"],
		config.Payment.Secret)
	calculatedSign := generateMD5(signString)

	log.Printf("[DEBUG] 回调签名字符串: %s", signString)
	log.Printf("[DEBUG] 计算的签名: %s, 收到的签名: %s", calculatedSign, receivedSign)

	if receivedSign != calculatedSign {
		log.Printf("[WARN] 签名验证失败: 期望 %s, 收到 %s", calculatedSign, receivedSign)
		c.String(http.StatusBadRequest, "fail")
		return
	}

	// 获取订单信息
	var order *Order
	var err error

	// 解析param参数，可能包含用户ID或用户ID|新IP
	paramParts := strings.Split(params["param"], "|")
	userID := paramParts[0]

	// 首先尝试通过param（用户ID）查找最近的未支付订单
	if userID != "" {
		order, err = getLatestPendingOrderByUserID(userID)
		if err != nil {
			log.Printf("[ERROR] 通过用户ID查询最新待支付订单失败: %v", err)
		}
	}

	// 如果通过用户ID找不到，尝试通过orderId查找
	if order == nil {
		epayOrderID := params["orderId"]
		order, err = getOrderByEpayOrderID(epayOrderID)
		if err != nil {
			log.Printf("[ERROR] 通过orderId查询订单失败: %v", err)
		}
	}

	if order == nil {
		log.Printf("[WARN] 订单不存在: param=%s, orderId=%s", params["param"], params["orderId"])
		c.String(http.StatusNotFound, "fail")
		return
	}

	log.Printf("[INFO] 找到订单: PayID=%s, UserID=%s, Status=%s", order.PayID, order.UserID, order.Status)

	if order.Status == "paid" {
		log.Printf("[INFO] 订单已处理过: %s", order.PayID)
		c.String(http.StatusOK, "success")
		return
	}

	// 解析金额和支付类型
	reallyPrice, _ := strconv.ParseFloat(params["reallyPrice"], 64)
	payType, _ := strconv.Atoi(params["type"])

	// 更新订单状态
	err = updateOrderStatus(order.PayID, "paid", reallyPrice, payType)
	if err != nil {
		log.Printf("[ERROR] 更新订单状态失败: %v", err)
		c.String(http.StatusInternalServerError, "fail")
		return
	}

	// 检查是否是换绑IP订单
	if strings.HasPrefix(order.PayID, "CHANGE_IP_") && len(paramParts) > 1 {
		// 处理换绑IP
		newIP := paramParts[1]
		err = handleChangeIPSuccess(order, newIP)
		if err != nil {
			log.Printf("[ERROR] 处理换绑IP失败: %v", err)
			c.String(http.StatusInternalServerError, "fail")
			return
		}
		log.Printf("[INFO] 换绑IP成功处理完成: 用户 %s, 订单 %s, 新IP %s",
			order.UserID, order.PayID, newIP)
	} else {
		// 普通充值订单，更新用户次数
		err = updateUserLimit(order.UserID, order.Count)
		if err != nil {
			log.Printf("[ERROR] 更新用户次数失败: %v", err)
			c.String(http.StatusInternalServerError, "fail")
			return
		}
		log.Printf("[INFO] 充值成功处理完成: 用户 %s, 订单 %s, 增加次数 %d",
			order.UserID, order.PayID, order.Count)
	}

	// 发送支付成功通知
	notifyPaymentSuccess(order, reallyPrice, payType)

	// 删除支付消息
	if order.ChatID != 0 && order.MessageID != 0 {
		bot, err := tgbotapi.NewBotAPI(config.Bot.Token)
		if err != nil {
			log.Printf("[ERROR] 创建Bot实例失败: %v", err)
		} else {
			deleteMsg := tgbotapi.NewDeleteMessage(order.ChatID, order.MessageID)
			_, err = bot.Request(deleteMsg)
			if err != nil {
				log.Printf("[ERROR] 删除支付消息失败: %v", err)
			} else {
				log.Printf("[INFO] 支付消息已删除: ChatID=%d, MessageID=%d", order.ChatID, order.MessageID)
			}
		}
	}

	c.String(http.StatusOK, "success")
}

func main() {
	// 初始化随机数种子
	rand.Seed(time.Now().UnixNano())

	err := loadConfig()
	if err != nil {
		log.Fatal("[FATAL] 加载配置失败:", err)
	}

	// 初始化MySQL数据库
	err = initDatabase()
	if err != nil {
		log.Fatal("[FATAL] 初始化数据库失败:", err)
	}
	defer db.Close()

	// 初始化易支付客户端
	if config.Payment.BaseURL != "" && config.Payment.MchID != "" && config.Payment.Secret != "" {
		epayClient = NewEpayClient(config.Payment.BaseURL, config.Payment.MchID, config.Payment.Secret)
		log.Printf("[INFO] 易支付客户端初始化成功: %s", config.Payment.MchID)
	} else {
		log.Printf("[WARN] 支付配置不完整，支付功能不可用")
	}

	log.Printf("[DEBUG] 准备启动HTTP服务器，配置端口: %d", config.Server.Port)

	// 设置Gin为发布模式（可选）
	gin.SetMode(gin.ReleaseMode)

	r := gin.Default()

	// 添加中间件记录所有请求
	r.Use(func(c *gin.Context) {
		log.Printf("[DEBUG] 收到请求: %s %s from %s", c.Request.Method, c.Request.URL.Path, c.ClientIP())
		c.Next()
	})

	// 添加根路径处理，确认服务正常
	r.GET("/", func(c *gin.Context) {
		log.Printf("[DEBUG] 根路径被访问")
		c.JSON(http.StatusOK, gin.H{
			"status":    "running",
			"message":   "Bot API Server",
			"endpoints": []string{"/verify", "/notify", "/return"},
		})
	})

	// 健康检查端点
	r.GET("/health", func(c *gin.Context) {
		log.Printf("[DEBUG] 健康检查被访问")
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.POST("/verify", verifyHandler)

	// 支付相关端点
	if epayClient != nil {
		r.POST("/notify", notifyHandler)
		r.GET("/notify", notifyHandler) // 添加GET方法支持
		r.GET("/return", returnHandler)
		log.Printf("[INFO] 支付回调端点已注册: /notify (GET/POST), /return")
	}

	// 添加404处理
	r.NoRoute(func(c *gin.Context) {
		log.Printf("[WARN] 访问了不存在的路径: %s %s", c.Request.Method, c.Request.URL.Path)
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "接口不存在",
			"path":    c.Request.URL.Path,
		})
	})

	// 启动HTTP服务器
	address := fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port)
	log.Printf("[INFO] HTTP服务器启动: %s", address)

	go func() {
		log.Printf("[DEBUG] 开始监听端口...")
		if err := r.Run(address); err != nil {
			log.Fatal("[FATAL] HTTP服务器启动失败:", err)
		}
	}()

	// 给HTTP服务器一点启动时间
	time.Sleep(1 * time.Second)
	log.Printf("[DEBUG] HTTP服务器应该已经启动完成")

	// 从配置文件读取 Bot Token
	if config.Bot.Token == "" || config.Bot.Token == "YOUR_BOT_TOKEN_HERE" {
		log.Fatal("[FATAL] 请在config.toml中设置正确的bot.token")
	}

	bot, err := tgbotapi.NewBotAPI(config.Bot.Token)
	if err != nil {
		log.Fatal("[FATAL] Bot初始化失败:", err)
	}

	bot.Debug = false
	log.Printf("[INFO] Bot启动成功: @%s", bot.Self.UserName)

	// Telegram Bot处理
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		// 处理普通消息
		if update.Message != nil {
			userID := update.Message.From.ID
			chatID := update.Message.Chat.ID
			messageID := update.Message.MessageID
			text := strings.TrimSpace(update.Message.Text)
			username := update.Message.From.UserName

			log.Printf("[INFO] 收到用户 %d (@%s) 的消息: %s", userID, username, text)

			// 检查用户状态
			userState := getUserState(userID)

			if text == "/help" {
				// 删除用户的/help消息
				deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
				bot.Request(deleteMsg)

				clearUserState(userID)
				welcomeMsg := "🎉 欢迎使用 Token 验证系统！\n\n" +
					"请选择你需要的功能："

				msg := tgbotapi.NewMessage(chatID, welcomeMsg)
				msg.ReplyMarkup = createMainMenuKeyboard(userID)
				sentMsg, err := bot.Send(msg)
				if err == nil {
					// 设置消息超时
					setMessageTimeout(bot, userID, chatID, sentMsg.MessageID)
				}
				log.Printf("[INFO] 用户 %d 使用了help命令", userID)

			} else if userState != nil {
				// 处理用户状态相关的输入（传递用户消息ID用于删除）
				handleUserStateInput(bot, userID, chatID, text, userState, messageID)
			} else {
				// 删除用户消息
				deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
				bot.Request(deleteMsg)

				// 没有状态时，提示用户使用按钮
				msg := tgbotapi.NewMessage(chatID, "请使用按钮进行操作，或发送 /help 查看主菜单")
				msg.ReplyMarkup = createMainMenuKeyboard(userID)
				sentMsg, err := bot.Send(msg)
				if err == nil {
					setMessageTimeout(bot, userID, chatID, sentMsg.MessageID)
				}
			}
		}

		// 处理回调查询（按钮点击）
		if update.CallbackQuery != nil {
			handleCallbackQuery(bot, update.CallbackQuery)
		}
	}
}

// 更新用户IP和Token - MySQL版本
func updateUserIPAndToken(userID, newIP, newToken string, timestamp int64) error {
	query := "UPDATE users SET ip = ?, token = ?, timestamp = ?, updated_at = ? WHERE user_id = ?"
	result, err := db.Exec(query, newIP, newToken, timestamp, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("更新用户IP和Token失败: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %v", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}

	log.Printf("[INFO] 用户 %s IP和Token已更新: %s", userID, newIP)
	return nil
}

// 处理换绑IP输入
func handleChangeIPInput(bot *tgbotapi.BotAPI, userID int64, chatID int64, newIP string) {
	userState := getUserState(userID)
	if userState == nil {
		return
	}

	messageID := userState.MessageID

	if isValidPublicIP(newIP) {
		// 检查新IP是否已被其他用户使用
		ipUsed, existingUserID, err := ipExists(newIP)
		if err != nil {
			log.Printf("[ERROR] 检查IP存在性失败: %v", err)
			editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 系统错误，请稍后再试")
			keyboard := createMainMenuKeyboard(userID)
			editMsg.ReplyMarkup = &keyboard
			bot.Send(editMsg)
			clearUserState(userID)
			return
		}

		// 检查是否是用户自己当前的IP
		currentUserID := fmt.Sprintf("%d", userID)
		if ipUsed && existingUserID != currentUserID {
			clearUserState(userID)
			editMsg := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf("❌ IP地址 %s 已被其他用户绑定！\n\n请使用其他IP地址。", newIP))
			keyboard := createMainMenuKeyboard(userID)
			editMsg.ReplyMarkup = &keyboard
			bot.Send(editMsg)
			return
		}

		// 如果是用户当前的IP，提示无需换绑
		if ipUsed && existingUserID == currentUserID {
			clearUserState(userID)
			editMsg := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf("❌ %s 就是你当前绑定的IP地址！\n\n无需重复换绑。", newIP))
			keyboard := createMainMenuKeyboard(userID)
			editMsg.ReplyMarkup = &keyboard
			bot.Send(editMsg)
			return
		}

		// IP可用，显示确认信息
		price := 1.0 // 换绑IP费用1元
		userState.Data["new_ip"] = newIP
		userState.Data["price"] = price

		confirmMsg := fmt.Sprintf("📋 确认换绑IP信息：\n\n🌐 新IP地址: %s\n💰 换绑费用: %.2f 元\n💳 支付方式: 微信支付\n\n⚠️ 换绑后将生成新的Token，旧Token将失效\n\n确认创建订单吗？", newIP, price)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, confirmMsg)
		keyboard := createConfirmKeyboard("change_ip")
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)

	} else if net.ParseIP(newIP) != nil && isPrivateIP(newIP) {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 禁止使用局域网地址！\n\n📥 请重新输入你的新公网 IP 地址：")
		keyboard := [][]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
			),
		}
		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
		bot.Send(editMsg)

	} else {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 输入的不是有效的 IP 地址格式！\n\n📥 请重新输入你的新公网 IP 地址：")
		keyboard := [][]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
			),
		}
		editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
		bot.Send(editMsg)
	}
}

// 处理换绑IP按钮
func handleChangeIPButton(bot *tgbotapi.BotAPI, userID int64, chatID int64, messageID int) {
	log.Printf("[DEBUG] handleChangeIPButton被调用: 用户 %d", userID)

	userInfo, err := getUserInfo(fmt.Sprintf("%d", userID))
	if err != nil {
		log.Printf("[ERROR] 获取用户信息失败: %v", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 系统错误，请稍后再试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		return
	}

	if userInfo == nil {
		log.Printf("[DEBUG] 用户 %d 还没有获取过Token", userID)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 你还没有获取过 Token\n\n💡 请先获取你的专属 Token")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		return
	}

	log.Printf("[DEBUG] 用户 %d 开始换绑IP流程，当前IP: %s", userID, userInfo.IP)
	setUserState(userID, "waiting_change_ip", nil, messageID)
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf("🔥 换绑IP地址\n\n🌐 当前绑定IP: %s\n💰 换绑费用: 1.00 元\n\n📥 请输入你的新公网 IP 地址：", userInfo.IP))
	keyboard := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
		),
	}
	editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
	bot.Send(editMsg)
}

// 处理确认换绑IP
func handleConfirmChangeIP(bot *tgbotapi.BotAPI, userID int64, chatID int64, messageID int) {
	userState := getUserState(userID)
	if userState == nil || userState.Data["new_ip"] == nil || userState.Data["price"] == nil {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 操作超时，请重新开始")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	newIP := userState.Data["new_ip"].(string)
	price := userState.Data["price"].(float64)

	payID := fmt.Sprintf("CHANGE_IP_%d_%d", userID, time.Now().UnixNano())

	order := &Order{
		PayID:      payID,
		UserID:     fmt.Sprintf("%d", userID),
		Count:      0, // 换绑IP不涉及次数
		GoodsName:  fmt.Sprintf("换绑IP地址到%s", newIP),
		Price:      price,
		Status:     "pending",
		CreateTime: time.Now(),
		ChatID:     chatID,
		MessageID:  messageID,
	}

	err := saveOrderToDB(order)
	if err != nil {
		log.Printf("[ERROR] 保存换绑IP订单失败: %v", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 创建订单失败，请稍后再试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	req := &CreateOrderRequest{
		PayID:     payID,
		Type:      1, // 微信支付
		Price:     price,
		GoodsName: order.GoodsName,
		Param:     fmt.Sprintf("%d|%s", userID, newIP), // 传递用户ID和新IP
		IsHTML:    0,
		NotifyURL: config.Payment.NotifyURL,
		ReturnURL: config.Payment.ReturnURL,
	}

	result, err := epayClient.CreateOrder(req)
	if err != nil {
		log.Printf("[ERROR] 创建换绑IP订单失败: %v", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "❌ 创建订单失败，请稍后再试")
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	if result.Code != 1 {
		log.Printf("[ERROR] 创建换绑IP订单失败: %s", result.Msg)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf("❌ 创建订单失败: %s", result.Msg))
		keyboard := createMainMenuKeyboard(userID)
		editMsg.ReplyMarkup = &keyboard
		bot.Send(editMsg)
		clearUserState(userID)
		return
	}

	// 保存易支付订单号到数据库
	err = updateOrderWithEpayInfo(payID, result.Data.OrderID, result.Data.ReallyPrice, result.Data.PayType)
	if err != nil {
		log.Printf("[ERROR] 更新换绑IP订单信息失败: %v", err)
	}

	msgText := fmt.Sprintf("🎉 换绑IP订单创建成功！\n\n"+
		"📦 服务: %s\n"+
		"💰 费用: %.2f 元\n"+
		"📋 订单号: %s\n\n"+
		"🔗 请点击下方链接完成支付：\n%s\n\n"+
		"⏰ 订单有效期: %d 分钟\n"+
		"💡 支付完成后将自动更新IP并生成新Token",
		order.GoodsName,
		result.Data.Price,
		result.Data.OrderID,
		result.Data.PayURL,
		result.Data.TimeOut)

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, msgText)
	keyboard := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("💳 去支付", result.Data.PayURL),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 查询订单状态", "check_order_"+result.Data.OrderID),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 返回主菜单", "main_menu"),
		),
	}
	editMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboard}
	bot.Send(editMsg)

	clearUserState(userID)
	log.Printf("[INFO] 用户 %d 创建换绑IP订单: %s, 新IP: %s, 金额: %.2f", userID, payID, newIP, price)
}

// 处理换绑IP成功后的Token生成
func handleChangeIPSuccess(order *Order, newIP string) error {
	userID := order.UserID

	// 生成新的时间戳和Token
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)

	deterministicKey, err := generateDeterministicKey(userID, timestamp)
	if err != nil {
		return fmt.Errorf("生成确定性密钥失败: %v", err)
	}

	payload := Payload{
		UserID:    userID,
		IP:        newIP,
		Timestamp: timestamp,
	}

	newToken, err := encryptPayload(payload, deterministicKey)
	if err != nil {
		return fmt.Errorf("生成新Token失败: %v", err)
	}

	// 更新数据库中的IP和Token
	err = updateUserIPAndToken(userID, newIP, newToken, timestamp)
	if err != nil {
		return fmt.Errorf("更新数据库失败: %v", err)
	}

	log.Printf("[INFO] 用户 %s 换绑IP成功: %s -> %s, 新Token已生成", userID, order.GoodsName, newIP)
	return nil
}
