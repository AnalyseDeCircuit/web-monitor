package cache

import (
	"testing"
	"time"
)

func TestMetricsCache(t *testing.T) {
	cache := NewMetricsCache()

	// 测试设置和获取
	cache.Set("test_key", "test_value", 1*time.Second)

	// 立即获取应该成功
	if value, found := cache.Get("test_key"); !found || value != "test_value" {
		t.Errorf("Get() = %v, %v, want %v, true", value, found, "test_value")
	}

	// 测试过期
	time.Sleep(2 * time.Second)
	if _, found := cache.Get("test_key"); found {
		t.Error("Get() after expiration should return false")
	}

	// 测试删除
	cache.Set("test_key2", "test_value2", 1*time.Minute)
	cache.Delete("test_key2")
	if _, found := cache.Get("test_key2"); found {
		t.Error("Get() after Delete() should return false")
	}

	// 测试清空
	cache.Set("test_key3", "test_value3", 1*time.Minute)
	cache.Set("test_key4", "test_value4", 1*time.Minute)
	cache.Clear()
	if size := cache.Size(); size != 0 {
		t.Errorf("Size() after Clear() = %v, want 0", size)
	}

	// 测试清理过期
	cache.Set("test_key5", "test_value5", 100*time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	cache.Cleanup()
	if size := cache.Size(); size != 0 {
		t.Errorf("Size() after Cleanup() = %v, want 0", size)
	}

	// 测试键列表
	cache.Set("key1", "value1", 1*time.Minute)
	cache.Set("key2", "value2", 1*time.Minute)
	keys := cache.Keys()
	if len(keys) != 2 {
		t.Errorf("Keys() length = %v, want 2", len(keys))
	}
}

func TestGlobalMetricsCache(t *testing.T) {
	// 测试全局实例
	GlobalMetricsCache.Set("global_test", "global_value", 1*time.Second)

	if value, found := GlobalMetricsCache.Get("global_test"); !found || value != "global_value" {
		t.Errorf("GlobalMetricsCache.Get() = %v, %v, want %v, true", value, found, "global_value")
	}
}
