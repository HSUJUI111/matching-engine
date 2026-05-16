package api

import (
	"net/http"
	"strconv"
	"time"

	"matching-engine/internal/domain"
	"matching-engine/internal/kafka"
	"matching-engine/internal/orderbook"
	"matching-engine/internal/repo"

	"github.com/gin-gonic/gin"
)

// OrderRequest 定义前端传过来的 JSON 数据长什么样
type OrderRequest struct {
	Symbol   string `json:"symbol" binding:"required"`
	Side     int8   `json:"side" binding:"required"` // 1: 买入, 2: 卖出
	Price    int64  `json:"price" binding:"required"`
	Quantity int64  `json:"quantity" binding:"required"`
}

// SetupRouter 初始化并配置路由
func SetupRouter(manager *orderbook.BookManager) *gin.Engine {
	// 创建一个默认的 Gin 引擎
	r := gin.Default()

	// 写一个 POST 接口接收发单请求
	r.POST("/api/order", func(c *gin.Context) {
		var req OrderRequest

		// 1. 把前端传过来的 JSON 解析到 req 结构体里
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
			return
		}

		// 2. 将前端请求转换成我们引擎核心的 domain.Order 模型
		order := &domain.Order{
			ID:        time.Now().UnixNano(), // 测试阶段先用当前时间戳模拟唯一ID
			Symbol:    req.Symbol,
			Side:      domain.Side(req.Side),
			Price:     req.Price,
			Quantity:  req.Quantity,
			Status:    domain.OrderStatusPending,
			CreatedAt: time.Now(),
		}

		err := kafka.SendOrder(order)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "投递订单失败"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "订单已极速受理，正在后台排队撮合",
			"order_id": order.ID,
		})
	})

	// GET 查询订单状态接口
	r.GET("/api/order/:id", func(c *gin.Context) {
		// 1. 从 URL 里拿到 /api/order/17765... 最后的那个数字
		idStr := c.Param("id")
		orderID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的订单ID"})
			return
		}

		// 2. 去数据库里查
		order, err := repo.GetOrder(orderID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "找不到该订单"})
			return
		}

		// 3. 返回给前端展示
		c.JSON(http.StatusOK, gin.H{
			"order_id":   order.ID,
			"symbol":     order.Symbol,
			"price":      order.Price,
			"quantity":   order.Quantity,
			"filled_qty": order.FilledQty, // 核心关注点：成交了多少？
			"status":     order.Status,    // 核心关注点：0挂单, 1部分成交, 2全部成交
		})
	})

	// 撤单接口
	r.POST("/api/order/:id/cancel", func(c *gin.Context) {
		idStr := c.Param("id")
		orderID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的订单ID"})
			return
		}
		//1.先查数据库，确认订单存在+检查状态
		order, err := repo.GetOrder(orderID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "找不到该订单"})
			return
		}
		//2.幂等处理：已结束的订单直接返回
		if order.Status == int8(domain.OrderStatusFilled) || order.Status == int8(domain.OrderStatusCanceled) {
			c.JSON(http.StatusOK, gin.H{"message": "订单已结束，无需撤单"})
			return
		}
		//3. 发撤单消息到Kafka (和下单走同一分区，保序)
		if err := kafka.SendCancel(orderID, order.Symbol); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "发送撤单请求失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "撤单请求已发送，正在处理中", "order_id": orderID})
	})

	//盘口快照
	r.GET("/api/orderbook/:symbol", func(c *gin.Context) {
		symbol := c.Param("symbol")

		book := manager.Get(symbol)
		if book == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "symbol 不存在"})
			return
		}

		depthStr := c.DefaultQuery("depth", "5")
		depth, err := strconv.Atoi(depthStr)
		if err != nil || depth <= 0 || depth > 50 {
			depth = 5
		}

		snapshot := book.GetSnapshot(depth)
		c.JSON(http.StatusOK, snapshot)
	})

	return r
}
