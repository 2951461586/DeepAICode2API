package credimport

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"deepaicode2api/internal/model"
)

// writeCredential 在指定目录下写一个桌面端凭据文件。
func writeCredential(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

// TestImportAllFromDesktopExplicitFile 覆盖「直接指到凭据文件」的最短路径。
func TestImportAllFromDesktopExplicitFile(t *testing.T) {
	dir := t.TempDir()
	path := writeCredential(t, dir, CredentialFile,
		`{"apiKey":"sk-desktop-1","username":"alice@example.com","isPro":true}`)

	results, err := ImportAllFromDesktop(path)
	if err != nil {
		t.Fatalf("ImportAllFromDesktop: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("导入条数 = %d，期望 1", len(results))
	}
	res := results[0]
	if res.Path != path {
		t.Fatalf("Path = %q，期望 %q", res.Path, path)
	}
	if res.Account.APIKey != "sk-desktop-1" || res.Account.Username != "alice@example.com" {
		t.Fatalf("账号字段错误: %+v", res.Account)
	}
	if res.Account.Source != "desktop" {
		t.Fatalf("Source = %q，期望 desktop", res.Account.Source)
	}
	if !res.Account.IsPro || !res.Account.Enabled || res.Account.Status != "unknown" {
		t.Fatalf("账号状态错误: %+v", res.Account)
	}
	if res.Account.Name != "alice@example.com" {
		t.Fatalf("Name 应取 username，得到 %q", res.Account.Name)
	}
}

// TestImportAllFromDesktopScansLayouts 覆盖 baseDir → userdata/dev 三种 secrets 目录，
// 并验证同一个 Key 在多处出现时只导入一次。
func TestImportAllFromDesktopScansLayouts(t *testing.T) {
	base := t.TempDir()
	sameKey := `{"apiKey":"sk-dup","username":"dup@example.com"}`

	// 同一个 Key 故意写两份：userdata/secrets 与 secrets。
	writeCredential(t, filepath.Join(base, "userdata", "secrets"), CredentialFile, sameKey)
	writeCredential(t, filepath.Join(base, "secrets"), CredentialFile, sameKey)
	// dev/secrets 里是另一个账号。
	writeCredential(t, filepath.Join(base, "dev", "secrets"), CredentialFile,
		`{"apiKey":"sk-dev","username":"dev@example.com"}`)

	results, err := ImportAllFromDesktop(base)
	if err != nil {
		t.Fatalf("ImportAllFromDesktop: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("去重后应有 2 个账号，得到 %d: %+v", len(results), results)
	}
	keys := map[string]bool{}
	for _, r := range results {
		keys[r.Account.KeyID()] = true
	}
	if len(keys) != 2 {
		t.Fatalf("导入的 Key 指纹重复: %v", keys)
	}

	// 显式指向 baseDir 与指向凭据文件同样有效。
	if got, err := ImportAllFromDesktop(filepath.Join(base, "userdata", "secrets", CredentialFile)); err != nil || len(got) != 1 {
		t.Fatalf("显式文件导入失败: %v / %d", err, len(got))
	}
}

func TestImportAllFromDesktopMissing(t *testing.T) {
	base := filepath.Join(t.TempDir(), "no-such-dir")
	_, err := ImportAllFromDesktop(base)
	if err == nil {
		t.Fatal("没有凭据文件时应报错")
	}
	if !strings.Contains(err.Error(), "未找到桌面端凭据") {
		t.Fatalf("错误文案应解释未找到，得到 %v", err)
	}
	if !strings.Contains(err.Error(), base) {
		t.Fatalf("错误应列出尝试过的目录，得到 %v", err)
	}
}

// TestImportAllFromDesktopCorruptFile 覆盖「文件在但解析失败」：
// 应把最后一次错误抛出来，而不是伪装成“未找到”。
func TestImportAllFromDesktopCorruptFile(t *testing.T) {
	base := t.TempDir()
	writeCredential(t, filepath.Join(base, "secrets"), CredentialFile, `{not json`)

	_, err := ImportAllFromDesktop(base)
	if err == nil || !strings.Contains(err.Error(), "凭据解析失败") {
		t.Fatalf("损坏的凭据应报解析失败，得到 %v", err)
	}
}

// TestImportSkipsFilesNamedLikeDirectories 确认目录名命中 CredentialFile 时不会 panic。
func TestImportSkipsFilesNamedLikeDirectories(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "secrets", CredentialFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportAllFromDesktop(base); err == nil || !strings.Contains(err.Error(), "未找到桌面端凭据") {
		t.Fatalf("应是“未找到”，得到 %v", err)
	}
}

func TestAccountFromCredentialsNil(t *testing.T) {
	if _, err := AccountFromCredentials(nil, "desktop"); err == nil ||
		!strings.Contains(err.Error(), "凭据为空") {
		t.Fatalf("nil 凭据应报错，得到 %v", err)
	}
}

func TestAccountFromAPIKeyValidation(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want string
	}{
		{"空", "", "不能为空"},
		{"纯空白", "   ", "不能为空"},
		{"超长", strings.Repeat("k", 513), "格式非法"},
		{"含空格", "sk-a b", "格式非法"},
		{"含换行", "sk-a\nb", "格式非法"},
		{"含制表符", "sk-a\tb", "格式非法"},
	}
	for _, c := range cases {
		_, err := AccountFromAPIKey(c.key, "u", "manual", false)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err=%v，期望包含 %q", c.name, err, c.want)
		}
	}

	// 512 字节恰好是上限，应通过。
	if _, err := AccountFromAPIKey(strings.Repeat("k", 512), "", "", false); err != nil {
		t.Fatalf("512 字节 Key 应合法，得到 %v", err)
	}

	acct, err := AccountFromAPIKey("  sk-ok  ", "", "", true)
	if err != nil {
		t.Fatalf("AccountFromAPIKey: %v", err)
	}
	if acct.APIKey != "sk-ok" {
		t.Fatalf("Key 未去空白: %q", acct.APIKey)
	}
	if acct.Source != "manual" {
		t.Fatalf("空 source 应回落 manual，得到 %q", acct.Source)
	}
	if acct.Group != "default" || !acct.Enabled || acct.Status != "unknown" {
		t.Fatalf("默认字段错误: %+v", acct)
	}
	if !acct.IsPro {
		t.Fatal("isPro 未透传")
	}
	if acct.Name != "DeepAI "+model.ShortHash("sk-ok") {
		t.Fatalf("无用户名时应生成兜底名，得到 %q", acct.Name)
	}
	if acct.ID == "" {
		t.Fatal("ID 不应为空")
	}
	// ID 必须是 8 字节随机 hex（16 字符）。
	if len(acct.ID) != 16 || strings.Trim(acct.ID, "0123456789abcdef") != "" {
		t.Fatalf("ID = %q，期望 16 位小写 hex", acct.ID)
	}
}

func TestAccountFromRegistration(t *testing.T) {
	r := model.Registration{
		Email:          "  bob@example.com  ",
		Password:       "pw",
		APIKey:         "sk-reg",
		SessionID:      "sess",
		CSRFToken:      "csrf",
		UserPK:         99,
		Plan:           "PRO",
		EmailVerified:  true,
		Group:          "  g2  ",
		Proxy:          " http://p:8080 ",
		Fingerprint:    " fp ",
		MachineID:      " machine ",
		UserAgent:      " UA ",
		AcceptLanguage: " zh-CN ",
		SecCHUA:        " chromium ",
		Platform:       " win ",
	}

	acct, err := AccountFromRegistration(r)
	if err != nil {
		t.Fatalf("AccountFromRegistration: %v", err)
	}
	if acct.Email != "bob@example.com" || acct.Username != "bob@example.com" {
		t.Fatalf("Email/Username 未规范化: %+v", acct)
	}
	if acct.Name != "bob@example.com" {
		t.Fatalf("无 username 时 Name 应取 Email，得到 %q", acct.Name)
	}
	if !acct.IsPro {
		t.Fatal("Plan=PRO 应判定 isPro")
	}
	if acct.Group != "g2" || acct.Proxy != "http://p:8080" || acct.Fingerprint != "fp" ||
		acct.MachineID != "machine" || acct.UserAgent != "UA" ||
		acct.AcceptLanguage != "zh-CN" || acct.SecCHUA != "chromium" || acct.Platform != "win" {
		t.Fatalf("注册字段未去空白/落库: %+v", acct)
	}
	if acct.Password != "pw" || acct.WebSessionID != "sess" || acct.CSRFToken != "csrf" || acct.UserPK != 99 {
		t.Fatalf("网页会话字段丢失: %+v", acct)
	}
	if !acct.EmailVerified || acct.Plan != "PRO" {
		t.Fatalf("验证状态/套餐丢失: %+v", acct)
	}
	if acct.Source != "register" {
		t.Fatalf("空 Source 应回落 register，得到 %q", acct.Source)
	}
	if acct.RegisteredAt == 0 || acct.LastVerified == 0 {
		t.Fatalf("时间戳未写入: %+v", acct)
	}

	// 显式 Source 优先。
	r.Source = "oauth"
	if acct, _ := AccountFromRegistration(r); acct.Source != "oauth" {
		t.Fatalf("Source = %q，期望 oauth", acct.Source)
	}
}

func TestAccountFromRegistrationEmptyGroup(t *testing.T) {
	acct, err := AccountFromRegistration(model.Registration{APIKey: "sk-1", Username: "u"})
	if err != nil {
		t.Fatalf("AccountFromRegistration: %v", err)
	}
	if acct.Group != "default" {
		t.Fatalf("空分组应规范化为 default，得到 %q", acct.Group)
	}
	if acct.Name != "u" {
		t.Fatalf("Name = %q，期望 u", acct.Name)
	}
	if acct.IsPro {
		t.Fatal("空 Plan 不应判定 isPro")
	}
}

func TestNormalizeGroup(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "default"},
		{"   ", "default"},
		{" g2 ", "g2"},
		{"default", "default"},
	}
	for _, c := range cases {
		if got := normalizeGroup(c.in); got != c.want {
			t.Errorf("normalizeGroup(%q)=%q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestEnvAccount(t *testing.T) {
	t.Setenv("DEEPAI_API_KEY", "")
	if got := EnvAccount(); got != nil {
		t.Fatalf("未设置环境变量时应返回 nil，得到 %+v", got)
	}

	t.Setenv("DEEPAI_API_KEY", "  sk-env  ")
	t.Setenv("DEEPAI_API_KEY_NAME", "  env-account  ")
	acct := EnvAccount()
	if acct == nil {
		t.Fatal("设置了有效 Key 应返回账号")
	}
	if acct.APIKey != "sk-env" || acct.Source != "env" || acct.Name != "env-account" {
		t.Fatalf("环境账号字段错误: %+v", acct)
	}

	// 非法 Key（含空格）应静默返回 nil，不阻塞网关启动。
	t.Setenv("DEEPAI_API_KEY", "sk-with space")
	if got := EnvAccount(); got != nil {
		t.Fatalf("非法 Key 应返回 nil，得到 %+v", got)
	}
}

// 从 internal/deepai 迁入：这两个用例断言的都是「构造器」行为，
// 与 HTTP 客户端无关，测试跟着代码走。

func TestRegistrationCarriesMachineID(t *testing.T) {
	acct, err := AccountFromRegistration(model.Registration{
		APIKey:    "sk-abc",
		Email:     "a@b.com",
		MachineID: "dddddddd-4444-4444-8444-444444444444",
	})
	if err != nil {
		t.Fatalf("AccountFromRegistration: %v", err)
	}
	if acct.MachineID != "dddddddd-4444-4444-8444-444444444444" {
		t.Fatalf("注册信息里的机器指纹丢失: %q", acct.MachineID)
	}
	// 首尾空白应被裁剪。
	acct2, err := AccountFromRegistration(model.Registration{
		APIKey: "sk-abc", Email: "a@b.com", MachineID: "  eeee  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if acct2.MachineID != "eeee" {
		t.Fatalf("机器指纹未裁剪: %q", acct2.MachineID)
	}
}

func TestAccountFromCredentialsPropagatesInvalidKey(t *testing.T) {
	_, err := AccountFromCredentials(&model.Credentials{APIKey: "bad key"}, "desktop")
	if err == nil || !strings.Contains(err.Error(), "格式非法") {
		t.Fatalf("非法 Key 应透传校验错误，得到 %v", err)
	}

	acct, err := AccountFromCredentials(&model.Credentials{APIKey: "sk-x", Username: "u", IsPro: true}, "")
	if err != nil {
		t.Fatalf("AccountFromCredentials: %v", err)
	}
	if acct.Source != "manual" {
		t.Fatalf("空 source 应回落 manual，得到 %q", acct.Source)
	}
	if acct.Name != "u" || !acct.IsPro {
		t.Fatalf("字段丢失: %+v", acct)
	}
}

// TestHomeDirFallback 覆盖主目录不可用时的回落顺序。
func TestHomeDirFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows 上 os.UserHomeDir 读 USERPROFILE，两者都清空才会失败。
		t.Setenv("USERPROFILE", "")
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
	} else {
		t.Setenv("HOME", "")
	}
	// 无论是否拿到真实主目录，都不允许返回空串。
	if got := homeDir(); got == "" {
		t.Fatal("HomeDir 不应为空串")
	}
}

// TestImportCredentialFileReadError 覆盖「路径存在但读不了」。
func TestImportCredentialFileReadError(t *testing.T) {
	dir := t.TempDir()
	if _, err := importCredentialFile(dir); err == nil ||
		!strings.Contains(err.Error(), "读取凭据失败") {
		t.Fatalf("目录当作凭据文件应报读取失败，得到 %v", err)
	}
	if _, err := importCredentialFile(dir + string(os.PathSeparator) + "missing.bin"); err == nil {
		t.Fatal("不存在的文件应报错")
	}
}

// 桌面端目录探测的用例随 homeDir / desktopBaseDirs / desktopSecretDirs 迁入。
func TestHomeDirIsNonEmpty(t *testing.T) {
	if got := homeDir(); got == "" {
		t.Fatal("HomeDir 任何时候都不应返回空串（需保证有回落）")
	}
}

func TestDesktopBaseDirsExplicit(t *testing.T) {
	got := desktopBaseDirs("/tmp/explicit-home")
	if len(got) != 1 || got[0] != "/tmp/explicit-home" {
		t.Fatalf("显式路径应原样返回单元素，得到 %v", got)
	}
}

// TestDesktopBaseDirsFromEnv 覆盖两个候选环境变量并验证去重。

func TestDesktopBaseDirsFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("T3CODE_HOME", dir)
	t.Setenv("DEEPAI_CODE_HOME", "  "+dir+"  ") // 同值（带空白）应被去重

	got := desktopBaseDirs("")
	if len(got) != 2 {
		t.Fatalf("期望 [envDir, ~/.deepai-code]，得到 %v", got)
	}
	if got[0] != dir {
		t.Fatalf("首个候选应来自 T3CODE_HOME 且已去空白，得到 %q", got[0])
	}
	if !strings.HasSuffix(got[1], filepath.Join(".deepai-code")) {
		t.Fatalf("末位候选应为 ~/.deepai-code，得到 %q", got[1])
	}

	// 环境变量清空后只剩主目录候选。
	t.Setenv("T3CODE_HOME", "")
	t.Setenv("DEEPAI_CODE_HOME", "")
	if got := desktopBaseDirs(""); len(got) != 1 {
		t.Fatalf("无环境变量时应只有一个候选，得到 %v", got)
	}
}

// TestDesktopSecretDirsLayouts 覆盖“用户可能指到任意一层”的四种形态。
func TestDesktopSecretDirsLayouts(t *testing.T) {
	base := t.TempDir()

	// 1) 指向 baseDir：展开成三个候选（userdata/secrets、secrets、dev/secrets）。
	got := desktopSecretDirs(base)
	want := []string{
		filepath.Join(base, "userdata", "secrets"),
		filepath.Join(base, "secrets"),
		filepath.Join(base, "dev", "secrets"),
	}
	if len(got) != len(want) {
		t.Fatalf("baseDir 展开 = %v，期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("baseDir 展开[%d] = %q，期望 %q", i, got[i], want[i])
		}
	}

	// 2) 直接指向凭据文件：回落到它所在目录。
	file := filepath.Join(base, CredentialFile)
	if err := os.WriteFile(file, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := desktopSecretDirs(file); len(got) != 1 || got[0] != base {
		t.Fatalf("凭据文件应回落到所在目录，得到 %v", got)
	}

	// 3) 指向不存在的路径：仍展开三个候选（调用方自己 Stat）。
	missing := filepath.Join(base, "nope")
	if got := desktopSecretDirs(missing); len(got) != 3 {
		t.Fatalf("不存在的路径也应展开候选，得到 %v", got)
	}

	// 4) 空串：沿用 DesktopBaseDirs 的探测结果。
	t.Setenv("T3CODE_HOME", base)
	t.Setenv("DEEPAI_CODE_HOME", "")
	got = desktopSecretDirs("")
	if len(got) != 6 { // base 的 3 个 + ~/.deepai-code 的 3 个
		t.Fatalf("空串探测应有 6 个候选，得到 %v", got)
	}
}

func TestDedupeNormalizesCaseAndClean(t *testing.T) {
	// Windows 下大小写不敏感，Linux 下 cleaner 只做 path.Clean；
	// dedupe 统一小写比较，两种平台都应只保留一份。
	got := dedupe([]string{"/a/b", "/a/b/", "/A/B", "/a//b", "/a/c"})
	if len(got) != 2 {
		t.Fatalf("dedupe 结果 = %v，期望 2 项", got)
	}
	if got[0] != "/a/b" {
		t.Fatalf("dedupe 应保留首次出现的原值，得到 %q", got[0])
	}
	if got := dedupe(nil); len(got) != 0 {
		t.Fatalf("nil 输入应返回空切片，得到 %v", got)
	}
}
