package domain

import "time"

type Side int8

const (
	Buy  Side = 1
	Sell Side = 2
)

type OrderStatus int8

const (
	OrderStatusPending  OrderStatus = 0
	OrderStatuspartial  OrderStatus = 1 //部分成交
	OrderStatusFilled   OrderStatus = 2 //全部成交
	OrderStatusCanceled OrderStatus = 3
)

type Order struct {
	ID        int64
	AccountID int64
	Symbol    string // 交易标的
	Side      Side   // 1: 买入 (Buy), 2: 卖出 (Sell)
	Price     int64  // 委托价格 (为了防止精度丢失，这里存放大 10000 倍的整数)
	Quantity  int64  // 委托总数量
	FilledQty int64  // 已成交数量 (要根据它计算剩余数量)
	Status    OrderStatus
	CreatedAt time.Time
}

// 获取该订单还剩多少没成交
func (o *Order) UnfilledQty() int64 {
	return o.Quantity - o.FilledQty
}

// 判断订单是否已经结束生命周期
func (o *Order) IsCompleted() bool {
	return o.Status == OrderStatusFilled || o.Status == OrderStatusCanceled
}
