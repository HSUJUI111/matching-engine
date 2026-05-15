package kafka

import (
	"context"
	"encoding/json"
	"log"
	"matching-engine/internal/domain"

	"github.com/segmentio/kafka-go"
)

var writer *kafka.Writer

func InitProducer(brokers []string, topic string) {
	writer = &kafka.Writer{
		Addr:     kafka.TCP(brokers...), // Kafka 的地址 (localhost:9092)
		Topic:    topic,                 // 发送到哪个主题 (orders_topic)
		Balancer: &kafka.Hash{},
		// Balancer: &kafka.LeastBytes{},   // 负载均衡策略
	}
	log.Println("✅ Kafka 生产者初始化成功！")
}

func SendOrder(order *domain.Order) error {
	msg := domain.KafkaMessage{
		Type:  domain.MsgTypeNew,
		Order: order,
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return writer.WriteMessages(context.Background(),
		kafka.Message{
			Key:   []byte(order.Symbol), // symbol 作为分区键
			Value: msgBytes,
		},
	)
	// orderBytes, err := json.Marshal(order)
	// if err != nil {
	// 	return err
	// }

	// err = writer.WriteMessages(context.Background(),
	// 	kafka.Message{
	// 		Key:   []byte(order.Symbol),
	// 		Value: orderBytes,
	// 	},
	// )
	// return err
}

// 撤单
func SendCancel(orderID int64, symbol string) error {
	msg := domain.KafkaMessage{
		Type:    domain.MsgTypeCancel,
		OrderID: orderID,
		Symbol:  symbol,
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return writer.WriteMessages(context.Background(),
		kafka.Message{
			Key:   []byte(symbol), // 同symbol路由同一分区，保序
			Value: msgBytes,
		},
	)
}
