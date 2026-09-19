package synclock

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestUserSubmitLockSerializesSameUser 模拟 SubmitAsync 的用法：
// 同一用户并发提交时，只有一次能拿到锁，其余立即被拒；前一次释放后下一次可再次获取。
func TestUserSubmitLockSerializesSameUser(t *testing.T) {
	l := NewUserSubmitLock()
	const userID = int64(42)

	// 第一次获取成功
	if !l.TryAcquire(userID) {
		t.Fatal("first acquire should succeed")
	}
	// 并发获取应失败（串行化）
	if l.TryAcquire(userID) {
		t.Fatal("second concurrent acquire should fail (lock held)")
	}
	// 释放后应可再次获取
	l.Release(userID)
	if !l.TryAcquire(userID) {
		t.Fatal("acquire after release should succeed")
	}
	l.Release(userID)
}

// TestUserSubmitLockConcurrent 并发压测：模拟多个用户、每个用户快速连续提交，
// 校验不会出现“两个提交同时持锁”以及死锁。
func TestUserSubmitLockConcurrent(t *testing.T) {
	l := NewUserSubmitLock()
	const users = 8
	const rounds = 200

	var wg sync.WaitGroup
	// 每个用户维护一个“当前持锁计数”，必须始终 <= 1
	counters := make([]int, users)
	var mu sync.Mutex
	failed := make(chan error, users)

	for u := 0; u < users; u++ {
		wg.Add(1)
		go func(u int) {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				if l.TryAcquire(int64(u)) {
					mu.Lock()
					counters[u]++
					if counters[u] > 1 {
						mu.Unlock()
						failed <- fmt.Errorf("user %d holds lock more than once: %d", u, counters[u])
						return
					}
					mu.Unlock()

					// 模拟异步处理耗时
					time.Sleep(time.Millisecond)

					l.Release(int64(u))
					mu.Lock()
					counters[u]--
					mu.Unlock()
				}
				// 被拒绝的情况直接进入下一轮
			}
		}(u)
	}
	wg.Wait()
	close(failed)
	for err := range failed {
		if err != nil {
			t.Fatal(err)
		}
	}
}
