# 🔥 高并发分布式金融撮合引擎

<p align="center">
  <img src="https://img.shields.io/badge/Language-Go%201.20+-00ADD8?style=flat-square&logo=go" alt="Go"/>
  <img src="https://img.shields.io/badge/Queue-Apache%20Kafka-231F20?style=flat-square&logo=apachekafka" alt="Kafka"/>
  <img src="https://img.shields.io/badge/Database-PostgreSQL%2015-336791?style=flat-square&logo=postgresql" alt="PostgreSQL"/>
  <img src="https://img.shields.io/badge/Framework-Gin-00ADD8?style=flat-square" alt="Gin"/>
</p>

> 基于 Go 语言实现的分布式高性能异步金融资产撮合引擎。核心撮合逻辑基于**红黑树** $O(\log N)$ 算法，配合 Kafka 事件驱动架构实现削峰解耦，支持金融级高精度整型定点数方案，完整还原真实证券交易所的核心撮合流程。

---

## 📊 性能压测报告

> **环境：** 本地单机（PostgreSQL 15 · Kafka 7.4.4 · Engine Service）  
> **工具：** `hey` HTTP 压测  
> **场景：** 并发数 200，总请求数 10,000

| 指标 | 结果 | 说明 |
|:--|:--|:--|
| **吞吐量 (TPS)** | **3,121.63 req/s** | 每秒受理并完成内存撮合的订单量 |
| **平均响应延迟** | **34.1 ms** | 用户"下单 → 收到受理回执"的体感等待时间 |
| **成功率** | **100%（Status 200）** | 高并发冲击下零丢单、零宕机 |

---

## 🌟 架构亮点

### 1. 红黑树 $O(\log N)$ 内存撮合算法

抛弃传统数据库撮合（$O(N)$ 级 I/O 阻塞）。买卖盘的深度查找、插入、删除均基于 Go 原生红黑树实现，时间复杂度稳定在 $O(\log N)$，达到**纳秒 / 微秒级**撮合速度。

```
价格优先：买盘取最高价，卖盘取最低价
时间优先：同价位按订单到达时间排序（FIFO）
数据结构：红黑树维护买卖两侧盘口，每个价位挂载订单队列
```

### 2. Kafka 削峰填谷与系统解耦（V2.0 架构演进）

V1.0 使用纯 HTTP 同步调用，突发流量下 API 接口被 DB I/O 阻塞卡死。V2.0 引入 Kafka 彻底解耦：

```
用户请求 → API 层瞬时写入 Kafka（削峰）→ 引擎 Consumer 异步消费（解耦）
                                           ↓
                                     撮合 → 异步落盘
```

- API 层不再等待撮合完成，TPS 提升数倍
- 消息持久化，服务重启后订单不丢失
- 分区键为 `hash(symbol)`，同一标的订单严格有序

### 3. 金融级高精度整型定点数方案

浮点数 `float64` 存在天生的二进制精度损失，在金融系统中不可接受。

```
统一使用 int64 存储，所有金额放大 10,000 倍
800.00 元 → 存储为 8,000,000
全程整数运算，彻底杜绝精度丢失
```

### 4. 异步持久化双 Channel 架构

核心撮合协程**绝不阻塞在 DB 写入上**。利用 Go Channel 构建异步落盘传送带：

```
撮合协程
  ├── TradeCh     ──→  成交持久化协程  ──→  PostgreSQL trades 表
  └── OrderUpdateCh ──→  订单更新协程  ──→  PostgreSQL orders 表
```

撮合完成即返回，DB 写入由后台协程从容处理，核心引擎吞吐量不受磁盘 I/O 影响。

---

## 📂 项目结构

```
matching-engine/
├── cmd/
│   └── engine/
│       └── main.go              # 启动入口，各模块初始化与编排
├── internal/
│   ├── orderbook/
│   │   └── book.go              # ★ 核心：红黑树撮合算法实现
│   ├── kafka/
│   │   ├── producer.go          # Kafka 生产者（API 层写入订单）
│   │   └── consumer.go          # Kafka 消费者（引擎异步取单）
│   ├── api/
│   │   └── router.go            # Gin HTTP 路由
│   ├── repo/                    # 数据库读写层
│   └── config/                  # Viper 配置管理
├── migrations/                  # SQL 迁移文件
├── docker-compose.yml           # 一键拉起 PG + Kafka
├── config.yaml                  # 本地开发配置
└── go.mod
```

---

## 🛠️ 快速启动

### 前置依赖

- Go 1.20+
- Docker & Docker Compose

### 1. 启动基础设施

```bash
docker-compose up -d
# 等待约 20 秒，让 Kafka 和 PostgreSQL 完成初始化
```

### 2. 启动撮合引擎

```bash
cd cmd/engine
go mod download
go run main.go
```

服务启动后监听 `http://localhost:8080`。

---

## 📡 API 接口示例

完整演示一笔订单从挂单到部分成交的完整流转。

### 步骤一：挂出卖单（Maker）

```http
POST http://localhost:8080/api/order
Content-Type: application/json

{
    "symbol":   "2330.TW",
    "side":     2,
    "price":    8000000,
    "quantity": 100
}
```

> `price` 单位为分（放大 10,000 倍），`8000000` = `800.00` 元。  
> 响应返回 `order_id`，假设为 `123456789`。

### 步骤二：查询卖单状态（挂单中）

```http
GET http://localhost:8080/api/order/123456789
```

预期响应：`status: 0`（Pending），`filled_qty: 0`。

### 步骤三：买单吃单（Taker）

```http
POST http://localhost:8080/api/order
Content-Type: application/json

{
    "symbol":   "2330.TW",
    "side":     1,
    "price":    8000000,
    "quantity": 60
}
```

引擎在内存中**瞬时完成撮合**，异步生成成交流水并更新订单状态。

### 步骤四：再次查询卖单（部分成交）

```http
GET http://localhost:8080/api/order/123456789
```

预期响应：`status: 1`（Partial），`filled_qty: 60`，数据库已异步写入对应成交记录。

---

## 📋 订单状态说明

| 状态码 | 含义 |
|:--:|:--|
| `0` | Pending — 挂单中，等待撮合 |
| `1` | Partial — 部分成交，剩余挂单 |
| `2` | Filled — 全部成交 |
| `3` | Cancelled — 已撤单 |

---

## 🔧 技术栈

| 层次 | 技术选型 | 用途 |
|:--|:--|:--|
| 语言 | Go 1.20+ | 高并发核心引擎 |
| HTTP 框架 | Gin | REST API 接口 |
| 消息队列 | Apache Kafka (KRaft) | 削峰解耦、订单队列 |
| 主数据库 | PostgreSQL 15 | 订单、成交持久化 |
| 配置管理 | Viper | 多环境配置 + 环境变量覆盖 |
| 数据结构 | 红黑树 | $O(\log N)$ 订单簿 |
