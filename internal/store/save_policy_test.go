package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"deepaicode2api/internal/deepai"
)

// Save 落盘时 LastUsed 应量化到 5 分钟：它每个请求都变，秒级精度会让
// 「内容未变则跳过」在持续流量下失效（653 号池每次重写约 950KB）。
func TestSaveQuantizesLastUsed(t *testing.T) {
	st := newStore(t)
	acct := addAccount(t, st, "sk-quant", "default")
	now := time.Now().Unix()
	st.UpdateAccount(acct.ID, func(a *deepai.Account) {
		a.LastUsed = now
		a.Theos = &deepai.Allowance{Enabled: true, Balance: "0.00000", CheckedAt: now}
	})

	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(st.Path())
	if err != nil {
		t.Fatalf("读取状态文件: %v", err)
	}
	var disk struct {
		Accounts []struct {
			LastUsed int64 `json:"last_used"`
			Theos    struct {
				CheckedAt int64 `json:"checked_at"`
			} `json:"theos"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatalf("解析状态文件: %v", err)
	}
	if len(disk.Accounts) != 1 {
		t.Fatalf("账号数 = %d", len(disk.Accounts))
	}
	got := disk.Accounts[0].LastUsed
	if got%saveTimestampGranularity != 0 {
		t.Fatalf("落盘 LastUsed = %d，未量化到 %ds", got, saveTimestampGranularity)
	}
	if got > now {
		t.Fatalf("落盘 LastUsed = %d 晚于当前时间 %d", got, now)
	}
	// 额度探测时间戳同样量化：它每次探测都会变，否则写放大又回来了。
	if ca := disk.Accounts[0].Theos.CheckedAt; ca%saveTimestampGranularity != 0 {
		t.Fatalf("落盘 checked_at = %d，未量化到 %ds", ca, saveTimestampGranularity)
	}
	// 内存里保持秒级精度。
	if mem := st.Account(acct.ID).LastUsed; mem != now {
		t.Fatalf("内存 LastUsed = %d，期望 %d", mem, now)
	}
}

// 同一 5 分钟窗口内的新请求不应触发第二次主文件重写。
func TestSaveSkipsRewriteWithinGranularityWindow(t *testing.T) {
	st := newStore(t)
	acct := addAccount(t, st, "sk-window", "default")
	base := time.Now().Unix() - time.Now().Unix()%saveTimestampGranularity
	st.UpdateAccount(acct.ID, func(a *deepai.Account) { a.LastUsed = base + 1 })
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	saves, _ := st.SaveCounters()

	st.UpdateAccount(acct.ID, func(a *deepai.Account) { a.LastUsed = base + 120 })
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got, _ := st.SaveCounters(); got != saves {
		t.Fatalf("同一窗口内又写了一次主文件：%d → %d", saves, got)
	}
}

// 写盘后若有请求推进统计，脏标志不能被清掉，否则最后一条统计只在内存里。
func TestStatsDirtyKeptWhenRecordArrivesAfterSnapshot(t *testing.T) {
	st := newStore(t)

	written := st.Stats() // 模拟 Save 取到的快照
	st.Record(LogEntry{Status: 200}, 0)
	st.clearStatsDirtyIfUnchanged(written)

	st.mu.RLock()
	dirty := st.statsDirty
	st.mu.RUnlock()
	if !dirty {
		t.Fatal("快照之后到达的统计被清脏，最后一次自增不会落盘")
	}

	// 快照与当前一致时才允许清脏。
	st.clearStatsDirtyIfUnchanged(st.Stats())
	st.mu.RLock()
	dirty = st.statsDirty
	st.mu.RUnlock()
	if dirty {
		t.Fatal("统计未变化时脏标志应被清掉")
	}
}

func TestClearStatsKeepsStartedAtAndRecordsReset(t *testing.T) {
	st := newStore(t)
	st.Record(LogEntry{Status: 200}, 0)
	started := st.Stats().StartedAt
	time.Sleep(1100 * time.Millisecond) // StartedAt / ResetAt 都是秒级

	st.ClearStats()
	got := st.Stats()
	if got.StartedAt != started {
		t.Fatalf("StartedAt 被重置：%d → %d", started, got.StartedAt)
	}
	if got.ResetAt <= started {
		t.Fatalf("ResetAt = %d，应晚于 StartedAt = %d", got.ResetAt, started)
	}
	if got.TotalRequests != 0 {
		t.Fatalf("重置后 TotalRequests = %d", got.TotalRequests)
	}
}

// 快照必须与活数据隔离：Save 的序列化在锁外，共享指针会和就地修改撞车。
func TestSnapshotIsolatesKeysAndModels(t *testing.T) {
	st := newStore(t)
	k1 := st.AddKey("a", "")
	k2 := st.AddKey("b", "")
	st.SetModels([]deepai.Model{{
		ID:             "m1",
		Input:          []string{"text"},
		TheosPerSecond: map[string]string{"out": "1"},
	}}, "m1")

	st.mu.RLock()
	snap := st.snapshotLocked()
	st.mu.RUnlock()

	// 快照之后的改动不得反向污染快照。
	st.ToggleKey(k1.Key)
	st.DeleteKey(k2.Key)
	st.SetModels([]deepai.Model{{ID: "m2"}}, "m2")

	if len(snap.Keys) != 2 {
		t.Fatalf("快照密钥数 = %d，期望 2", len(snap.Keys))
	}
	if !snap.Keys[0].Enabled {
		t.Fatal("快照密钥被后来的 ToggleKey 改了")
	}
	if len(snap.Models) != 1 || snap.Models[0].ID != "m1" {
		t.Fatalf("快照模型被后来的 SetModels 改了: %#v", snap.Models)
	}
	if len(snap.Models[0].Input) != 1 || snap.Models[0].Input[0] != "text" {
		t.Fatalf("模型 Input 未深拷贝: %#v", snap.Models[0].Input)
	}
	if snap.Models[0].TheosPerSecond["out"] != "1" {
		t.Fatalf("模型 map 未深拷贝: %#v", snap.Models[0].TheosPerSecond)
	}
}

// 并发：密钥增删/开关与 Save 同时进行（配合 -race 验证无共享写）。
func TestSaveWithConcurrentKeyMutations(t *testing.T) {
	st := newStore(t)
	st.AddKey("base", "")

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 40; i++ {
			k := st.AddKey("tmp", "")
			st.ToggleKey(k.Key)
			st.DeleteKey(k.Key)
		}
	}()
	for i := 0; i < 20; i++ {
		if err := st.Save(); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	<-done
}

func TestSaveCountersTrackMainFileWrites(t *testing.T) {
	st := newStore(t)
	addAccount(t, st, "sk-counter", "default")
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	saves, written := st.SaveCounters()
	if saves != 1 || written <= 0 {
		t.Fatalf("首次保存计数 = (%d, %d)", saves, written)
	}
	// 内容未变：不应再写。
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if again, _ := st.SaveCounters(); again != saves {
		t.Fatalf("无变化时仍写盘：%d → %d", saves, again)
	}
}

// -seal 的核心承诺：历史明文日志行也要被一并封存，且重开后仍可读。
func TestResealSealsLegacyPlaintextLogs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	key := bytes.Repeat([]byte{0x2a}, 32)

	// 1) 先用无密钥模式写出明文状态与明文日志（模拟历史数据）。
	plain, err := OpenWithKey(path, nil)
	if err != nil {
		t.Fatalf("OpenWithKey(nil): %v", err)
	}
	if plain.MasterKeySet() {
		t.Fatal("前置断言失败：无密钥模式不应报告已配置主密钥")
	}
	addAccount(t, plain, "sk-legacy", "default")
	plain.Record(LogEntry{Time: 1, Status: 200}, 0)
	if err := plain.Save(); err != nil {
		t.Fatalf("明文 Save: %v", err)
	}

	statsPath, logsPath := deriveSidePaths(path)
	extras := []string{
		logsPath,
		logsPath + ".1",
	}
	for _, p := range extras {
		if err := os.WriteFile(p, []byte(`{"time":2,"status":200}`+"\n"), 0o600); err != nil {
			t.Fatalf("写入明文日志 %s: %v", p, err)
		}
	}

	// 2) 用主密钥重新打开并封存。
	st, err := OpenWithKey(path, key)
	if err != nil {
		t.Fatalf("OpenWithKey(key): %v", err)
	}
	rep, err := st.Reseal()
	if err != nil {
		t.Fatalf("Reseal: %v", err)
	}
	if rep.Accounts != 1 {
		t.Fatalf("Reseal 账号数 = %d", rep.Accounts)
	}
	if rep.SealedLines < 2 {
		t.Fatalf("封存的明文日志行 = %d，期望 ≥2", rep.SealedLines)
	}

	// 3) 每个文件都不再是明文。
	assertSealedFile(t, path)
	assertSealedFile(t, statsPath)
	for _, p := range extras {
		assertSealedLines(t, p)
	}

	// 4) 重开后历史日志仍可读（说明封存没有破坏内容）。
	reopened, err := OpenWithKey(path, key)
	if err != nil {
		t.Fatalf("重新打开: %v", err)
	}
	if logs := reopened.Logs(10); len(logs) < 1 {
		t.Fatalf("封存后日志丢失：%d 条", len(logs))
	}

	// 5) 幂等：再跑一次不应再"转换"任何行。
	again, err := st.Reseal()
	if err != nil {
		t.Fatalf("二次 Reseal: %v", err)
	}
	if again.SealedLines != 0 {
		t.Fatalf("二次 Reseal 转换了 %d 行，应幂等", again.SealedLines)
	}
}

// 加密开启后日志必须仍能重启读回：这是 sealLine（单行信封）的回归测试。
//
// 改用缩进信封写日志时，一条记录会被拆成多行，按行切分的 decodeLine
// 解析不出来，表现为“开启加密后日志一重启就消失”。
func TestEncryptedLogsSurviveReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	key := bytes.Repeat([]byte{0x11}, 32)

	st, err := OpenWithKey(path, key)
	if err != nil {
		t.Fatalf("OpenWithKey: %v", err)
	}
	st.Record(LogEntry{Time: 100, Status: 200, Model: "glm-5.3-flash"}, 0)
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, logsPath := deriveSidePaths(path)
	assertSealedLines(t, logsPath)

	reopened, err := OpenWithKey(path, key)
	if err != nil {
		t.Fatalf("重新打开: %v", err)
	}
	logs := reopened.Logs(10)
	if len(logs) != 1 {
		t.Fatalf("重启后日志条数 = %d，期望 1", len(logs))
	}
	if logs[0].Status != 200 || logs[0].Model != "glm-5.3-flash" {
		t.Fatalf("日志内容损坏: %#v", logs[0])
	}
}

func TestResealRequiresMasterKey(t *testing.T) {
	st := newStore(t) // 未配置主密钥
	if _, err := st.Reseal(); err == nil {
		t.Fatal("未配置主密钥时 Reseal 应报错")
	}
}

func assertSealedFile(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s: %v", path, err)
	}
	if !isEnvelope(raw) {
		t.Fatalf("%s 仍是明文", path)
	}
}

func assertSealedLines(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s: %v", path, err)
	}
	for i, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if !isEnvelope(line) {
			t.Fatalf("%s 第 %d 行仍是明文: %q", path, i+1, line)
		}
	}
}
