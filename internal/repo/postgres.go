package repo

import (
	"log"
	"matching-engine/internal/config"
	"matching-engine/internal/domain"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type TradeModel struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`
	Symbol       string    `gorm:"type:varchar(20);index"`
	MakerOrderID int64     `gorm:"index"`
	TakerOrderID int64     `gorm:"index"`
	Price        int64     `gorm:"not null"`
	Quantity     int64     `gorm:"not null"`
	TradedAt     time.Time `gorm:"index"`
}

func (TradeModel) TableName() string {
	return "trades"
}

// OrderModel 对应数据库里的 orders 表
type OrderModel struct {
	ID        int64  `gorm:"primaryKey"` // 这里不用自增，直接用我们代码里生成的唯一时间戳ID
	Symbol    string `gorm:"type:varchar(20);index"`
	Side      int8   `gorm:"index"`
	Price     int64
	Quantity  int64
	FilledQty int64
	Status    int8      `gorm:"index"` // 0: Pending, 1: Partial, 2: Filled, 3: Canceled
	CreatedAt time.Time `gorm:"index"`
}

func (OrderModel) TableName() string {
	return "orders"
}

var DB *gorm.DB

func InitPostgres(cfg config.PostgresConfig) {
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("连接 PostgreSQL 失败: %v", err)
	}

	// 配置连接池
	sqlDB, _ := db.DB()
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)

	// 自动建表
	err = db.AutoMigrate(&TradeModel{}, &OrderModel{})
	if err != nil {
		log.Fatalf("自动建表失败: %v", err)
	}

	DB = db
	log.Println("✅ PostgreSQL 连接成功并完成建表！")
}

// 异步保存单条交易记录（无批量插入）
func SaveTrade(trade *domain.Trade) {
	dbTrade := &TradeModel{
		Symbol:       trade.Symbol,
		MakerOrderID: trade.MakerOrderID,
		TakerOrderID: trade.TakerOrderID,
		Price:        trade.Price,
		Quantity:     trade.Quantity,
		TradedAt:     trade.TradeAt,
	}

	if err := DB.Create(dbTrade).Error; err != nil {
		log.Printf("写入交易记录失败: %v", err)
	}
}

// 保存或更新订单
func SaveOrUpdateOrder(order *domain.Order) {
	dbOrder := &OrderModel{
		ID:        order.ID,
		Symbol:    order.Symbol,
		Side:      int8(order.Side),
		Price:     order.Price,
		Quantity:  order.Quantity,
		FilledQty: order.FilledQty,
		Status:    int8(order.Status),
		CreatedAt: order.CreatedAt,
	}

	// Save 方法在 GORM 里：如果 ID 存在就更新，不存在就插入
	if err := DB.Save(dbOrder).Error; err != nil {
		log.Printf("❌ 写入/更新订单记录失败: %v", err)
	}
}

// 根据 ID 查询订单的方法
func GetOrder(id int64) (*OrderModel, error) {
	var order OrderModel
	if err := DB.First(&order, id).Error; err != nil {
		return nil, err
	}
	return &order, nil
}
