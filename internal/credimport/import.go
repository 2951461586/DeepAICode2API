// Package credimport 把「外部凭据」转换成号池账号。
//
// 来源有三种：桌面端凭据文件（deepai-account.bin）、手工填的 API Key、
// 网页注册/浏览器登录链路的结果。它们的共同点是**要碰文件系统**（扫描
// T3CODE_HOME / ~/.deepai-code）——这也是它从 internal/deepai 拆出来的原因：
// 一个只想要 Account 类型的包不该被迫接触用户目录。
//
// 依赖方向：credimport → deepai（只用到 Account/Credentials/Registration
// 这几个数据类型与两个哈希工具）。
package credimport

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"deepaicode2api/internal/configenv"
	"deepaicode2api/internal/model"
)

// 桌面端凭据文件名。
const (
	CredentialFile     = "deepai-account.bin"
	CredentialUserFlag = "deepai-account-user-managed.bin"
)

// ImportResult 是一次桌面端凭据导入结果。
type ImportResult struct {
	Account *model.Account
	Path    string
}

// ImportAllFromDesktop 扫描本机 DeepAI Code 桌面端的凭据文件。
//
// explicit 为空时自动探测 T3CODE_HOME / ~/.deepai-code；
// 也可传入任意层级路径（baseDir / stateDir / secrets / 凭据文件）。
func ImportAllFromDesktop(explicit string) ([]ImportResult, error) {
	dirs := desktopSecretDirs(explicit)

	// 直接指向凭据文件时优先读取它。
	if explicit != "" {
		if info, err := os.Stat(explicit); err == nil && !info.IsDir() {
			if res, err := importCredentialFile(explicit); err == nil {
				return []ImportResult{*res}, nil
			}
		}
	}

	seen := map[string]bool{}
	var out []ImportResult
	var lastErr error
	for _, dir := range dirs {
		path := filepath.Join(dir, CredentialFile)
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			continue
		}
		res, err := importCredentialFile(path)
		if err != nil {
			lastErr = err
			continue
		}
		fp := res.Account.KeyID()
		if seen[fp] {
			continue
		}
		seen[fp] = true
		out = append(out, *res)
	}
	if len(out) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("未找到桌面端凭据（已尝试：%s）", strings.Join(dirs, ", "))
	}
	return out, nil
}

func importCredentialFile(path string) (*ImportResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取凭据失败: %w", err)
	}
	var cred model.Credentials
	if err := json.Unmarshal(raw, &cred); err != nil {
		return nil, fmt.Errorf("凭据解析失败（%s）: %w", path, err)
	}
	acct, err := AccountFromCredentials(&cred, "desktop")
	if err != nil {
		return nil, err
	}
	return &ImportResult{Account: acct, Path: path}, nil
}

// AccountFromCredentials 把凭据转换为账号。
func AccountFromCredentials(cred *model.Credentials, source string) (*model.Account, error) {
	if cred == nil {
		return nil, errors.New("凭据为空")
	}
	acct, err := AccountFromAPIKey(cred.APIKey, cred.Username, source, cred.IsPro)
	if err != nil {
		return nil, err
	}
	return acct, nil
}

// AccountFromAPIKey 校验并构造一个账号。
func AccountFromAPIKey(key, username, source string, isPro bool) (*model.Account, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("API Key 不能为空")
	}
	if len(key) > 512 || strings.ContainsAny(key, " \t\r\n") {
		return nil, errors.New("API Key 格式非法")
	}
	username = strings.TrimSpace(username)
	name := username
	if name == "" {
		name = "DeepAI " + model.ShortHash(key)
	}
	if source == "" {
		source = "manual"
	}
	return &model.Account{
		ID:       model.RandomHex(8),
		Name:     name,
		Source:   source,
		APIKey:   key,
		Username: username,
		IsPro:    isPro,
		Group:    "default",
		Enabled:  true,
		Status:   "unknown",
	}, nil
}

// AccountFromRegistration 从网页注册链路的结果构造账号。
func AccountFromRegistration(r model.Registration) (*model.Account, error) {
	acct, err := AccountFromAPIKey(r.APIKey, r.Username, r.Source, strings.EqualFold(r.Plan, "pro"))
	if err != nil {
		return nil, err
	}
	email := strings.TrimSpace(r.Email)
	if acct.Name == "" || acct.Name == "DeepAI "+model.ShortHash(r.APIKey) {
		if email != "" {
			acct.Name = email
		}
	}
	if r.Username == "" && email != "" {
		acct.Username = email
	}
	acct.Email = email
	acct.Password = r.Password
	acct.WebSessionID = r.SessionID
	acct.CSRFToken = r.CSRFToken
	acct.UserPK = r.UserPK
	acct.Plan = r.Plan
	acct.EmailVerified = r.EmailVerified
	acct.RegisteredAt = timeNowUnix()
	acct.LastVerified = timeNowUnix()
	acct.Group = normalizeGroup(r.Group)
	acct.Proxy = strings.TrimSpace(r.Proxy)
	acct.Fingerprint = strings.TrimSpace(r.Fingerprint)
	acct.MachineID = strings.TrimSpace(r.MachineID)
	acct.UserAgent = strings.TrimSpace(r.UserAgent)
	acct.AcceptLanguage = strings.TrimSpace(r.AcceptLanguage)
	acct.SecCHUA = strings.TrimSpace(r.SecCHUA)
	acct.Platform = strings.TrimSpace(r.Platform)
	acct.Source = r.Source
	if acct.Source == "" {
		acct.Source = "register"
	}
	if acct.Username == "" {
		acct.Username = acct.Email
	}
	if acct.Name == "" {
		acct.Name = acct.Username
	}
	if acct.Name == "" {
		acct.Name = "DeepAI " + model.ShortHash(r.APIKey)
	}
	return acct, nil
}

// normalizeGroup 规范化分组名。
func normalizeGroup(g string) string {
	g = strings.TrimSpace(g)
	if g == "" {
		return "default"
	}
	return g
}

func timeNowUnix() int64 { return time.Now().Unix() }

// EnvAccount 从 DEEPAI_API_KEY 环境变量构造账号（可选）。
func EnvAccount() *model.Account {
	key := strings.TrimSpace(os.Getenv("DEEPAI_API_KEY"))
	if key == "" {
		return nil
	}
	acct, err := AccountFromAPIKey(key, configenv.Or("DEEPAI_API_KEY_NAME", ""), "env", false)
	if err != nil {
		return nil
	}
	return acct
}

// homeDir 返回当前用户主目录（失败时回落到当前目录）。
func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	if h := os.Getenv("USERPROFILE"); h != "" {
		return h
	}
	return "."
}

// desktopBaseDirs 返回本机 DeepAI Code 桌面端数据目录候选。
//
// 上游逻辑（apps/desktop/DesktopStatePaths.ts）：
//
//	baseDir = T3CODE_HOME || ~/.deepai-code
//	stateDir = baseDir/userdata（开发模式且未显式指定时用 baseDir/dev）
//
// secrets 位于 stateDir/secrets。
func desktopBaseDirs(explicit string) []string {
	if explicit != "" {
		return []string{explicit}
	}
	var out []string
	for _, env := range []string{"T3CODE_HOME", "DEEPAI_CODE_HOME"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			out = append(out, v)
		}
	}
	out = append(out, filepath.Join(homeDir(), ".deepai-code"))
	return dedupe(out)
}

// desktopSecretDirs 把 baseDir 展开为可能包含 secrets 的目录。
//
// 同时接受 baseDir、stateDir、secrets 目录或凭据文件本身，
// 方便用户直接把路径指到任意层级。
func desktopSecretDirs(explicit string) []string {
	var out []string
	for _, base := range desktopBaseDirs(explicit) {
		base = strings.TrimSpace(base)
		if base == "" {
			continue
		}
		info, err := os.Stat(base)
		if err == nil && !info.IsDir() {
			out = append(out, filepath.Dir(base))
			continue
		}
		out = append(out,
			filepath.Join(base, "userdata", "secrets"),
			filepath.Join(base, "secrets"),
			filepath.Join(base, "dev", "secrets"),
		)
	}
	return dedupe(out)
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		key := strings.ToLower(filepath.Clean(v))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	return out
}
