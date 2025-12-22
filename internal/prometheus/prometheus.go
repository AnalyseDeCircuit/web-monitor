// Package prometheus 提供简单的指标导出功能
package prometheus

import (
	"net/http"
)

// GetMetricsHandler 获取Prometheus指标处理器
func GetMetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(`# HELP opskernel_info OpsKernel information
# TYPE opskernel_info gauge
opskernel_info{version="1.0.0"} 1

# HELP opskernel_health OpsKernel health status
# TYPE opskernel_health gauge
opskernel_health 1

# HELP opskernel_api_requests_total Total API requests
# TYPE opskernel_api_requests_total counter
opskernel_api_requests_total 0

# HELP opskernel_cache_size Cache size
# TYPE opskernel_cache_size gauge
opskernel_cache_size 0
`))
	})
}

// InitPrometheus 初始化Prometheus指标（空实现）
func InitPrometheus() {
	// 空实现，不执行任何操作
}

// GetMetrics 获取当前指标值
func GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"status":  "healthy",
		"version": "1.0.0",
		"metrics": map[string]interface{}{
			"cache_size": 0,
			"api_calls":  0,
		},
	}
}
