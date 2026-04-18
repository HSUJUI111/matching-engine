package api

import (
	"net/http"
	"strconv"
	"time"

	"matching-engine/internal/domain"
	"matching-engine/internal/kafka"
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
func SetupRouter() *gin.Engine {
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

	// // 3. 激动人心的时刻：把订单塞进你手写的撮合引擎！！
	// trades := book.Match(order)

	// // 4. 把引擎的处理结果返回给前端
	// c.JSON(http.StatusOK, gin.H{
	// 	"message":          "订单接收成功",
	// 	"order_id":         order.ID,
	// 	"order_status":     order.Status,    // 看看是全部成交了，还是挂在树上了
	// 	"filled_qty":       order.FilledQty, // 看看成交了多少
	// 	"trades_generated": trades,          // 看看生成了几笔成交记录
	// })

	return r
}
