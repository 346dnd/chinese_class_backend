// Package synclock 提供按 key 的并发控制锁。
//
// UserSubmitLock 用于按用户 ID 串行化异步提交，同一用户同一时间只允许一个提交在处理，
// 不同用户互不影响。
package synclock

import "sync"

// UserSubmitLock 按用户对异步提交做并发控制：同一用户同一时间只允许一个异步提交在处理，
// 不同用户各自拥有独立的提交槽，互不影响。
//
// 采用"立即拒绝"策略：槽被占用（上一条仍在处理）时 TryAcquire 直接返回 false，
// 调用方返回业务错误，不阻塞等待，避免并发刷提交。
type UserSubmitLock struct {
	mu    sync.Mutex
	slots map[int64]chan struct{}
}

// NewUserSubmitLock 创建 UserSubmitLock。
func NewUserSubmitLock() *UserSubmitLock {
	return &UserSubmitLock{slots: make(map[int64]chan struct{})}
}

// TryAcquire 尝试占用指定用户的提交槽。
// 成功返回 true；该用户已有提交在处理时立即返回 false。
func (l *UserSubmitLock) TryAcquire(userID int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	ch, ok := l.slots[userID]
	if !ok {
		ch = make(chan struct{}, 1)
		l.slots[userID] = ch
	}
	select {
	case ch <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release 释放指定用户的提交槽，须与 TryAcquire 成对调用。
// 槽空闲后从 map 中移除，避免长期占用内存；TryAcquire 与 Release 都在同一互斥锁内访问，
// 不会出现持有过期通道的竞态。
//
// 防御性要求：释放必须是非阻塞的——若未被占用（漏调 TryAcquire 或重复 Release），
// 直接跳过而不阻塞等待，避免持有互斥锁时永久阻塞导致所有用户提交全局死锁。
func (l *UserSubmitLock) Release(userID int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ch, ok := l.slots[userID]
	if !ok {
		return
	}
	select {
	case <-ch:
	default: // 槽未被占用（未成对调用），幂等跳过
	}
	if len(ch) == 0 {
		delete(l.slots, userID)
	}
}