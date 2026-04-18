# 🔥 极速分布式金融撮合引擎 (High-Concurrency Matching Engine)

[![Golang](https://img.shields.io/badge/Language-Go%201.20+-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![Kafka](https://img.shields.io/badge/Queue-Apache%20Kafka-231F20?style=flat-square&logo=apachekafka)](https://kafka.apache.org/)
[![PostgreSQL](https://img.shields.io/badge/Database-PostgreSQL-336791?style=flat-square&logo=postgresql)](https://www.postgresql.org/)

本项是一个基于 **Golang** 语言开发的分布式、高性能、异步金融资产撮合引擎核心。它不仅是一个高并发分布式系统的实践案例，更是对底层算法（红黑树 $O(\log N)$）和架构设计模式（事件驱动、异步落盘、削峰填谷）的深度演进。

---

## 🚀 性能压测报告 (Performance Report)

> **环境：** 本地单机部署 (PostgreSQL 15, Kafka 7.4.4, Engine-Service)
> **工具：** `hey` 工具对 HTTP API 接口进行压力测试
> **场景：** 并发数: 200, 总请求数: 10,000

| 核心指标 | 测试结果 | 业务意义 |
| :--- | :--- | :--- |
| **Requests/sec (TPS)** | **3121.63** | **每秒可受理并接收并在内存中撮合的订单数量。极高吞吐量。** |
| **API 平均响应延迟** | **34.1 ms** | **用户“下单 -> 收到受理成功回执”的体感等待时间。达到无感级极速响应。** |
| **成功率 (Success Rate)** | **100% (Status 200)** | **在高并发冲击下，系统运行平稳，无任何请求丢失或宕机。** |

---

## 🌟 技术难点与架构亮点 (Technical Highlights) - 【面试官爱看这里】

项目拒绝成为简单的 CRUD，在架构设计上解决了以下经典的分布式高并发痛点：

### 1. $O(\log N)$ 的极速内存撮合算法
抛弃传统关系型数据库撮合（$O(N)$ 级 I/O 阻塞）。核心撮合逻辑完全基于 Go 原生 **红黑树 (Red-Black Tree)** 实现。买卖盘深度查找、插入、删除操作的时间复杂度均为 **$O(\log N)$**，达到纳秒/微秒级撮合。

### 2. Kafka 削峰填谷与系统解耦 (V2.0 架构演进)
* **痛点：** V1.0 架构使用纯 HTTP 同步调用，遇到突发流量 API 接口会瞬间卡死（被 DB I/O 堵塞）。
* **对策：** 引入 **Apache Kafka** 消息队列。API 收单后瞬间投递至 Kafka（削峰），核心引擎通过专门的 Consumer 异步拿单处理（解耦）。彻底解决了 API 的读写瓶颈，使 API 的 TPS 提升了数倍。

### 3. 金融级高精度定点数方案 (`int64` 放大万倍)
* **痛点：** 浮点数 (`float64`) 在二进制计算中存在天生的精度丢失，在金融系统中绝对不被允许。
* **对策：** 统一使用 **`int64` (长整型)** 表示金额与数量。所有金额统一**放大 10,000 倍**存储（即：800.00 元表示为 8,000,000 分）。计算时完全采用整数运算，杜绝了任何精度丢失。

### 4. 彻底解耦的“异步持久化”策略 (`TradeCh` & `OrderUpdateCh`)
核心撮合协程决不执行耗时的 DB 操作。我们利用 Go 语言原生的 **Channel (管道)** 构建了一条“异步落盘传送带”。
撮合产生的成交记录 (`Trade`) 和订单最新状态 (`Order`) 分别被丢入专门的 Channel。后台独立的持久化协程异步、从容地将其保存至 PostgreSQL，确保了核心引擎的高吞吐量。

📂 项目目录结构 (Project Structure)
cmd/engine/main.go：项目入口，负责各个微服务的初始化与启动。

internal/orderbook/book.go：核心逻辑！ 红黑树撮合算法 (Match) 的具体实现。

internal/kafka/：负责 Kafka 生产者 (producer.go) 与消费者 (consumer.go) 的高可用连接与处理。

internal/api/router.go：基于 Gin 框架开辟的 HTTP API 接口路由。

🛠️ 如何在本地运行 (Getting Started)
基建依赖 (使用 Docker 一键拉起)
项目根目录执行：

Bash
docker-compose up -d
# 等待约 20 秒，给 Kafka 和 Postgres 充分的开机时间
启动撮合引擎服务
进入 cmd/engine 目录执行：

Bash
go mod download
go run main.go
尝试下单与查询
挂一个卖单： POST http://localhost:8081/api/order，Body: {"symbol":"2330.TW","side":2,"price":8000000,"quantity":100}

查订单状态： GET http://localhost:8081/api/order/刚才下单拿到的OrderID

挂一个买单： POST http://localhost:8081/api/order，Body: {"symbol":"2330.TW","side":1,"price":8000000,"quantity":60}

