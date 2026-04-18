package domain

import "time"

type Trade struct {
	ID           int64
	Symbol       string
	MakerOrderID int64 // 挂单方订单ID (被动成交，比如早就挂在那里等待的单子)
	TakerOrderID int64 // 吃单方订单ID (主动成交，比如刚刚发进来立刻就成交的单子)
	Price        int64
	Quantity     int64
	TradeAt      time.Time
}
