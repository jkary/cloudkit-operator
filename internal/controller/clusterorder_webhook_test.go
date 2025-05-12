package controller

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestInflightRequestCache(t *testing.T) {
	var (
		url1            = "http://webhook1.example.com"
		url2            = "http://webhook2.example.com"
		minInterval     = 2 * time.Second
		sleepBufferTime = 500 * time.Millisecond
		ctx             = context.TODO()
	)

	t.Run("checkForExistingRequest returns 0 when no request exists", func(t *testing.T) {
		resetInflightCache()
		if got := checkForExistingRequest(ctx, url1, minInterval); got != 0 {
			t.Errorf("Expected 0, got %v", got)
		}
	})

	t.Run("addInflightRequest stores the request", func(t *testing.T) {
		resetInflightCache()
		addInflightRequest(ctx, url1, minInterval)
		inflightRequestsLock.RLock()
		_, ok := inflightRequests[url1]
		inflightRequestsLock.RUnlock()
		if !ok {
			t.Errorf("Expected %s to be present in inflightRequests", url1)
		}
	})

	t.Run("checkForExistingRequest returns non-zero for recent request", func(t *testing.T) {
		resetInflightCache()
		addInflightRequest(ctx, url1, minInterval)
		delta := checkForExistingRequest(ctx, url1, minInterval)
		if delta <= 0 || delta > minInterval {
			t.Errorf("Expected delta in (0, %v], got %v", minInterval, delta)
		}
	})

	t.Run("purgeExpiredRequests only removes expired", func(t *testing.T) {
		resetInflightCache()
		addInflightRequest(ctx, url1, minInterval)
		time.Sleep(minInterval + sleepBufferTime)
		addInflightRequest(ctx, url2, minInterval)
		purgeExpiredRequests(ctx, minInterval)

		inflightRequestsLock.RLock()
		_, exists1 := inflightRequests[url1]
		_, exists2 := inflightRequests[url2]
		inflightRequestsLock.RUnlock()

		if exists1 {
			t.Errorf("Expected %s to be purged", url1)
		}
		if !exists2 {
			t.Errorf("Expected %s to still be in inflightRequests", url2)
		}
	})

	t.Run("concurrent access is safe", func(t *testing.T) {
		resetInflightCache()
		const workers = 10
		var wg sync.WaitGroup

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				u := url1
				if i%2 == 0 {
					u = url2
				}
				addInflightRequest(ctx, u, minInterval)
				checkForExistingRequest(ctx, u, minInterval)
				purgeExpiredRequests(ctx, minInterval)
			}(i)
		}
		wg.Wait()
	})

	t.Run("verify lock prevents data race with high concurrency", func(t *testing.T) {
		resetInflightCache()
		const goroutines = 100
		var wg sync.WaitGroup

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				url := url1
				if i%2 == 0 {
					url = url2
				}
				addInflightRequest(ctx, url, minInterval)
				_ = checkForExistingRequest(ctx, url, minInterval)
				purgeExpiredRequests(ctx, minInterval)
			}(i)
		}
		wg.Wait()
	})

}

// resetInflightCache is a helper to zero out the global state before each test
func resetInflightCache() {
	inflightRequestsLock.Lock()
	inflightRequests = make(map[string]InflightRequest)
	inflightRequestsLock.Unlock()
}
