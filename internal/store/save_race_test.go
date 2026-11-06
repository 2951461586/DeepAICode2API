package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"deepaicode2api/internal/deepai"
)

// 并发 Save 必须保证落盘文件始终是完整合法的状态文件。
//
// 真实触发路径：批量入池后每个账号起一个复探 goroutine（scheduleQuotaReprobe），
// 它们在同一时刻醒来并各自调用 Save()；同时后台还有 30s 周期保存、
// 管理接口的增删改也各自 Save。
func TestConcurrentSaveKeepsFileValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// 先塞入足够多的账号，让单次序列化体足够大，更容易撞上临时文件竞争。
	for i := 0; i < 200; i++ {
		st.UpsertAccount(&deepai.Account{
			ID: fmt.Sprintf("seed-%03d", i), APIKey: fmt.Sprintf("sk-seed-%03d", i),
			Email: fmt.Sprintf("seed%03d@x.com", i), Enabled: true,
		})
	}
	if err := st.Save(); err != nil {
		t.Fatalf("初始 Save: %v", err)
	}

	const goroutines, rounds = 8, 15
	var wg sync.WaitGroup
	errs := make(chan error, goroutines*rounds)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				id := fmt.Sprintf("acc-%d-%d", g, i)
				st.UpsertAccount(&deepai.Account{
					ID: id, APIKey: "sk-" + id, Email: id + "@x.com", Enabled: true,
				})
				st.Record(LogEntry{Endpoint: "chat", Status: 200}, 1)
				if err := st.Save(); err != nil {
					errs <- err
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("并发 Save 返回错误: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取状态文件: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("并发 Save 后状态文件不是合法 JSON（%d 字节）", len(raw))
	}

	// 还应能被正常重新载入，且账号不丢。
	back, err := Open(path)
	if err != nil {
		t.Fatalf("重新打开失败: %v", err)
	}
	if got := len(back.Accounts()); got < 200 {
		t.Fatalf("账号丢失：got %d", got)
	}
}

// 同一路径的两个 Store 实例（比如误起了两个网关）也不应写坏文件。
func TestConcurrentSaveAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open a: %v", err)
	}
	b, err := Open(path)
	if err != nil {
		t.Fatalf("Open b: %v", err)
	}
	for i := 0; i < 100; i++ {
		a.UpsertAccount(&deepai.Account{ID: fmt.Sprintf("a-%03d", i), APIKey: fmt.Sprintf("sk-a-%03d", i)})
		b.UpsertAccount(&deepai.Account{ID: fmt.Sprintf("b-%03d", i), APIKey: fmt.Sprintf("sk-b-%03d", i)})
	}

	var wg sync.WaitGroup
	for _, st := range []*Store{a, b} {
		wg.Add(1)
		go func(st *Store) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				_ = st.Save()
			}
		}(st)
	}
	wg.Wait()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("跨实例并发 Save 写坏了状态文件（%d 字节）", len(raw))
	}
}
