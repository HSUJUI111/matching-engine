package orderbook

import (
	"container/list"
	"matching-engine/internal/domain"
	"matching-engine/internal/repo"
	"sync"
	"time"

	"github.com/emirpasic/gods/trees/redblacktree"
)

// 价格挡位
// 所以每个价格节点上，挂着一个“订单队列” (先进先出 FIFO)
type PriceLevel struct {
	Price int64
	Order *list.List
}

type orderEntry struct {
	element    *list.Element
	priceLevel *PriceLevel
	tree       *redblacktree.Tree
}
type OrderBook struct {
	Symbol string
	// Bids 买盘：愿意买入的订单。出价最高的人排在最前面！(降序)
	Bids *redblacktree.Tree

	// Asks 卖盘：愿意卖出的订单。叫价最低的人排在最前面！(升序)
	Asks *redblacktree.Tree
	// 保护红黑树并发读写的读写锁
	mu sync.RWMutex

	orderIndex    map[int64]*orderEntry //订单ID到订单位置的索引，方便快速更新和删除
	TradeCh       chan *domain.Trade    //将成交记录传给记账协程
	OrderUpdateCh chan *domain.Order    //更新订单管道
}

type PriceLevelSnapshot struct {
	Price    int64 `json:"price"`
	Quantity int64 `json:"quantity"` //这个价位上所有订单的总剩余数量
}
type OrderBookSnapshot struct {
	Symbol string                `json:"symbol"`
	Bids   []*PriceLevelSnapshot `json:"bids"` // 买盘
	Asks   []*PriceLevelSnapshot `json:"asks"` // 卖盘
}

func NewOrderBook(symbol string) *OrderBook {
	ob := &OrderBook{
		Symbol: symbol,
		Bids: redblacktree.NewWith(func(a, b interface{}) int {
			p1 := a.(int64)
			p2 := b.(int64)
			if p1 > p2 {
				return -1
			} else if p1 < p2 {
				return 1
			}
			return 0
		}),
		Asks: redblacktree.NewWith(func(a, b interface{}) int {
			p1 := a.(int64)
			p2 := b.(int64)
			if p1 > p2 {
				return -1 // p1 小，排前面
			} else if p1 < p2 {
				return 1
			}
			return 0
		}),
		orderIndex:    make(map[int64]*orderEntry),
		TradeCh:       make(chan *domain.Trade, 10000),
		OrderUpdateCh: make(chan *domain.Order, 10000),
	}
	//协程永远在后台盯着管道
	go ob.asyncTradeSaver()
	go ob.asyncOrderUpdater()
	return ob
}

// 记账协程
func (ob *OrderBook) asyncTradeSaver() {
	// range Channel 的特性：只要管道不关，这个循环就永远不会停，一直在等新数据
	for trade := range ob.TradeCh {
		// 收到数据，调用 repo 写入数据库
		repo.SaveTrade(trade)
	}
}
func (ob *OrderBook) asyncOrderUpdater() {
	for order := range ob.OrderUpdateCh {
		repo.SaveOrUpdateOrder(order)
	}
}

// 不带锁
func (ob *OrderBook) add(order *domain.Order) {
	var tree *redblacktree.Tree
	if order.Side == domain.Buy {
		tree = ob.Bids
	} else {
		tree = ob.Asks
	}

	//查找该价位是否已经存在
	node, ok := tree.Get(order.Price)
	var pl *PriceLevel
	if ok {
		pl = node.(*PriceLevel)
		pl.Order.PushBack(order)
	} else {
		pl = &PriceLevel{Price: order.Price, Order: list.New()}
		pl.Order.PushBack(order)
		tree.Put(order.Price, pl)
	}

	//注册到索引
	elem := pl.Order.Back() //刚才新加的订单，一定在队列的最后面
	ob.orderIndex[order.ID] = &orderEntry{
		element:    elem,
		priceLevel: pl,
		tree:       tree,
	}
}

func (ob *OrderBook) AddOrder(order *domain.Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	ob.add(order)
}
func (ob *OrderBook) Match(takerOrder *domain.Order) []*domain.Trade {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	var trades []*domain.Trade
	var makerTree *redblacktree.Tree
	if takerOrder.Side == domain.Buy {
		makerTree = ob.Asks // Taker是买，去找卖盘 (卖盘树的最左边是最低价)
	} else {
		makerTree = ob.Bids // Taker是卖，去找买盘 (买盘树的最左边是最高价)
	}
	it := makerTree.Iterator()
	it.Begin()

	for it.Next() && takerOrder.UnfilledQty() > 0 {
		makerPrice := it.Key().(int64)

		if takerOrder.Side == domain.Buy && takerOrder.Price < makerPrice {
			break
		}
		if takerOrder.Side == domain.Sell && takerOrder.Price > makerPrice {
			break
		}

		//价格匹配成功
		priceLevel := it.Value().(*PriceLevel)
		queue := priceLevel.Order

		//遍历价格下所有排队订单
		var nextElement *list.Element
		for element := queue.Front(); element != nil; element = nextElement {
			nextElement = element.Next()
			makerOrder := element.Value.(*domain.Order)

			//1.决定本次成交数量 (取 Taker剩余需要 和 Maker剩余提供 的最小值)
			takerUnfilled := takerOrder.UnfilledQty()
			makerUnfilled := makerOrder.UnfilledQty()

			var tradeQty int64
			if takerUnfilled < makerUnfilled {
				tradeQty = takerUnfilled //情况一，T>M
			} else {
				tradeQty = makerUnfilled //情况二,M>T
			}
			//2.更新已成交数量
			takerOrder.FilledQty += tradeQty
			makerOrder.FilledQty += tradeQty

			//3.创建成交记录
			trade := &domain.Trade{
				Symbol:       takerOrder.Symbol,
				MakerOrderID: makerOrder.ID,
				TakerOrderID: takerOrder.ID,
				Price:        makerPrice,
				Quantity:     tradeQty,
				TradeAt:      time.Now(),
			}

			trades = append(trades, trade)

			//把生成的这笔成交记录扔到传送带上，立刻回头处理下一个！
			ob.TradeCh <- trade
			ob.OrderUpdateCh <- makerOrder

			//情况一,把Maker提出队列
			if makerOrder.UnfilledQty() == 0 {
				makerOrder.Status = domain.OrderStatusFilled
				queue.Remove(element)
				delete(ob.orderIndex, makerOrder.ID) // ★ 成交完移除索引
				// makerOrder.Status = 2
				// queue.Remove(element)
			} else {
				makerOrder.Status = 1
			}

			//情况二
			if takerOrder.UnfilledQty() == 0 {
				takerOrder.Status = 2
				break
			} else {
				takerOrder.Status = 1
			}
		}
		// 6. 拔树根！(这也是面试必考点：内存泄漏防护)
		// 如果刚才的一波吃单，把这个价格档位里的所有人全吃光了，队列空了
		// 我们必须把这个价格节点从红黑树上彻底删掉，否则下次搜索会搜到空节点！
		if queue.Len() == 0 {
			makerTree.Remove(makerPrice)
		}
	}
	// 如果循环结束了，Taker 单子还没吃饱，剩下的部分就变成 Maker 单，挂到自己的树上排队
	if takerOrder.UnfilledQty() > 0 {
		ob.add(takerOrder)
		// ob.AddOrder(takerOrder)
	}
	return trades
}

func (ob *OrderBook) CancelOrder(orderID int64) (*domain.Order, bool) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	entry, exists := ob.orderIndex[orderID]
	if !exists {
		//不在内存里：已全部成交or订单ID根本不存在
		return nil, false
	}
	order := entry.element.Value.(*domain.Order)
	entry.priceLevel.Order.Remove(entry.element)
	delete(ob.orderIndex, orderID)

	//价位队列空了就拔树根，防止内存泄漏
	if entry.priceLevel.Order.Len() == 0 {
		entry.tree.Remove(entry.priceLevel.Price)
	}
	order.Status = domain.OrderStatusCanceled
	ob.OrderUpdateCh <- order //通知数据库更新订单状态
	return order, true
}

// GetSnapshot 获取盘口快照，depth 表示买卖各取几档
func (ob *OrderBook) GetSnapshot(depth int) *OrderBookSnapshot {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	snapshot := &OrderBookSnapshot{
		Symbol: ob.Symbol,
		Bids:   make([]*PriceLevelSnapshot, 0, depth),
		Asks:   make([]*PriceLevelSnapshot, 0, depth),
	}

	bidIt := ob.Bids.Iterator()
	bidIt.Begin()
	for bidIt.Next() && len(snapshot.Bids) < depth {
		pl := bidIt.Value().(*PriceLevel)
		snapshot.Bids = append(snapshot.Bids, &PriceLevelSnapshot{
			Price:    pl.Price,
			Quantity: sumUnfilled(pl),
		})
	}

	askIt := ob.Asks.Iterator()
	askIt.Begin()
	for askIt.Next() && len(snapshot.Asks) < depth {
		pl := askIt.Value().(*PriceLevel)
		snapshot.Asks = append(snapshot.Asks, &PriceLevelSnapshot{
			Price:    pl.Price,
			Quantity: sumUnfilled(pl),
		})
	}

	return snapshot
}

func sumUnfilled(pl *PriceLevel) int64 {
	var total int64
	for e := pl.Order.Front(); e != nil; e = e.Next() {
		order := e.Value.(*domain.Order)
		total += order.UnfilledQty()
	}
	return total
}
