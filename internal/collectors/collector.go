// Package collectors 提供系统指标的并行采集功能
package collectors

import (
	"context"
	"sync"
	"time"
)

// Collector 定义了一个指标采集器的接口
type Collector interface {
	// Name 返回采集器名称
	Name() string
	// Collect 执行采集，返回采集结果
	Collect(ctx context.Context) interface{}
}

// CollectorResult 包装采集器返回的结果
type CollectorResult struct {
	Name   string
	Data   interface{}
	Error  error
	Timing time.Duration
}

// ParallelCollector 并行执行多个采集器
type ParallelCollector struct {
	collectors []Collector
	timeout    time.Duration
}

// NewParallelCollector 创建一个新的并行采集器
func NewParallelCollector(timeout time.Duration) *ParallelCollector {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &ParallelCollector{
		timeout: timeout,
	}
}

// Register 注册一个采集器
func (p *ParallelCollector) Register(c Collector) {
	p.collectors = append(p.collectors, c)
}

// CollectAll 并行执行所有采集器
// 每个采集器都在独立的 goroutine 中运行，并有超时保护
func (p *ParallelCollector) CollectAll(ctx context.Context) map[string]CollectorResult {
	results := make(map[string]CollectorResult)
	resultCh := make(chan CollectorResult, len(p.collectors))

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	var wg sync.WaitGroup
	for _, c := range p.collectors {
		wg.Add(1)
		go func(collector Collector) {
			defer wg.Done()

			start := time.Now()
			data := collector.Collect(ctx)
			timing := time.Since(start)

			select {
			case resultCh <- CollectorResult{
				Name:   collector.Name(),
				Data:   data,
				Timing: timing,
			}:
			case <-ctx.Done():
				// 超时，丢弃结果
			}
		}(c)
	}

	// 等待所有采集器完成或超时
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 收集结果
	for result := range resultCh {
		results[result.Name] = result
	}

	return results
}
