package kafka

import (
	"context"
	"encoding/json"
	"log"
	"matching-engine/internal/domain"
	"matching-engine/internal/orderbook"
	"matching-engine/internal/repo"

	"github.com/segmentio/kafka-go"
)

func StartConsumer(brokers []string, topic string, manager *orderbook.BookManager) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		GroupID: "matching-engine-group", //设置消费者组
		// Partition: 0,
		MinBytes: 10e3,
		MaxBytes: 10e6,
	})
	log.Println("✅ Kafka 消费者已启动，正在后台高速接收订单...")

	//开一个Goroutine让后台永远循环读取
	go func() {
		for {
			//	阻塞等待,知道新订单进来
			m, err := reader.ReadMessage(context.Background())
			if err != nil {
				log.Printf("读取 Kafka 消息失败: %v", err)
				continue
			}

			//改为解析统一消息结构
			var msg domain.KafkaMessage
			if err := json.Unmarshal(m.Value, &msg); err != nil {
				log.Printf("解析 Kafka 消息失败: %v", err)
				continue
			}

			switch msg.Type {
			case domain.MsgTypeNew:
				order := msg.Order
				// 1. 引擎处理前：先把这笔新订单存进数据库 (状态是 Pending 挂单中)
				book := manager.GetOrCreate(order.Symbol)
				repo.SaveOrUpdateOrder(order)
				log.Printf("📥 新订单 [%s] 价格:%d 数量:%d", order.Symbol, order.Price, order.Quantity)
				book.Match(order)
				repo.SaveOrUpdateOrder(order)

			case domain.MsgTypeCancel:
				book := manager.Get(msg.Symbol)
				if book == nil {
					log.Printf("⚠️ 撤单失败：symbol [%s] 不存在", msg.Symbol)
					continue
				}
				log.Printf("🚫 撤单请求 OrderID:%d Symbol:%s", msg.OrderID, msg.Symbol)
				_, ok := book.CancelOrder(msg.OrderID)
				if !ok {
					log.Printf("⚠️ 撤单失败 OrderID:%d 不在内存中（已成交或不存在）", msg.OrderID)
				}
			}
			// //把收到的JSON变回Go订单结构体
			// var order domain.Order
			// if err := json.Unmarshal(m.Value, &order); err != nil {
			// 	log.Printf("解析订单数据失败: %v", err)
			// 	continue
			// }
			// // 1. 引擎处理前：先把这笔新订单存进数据库 (状态是 Pending 挂单中)
			// repo.SaveOrUpdateOrder(&order)
			// log.Printf("📥 从 Kafka 收到并持久化 [%s] 订单...", order.Symbol)

			// //2.核心交接棒：把订单正式交给撮合引擎！
			// log.Printf("📥 从 Kafka 收到 [%s] 订单，价格: %d，数量: %d", order.Symbol, order.Price, order.Quantity)
			// book.Match(&order)

			// // 3. 引擎处理后：因为 Match 内部可能会改变 order 的 Status 和 FilledQty
			// // 所以撮合完之后，我们再把最新的状态更新回数据库！
			// repo.SaveOrUpdateOrder(&order)
		}
	}()
}
