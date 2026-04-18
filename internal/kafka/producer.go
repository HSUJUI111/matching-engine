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
		Balancer: &kafka.LeastBytes{},   // 负载均衡策略
	}
	log.Println("✅ Kafka 生产者初始化成功！")
}

func SendOrder(order *domain.Order) error {
	orderBytes, err := json.Marshal(order)
	if err != nil {
		return err
	}

	err = writer.WriteMessages(context.Background(),
		kafka.Message{
			Key:   []byte(order.Symbol),
			Value: orderBytes,
		},
	)
	return err
}
