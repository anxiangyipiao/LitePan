// Package ttlcache 提供极简有界 TTL 缓存，供 WebDAV/鉴权等热路径
// 消除重复的昂贵计算（密码哈希校验、DB 读取）。非 LRU：超上限时
// 先清过期项，仍超限则整体清空以保界。
package ttlcache

import (
	"sync"
	"time"
)

type item[V any] struct {
	val V
	at  time.Time
}

// Cache 是并发安全的有界 TTL 缓存。
type Cache[K comparable, V any] struct {
	mu    sync.Mutex
	ttl   time.Duration
	max   int
	items map[K]item[V]
}

// New 构造缓存。ttl<=0 用 1 分钟；max<=0 用 1024。
func New[K comparable, V any](ttl time.Duration, max int) *Cache[K, V] {
	if ttl <= 0 {
		ttl = time.Minute
	}
	if max <= 0 {
		max = 1024
	}
	return &Cache[K, V]{ttl: ttl, max: max, items: make(map[K]item[V])}
}

// Get 读取缓存；未命中或已过期返回 (零值, false) 并清除过期项。
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	if time.Since(it.at) >= c.ttl {
		delete(c.items, key)
		var zero V
		return zero, false
	}
	return it.val, true
}

// Set 写入缓存。已有键直接刷新；新键达到上限时先清过期项，仍满则整体清空。
func (c *Cache[K, V]) Set(key K, val V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; !ok && len(c.items) >= c.max {
		c.pruneExpiredLocked()
		if len(c.items) >= c.max {
			clear(c.items)
		}
	}
	c.items[key] = item[V]{val: val, at: time.Now()}
}

// Delete 删除单个键。
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Clear 清空全部缓存。写入侧发生变更时调用，避免读到过期值。
func (c *Cache[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.items)
}

// Len 返回当前条数（含未过期项）。
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *Cache[K, V]) pruneExpiredLocked() {
	now := time.Now()
	for k, it := range c.items {
		if now.Sub(it.at) >= c.ttl {
			delete(c.items, k)
		}
	}
}
