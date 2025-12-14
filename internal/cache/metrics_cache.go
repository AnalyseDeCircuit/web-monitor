// Package cache 提供缓存功能
package cache

import (
	"sync"
	"time"
)

// CacheItem 缓存项
type CacheItem struct {
	Data      interface{}
	Timestamp time.Time
	TTL       time.Duration
}

// MetricsCache 监控数据缓存
type MetricsCache struct {
	items map[string]CacheItem
	mu    sync.RWMutex
}

// NewMetricsCache 创建新的监控数据缓存
func NewMetricsCache() *MetricsCache {
	return &MetricsCache{
		items: make(map[string]CacheItem),
	}
}

// Set 设置缓存项
func (c *MetricsCache) Set(key string, data interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = CacheItem{
		Data:      data,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
}

// Get 获取缓存项
func (c *MetricsCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	item, exists := c.items[key]
	c.mu.RUnlock()

	if !exists {
		return nil, false
	}

	// 检查是否过期
	if time.Since(item.Timestamp) > item.TTL {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return nil, false
	}

	return item.Data, true
}

// Delete 删除缓存项
func (c *MetricsCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Clear 清空所有缓存
func (c *MetricsCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]CacheItem)
}

// Cleanup 清理过期缓存
func (c *MetricsCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, item := range c.items {
		if now.Sub(item.Timestamp) > item.TTL {
			delete(c.items, key)
		}
	}
}

// Size 获取缓存大小
func (c *MetricsCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Keys 获取所有缓存键
func (c *MetricsCache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.items))
	for key := range c.items {
		keys = append(keys, key)
	}
	return keys
}

// GlobalMetricsCache 全局监控数据缓存实例
var GlobalMetricsCache = NewMetricsCache()

// 缓存键常量
const (
	CacheKeySystemMetrics    = "system_metrics"
	CacheKeyDockerContainers = "docker_containers"
	CacheKeySystemdServices  = "systemd_services"
	CacheKeyNetworkInfo      = "network_info"
	CacheKeyDiskUsage        = "disk_usage"
	CacheKeyProcessList      = "process_list"
)

// DefaultTTL 默认缓存过期时间
const DefaultTTL = 5 * time.Second
