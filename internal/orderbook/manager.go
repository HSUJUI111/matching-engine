package orderbook

import "sync"

// BookManager 管理所有symbol 的OrderBook
type BookManager struct {
	books map[string]*OrderBook
	mu    sync.RWMutex
}

func NewBookManager() *BookManager {
	return &BookManager{
		books: make(map[string]*OrderBook),
	}
}

func (m *BookManager) Register(symbol string) *OrderBook {
	m.mu.Lock()
	defer m.mu.Unlock()

	if book, ok := m.books[symbol]; ok {
		return book
	}
	book := NewOrderBook(symbol)
	m.books[symbol] = book
	return book
}

func (m *BookManager) Get(symbol string) *OrderBook {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.books[symbol]
}

// 获取或自动创建（消费者收到未注册symbol 时使用）
func (m *BookManager) GetOrCreate(symbol string) *OrderBook {
	m.mu.RLock()
	if book, ok := m.books[symbol]; ok {
		m.mu.RUnlock()
		return book
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	// 双重检查锁，防止重复创建
	if book, ok := m.books[symbol]; ok {
		return book
	}
	book := NewOrderBook(symbol)
	m.books[symbol] = book
	return book
}

// 列出当前所有已注册的symbol
func (m *BookManager) ListSymbols() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	symbols := make([]string, 0, len(m.books))
	for s := range m.books {
		symbols = append(symbols, s)
	}
	return symbols
}
