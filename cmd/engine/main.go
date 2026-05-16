package main

import (
	"log"

	"matching-engine/internal/api"
	"matching-engine/internal/config"
	"matching-engine/internal/kafka"
	"matching-engine/internal/orderbook"
	"matching-engine/internal/repo"
)

func main() {
	// ==========================================
	// 第一阶段：加载环境与配置 (你的地基)
	// ==========================================
	cfg, err := config.Load("../../config.yaml")
	if err != nil {
		log.Fatalf("启动失败，无法加载配置: %v", err)
	}
	log.Printf("读取配置成功！当前环境: %s, 数据库DSN: %s", cfg.App.Env, cfg.Postgres.DSN)

	repo.InitPostgres(cfg.Postgres)

	// ==========================================
	// 第二阶段：初始化核心业务模块 (引擎)
	// ==========================================
	log.Println("🚀 正在初始化核心撮合引擎...")
	// 在内存里造一本属于特斯拉 (TSLA) 的订单簿
	// tsmcBook := orderbook.NewOrderBook("2330.TW")
	// tslaBook := orderbook.NewOrderBook("TSLA")

	manager := orderbook.NewBookManager()
	for _, sym := range cfg.Engine.Symbols {
		manager.GetOrCreate(sym)
		log.Printf("✅ 已创建订单簿: %s", sym)
	}

	// 4. 【新增】初始化 Kafka 生产者 (面向前端发单)
	kafka.InitProducer(cfg.Kafka.Brokers, cfg.Kafka.TopicOrders)

	// 5. 【新增】启动 Kafka 消费者 (连接引擎和 Kafka)
	kafka.StartConsumer(cfg.Kafka.Brokers, cfg.Kafka.TopicOrders, manager)
	r := api.SetupRouter(manager)

	// 启动 Web 服务器，程序会在这里阻塞运行
	err = r.Run(":8080")
	if err != nil {
		log.Fatalf("服务器异常退出: %v", err)
	}
}
