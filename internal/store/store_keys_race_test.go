package store

import (
	"sync"
	"testing"
)

// TestKeysSnapshotIsRaceFree 锁死「Keys() 必须返回副本」这个契约。
//
// ValidKey 在每个数据面请求上就地累加 LastUsed / Requests（持写锁），而控制台
// 拿到 Keys() 之后是在锁外读同样的字段（server.safeKeyList）。返回号池里的活指针
// 时，下面这段并发会被 -race 直接抓到（历史实现就是这样）；返回副本则始终干净。
func TestKeysSnapshotIsRaceFree(t *testing.T) {
	st := newStore(t)
	key := st.AddKey("console", "")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			if !st.ValidKey(key.Key) {
				t.Error("ValidKey 应该一直成功")
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			// 读的字段与 safeKeyList 完全一致：任何一个共享都会命中竞态检测。
			for _, k := range st.Keys() {
				_ = k.Enabled
				_ = k.Group
				_ = k.LastUsed
				_ = k.Requests
			}
		}
	}()
	wg.Wait()

	if got := st.Keys()[0]; got.Requests != 500 {
		t.Fatalf("Requests = %d，期望 500", got.Requests)
	}
}

// TestAddKeyReturnsCopy 保证 AddKey 的返回值不是号池里的活指针。
func TestAddKeyReturnsCopy(t *testing.T) {
	st := newStore(t)
	k := st.AddKey("x", "")

	// 改返回值不得影响存储。
	k.Enabled = false
	k.Name = "hacked"
	k.Requests = 999

	got := st.Keys()[0]
	if !got.Enabled {
		t.Fatal("AddKey 返回值是活指针：改 Enabled 污染了号池")
	}
	if got.Name != "x" || got.Requests != 0 {
		t.Fatalf("AddKey 返回值是活指针：%+v", got)
	}
}
