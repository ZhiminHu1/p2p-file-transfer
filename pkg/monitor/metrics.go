package monitor

import (
	"runtime"
	"sync/atomic"
	"time"

	"tarun-kavipurapu/p2p-transfer/pkg/logger"
)

// Metrics holds performance metrics for the peer server
type Metrics struct {
	// Total bytes transferred
	TransferBytes int64
	// Number of files/chunks transferred
	TransferCount int64
	// Server start time
	ServerStart time.Time
	// Current transfer start time
	TransferStart time.Time
}

// Global metrics instance
var Global = &Metrics{
	ServerStart: time.Now(),
}

// LogPeriodic logs runtime metrics at the specified interval
func LogPeriodic(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		elapsed := time.Since(Global.ServerStart).Seconds()
		var throughput float64
		if elapsed > 0 {
			throughput = float64(atomic.LoadInt64(&Global.TransferBytes)) / elapsed / 1024 / 1024
		}

		count := atomic.LoadInt64(&Global.TransferCount)

		logger.Sugar.Infof("[Metrics] Goroutines=%d | HeapAlloc=%dMB | HeapSys=%dMB | Throughput=%.2fMB/s | Transfers=%d",
			runtime.NumGoroutine(),
			m.HeapAlloc/1024/1024,
			m.HeapSys/1024/1024,
			throughput,
			count,
		)
	}
}

// StartTransfer records the start of a transfer
func StartTransfer() {
	Global.TransferStart = time.Now()
}

// RecordTransfer records a completed transfer
func RecordTransfer(bytes int64) {
	atomic.AddInt64(&Global.TransferBytes, bytes)
	atomic.AddInt64(&Global.TransferCount, 1)

	duration := time.Since(Global.TransferStart).Seconds()
	var speed float64
	if duration > 0 {
		speed = float64(bytes) / duration / 1024 / 1024
	}

	logger.Sugar.Infof("[Transfer] Size=%dMB | Duration=%.2fs | Speed=%.2fMB/s",
		bytes/1024/1024, duration, speed)
}
