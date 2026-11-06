package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnv(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("写入 %s: %v", name, err)
	}
	return path
}

func TestLoadFilesInjectsValues(t *testing.T) {
	t.Setenv("ENVFILE_A", "")
	os.Unsetenv("ENVFILE_A")
	path := writeEnv(t, ".env", "# 注释\nENVFILE_A=hello\n\nexport ENVFILE_B='quoted value'\n")

	if n := LoadFiles(path); n != 2 {
		t.Fatalf("注入键数 = %d，期望 2", n)
	}
	if got := os.Getenv("ENVFILE_A"); got != "hello" {
		t.Fatalf("ENVFILE_A = %q", got)
	}
	if got := os.Getenv("ENVFILE_B"); got != "quoted value" {
		t.Fatalf("ENVFILE_B = %q（引号未剥离）", got)
	}
}

func TestLoadFilesKeepsRealEnvFirst(t *testing.T) {
	t.Setenv("ENVFILE_KEEP", "from-env")
	path := writeEnv(t, ".env", "ENVFILE_KEEP=from-file\n")

	if n := LoadFiles(path); n != 0 {
		t.Fatalf("已存在的键不应被注入，实际 %d", n)
	}
	if got := os.Getenv("ENVFILE_KEEP"); got != "from-env" {
		t.Fatalf("真实环境变量被覆盖: %q", got)
	}
}

func TestLoadFilesSkipsMissingAndBrokenLines(t *testing.T) {
	path := writeEnv(t, ".env", "no-equals-sign\nENVFILE_OK=1\n")
	if n := LoadFiles(filepath.Join(t.TempDir(), "absent.env"), path); n != 1 {
		t.Fatalf("注入键数 = %d，期望 1", n)
	}
	if got := os.Getenv("ENVFILE_OK"); got != "1" {
		t.Fatalf("ENVFILE_OK = %q", got)
	}
}

// .env 中不存在的键不应被清空：LoadFiles 只做新增。
func TestLoadFilesDoesNotUnset(t *testing.T) {
	t.Setenv("ENVFILE_UNTOUCHED", "keep")
	_ = LoadFiles(writeEnv(t, ".env", "ENVFILE_OTHER=1\n"))
	if got := os.Getenv("ENVFILE_UNTOUCHED"); got != "keep" {
		t.Fatalf("无关变量被改动: %q", got)
	}
}
