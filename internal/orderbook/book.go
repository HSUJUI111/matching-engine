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

type OrderBook struct {
	Symbol string
	// Bids 买盘：愿意买入的订单。出价最高的人排在最前面！(降序)
	Bids *redblacktree.Tree

	// Asks 卖盘：愿意卖出的订单。叫价最低的人排在最前面！(升序)
	Asks *redblacktree.Tree
	// 保护红黑树并发读写的读写锁
	mu sync.RWMutex

	TradeCh       chan *domain.Trade //将成交记录传给记账协程
	OrderUpdateCh chan *domain.Order //更新订单管道
}

func NewOrderBook(symbol string) *OrderBook {
	ob := &OrderBook{
		Symbol: symbol,
		Bids: redblacktree.NewWith(func(a, b interface{}) int {
			p1 := a.(int64)
			p2 := a.(int64)
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
			if p1 < p2 {
				return -1 // p1 小，排前面
			} else if p1 > p2 {
				return 1
			}
			return 0
		}),

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
	// ob.mu.Lock()
	// defer ob.mu.Unlock()

	var tree *redblacktree.Tree
	if order.Side == domain.Buy {
		tree = ob.Bids
	} else {
		tree = ob.Asks
	}

	//查找该价位是否已经存在
	node, ok := tree.Get(order.Price)
	if ok {
		// 存在，追加到该价格队列的尾部（先进先出原则）
		priceLevel := node.(*PriceLevel)
		priceLevel.Order.PushBack(order)
	} else {
		//不存在创建新挡位和新队列
		priceLevel := &PriceLevel{
			Price: order.Price,
			Order: list.New(),
		}
		priceLevel.Order.PushBack(order)
		tree.Put(order.Price, priceLevel)
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
				makerOrder.Status = 2
				queue.Remove(element)
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
