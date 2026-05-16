package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config 是整个应用的配置总线
type Config struct {
	App      AppConfig      `mapstructure:"app"`
	Postgres PostgresConfig `mapstructure:"postgres"`
	Kafka    KafkaConfig    `mapstructure:"kafka"`
	Engine   EngineConfig   `mapstructure:"engine"`
}

type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
}

type PostgresConfig struct {
	DSN          string `mapstructure:"dsn"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
}

type KafkaConfig struct {
	Brokers     []string `mapstructure:"brokers"`
	TopicOrders string   `mapstructure:"topic_orders"`
	TopicTrades string   `mapstructure:"topic_trades"`
}

type EngineConfig struct {
	Symbols []string `mapstructure:"symbols"`
}

// Load 读取配置文件并融合环境变量
func Load(path string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// 兜底默认值
	v.SetDefault("app.env", "dev")
	v.SetDefault("postgres.max_idle_conns", 10)
	v.SetDefault("postgres.max_open_conns", 100)

	// 开启环境变量覆盖 (前缀为 ME_，例如 ME_POSTGRES_DSN)
	v.SetEnvPrefix("ME")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置到结构体失败: %w", err)
	}

	return &cfg, nil
}
