package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// ResealReport 描述一次存量明文封存的结果。
type ResealReport struct {
	// Files 是实际被重写的文件（不存在则跳过）。
	Files []string
	// SealedLines 是被转成密文的历史明文日志行数。
	SealedLines int
	// KeptLines 是无法解析、原样保留的行数（不解密也不丢数据）。
	KeptLines int
	// Accounts 是主文件里的账号数，用于校对没有写出空池。
	Accounts int
}

// Reseal 把主文件、统计旁路与日志中的存量明文一并封存为当前主密钥的密文。
//
// 为什么需要它：启用加密只影响「之后写入」的数据。主文件与统计在下次
// Save 时会整体重写为 v2，但请求日志是 append-only 的——历史明文行不会
// 因为配置了主密钥就消失（decodeLine 兼容混合格式，读取时也不会报错）。
// 只靠"设个环境变量"会留下长期明文痕迹，所以这里显式重写。
//
// 幂等：已加密的行原样保留（但会用同一把钥匙校验，密钥错配立即报错）。
// 要求先停止正在运行的网关：本方法会 rename 主文件与日志文件。
func (s *Store) Reseal() (ResealReport, error) {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	if len(s.masterKey) == 0 {
		return ResealReport{}, fmt.Errorf("未设置 %s，无法加密", EnvMasterKey)
	}
	key := s.masterKey

	s.mu.RLock()
	snap := s.snapshotLocked()
	accounts := len(s.st.Accounts)
	stats := s.st.Stats
	s.mu.RUnlock()

	rep := ResealReport{Accounts: accounts}

	plain, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return rep, err
	}
	if err := writeAtomicSealed(s.path, plain, key); err != nil {
		return rep, fmt.Errorf("重写状态文件失败: %w", err)
	}
	s.savesTotal.Add(1)
	s.savedBytes.Add(int64(len(plain)))
	rep.Files = append(rep.Files, s.path)

	rawStats, err := json.Marshal(stats)
	if err != nil {
		return rep, err
	}
	if err := writeAtomicSealed(s.statsPath, rawStats, key); err != nil {
		return rep, fmt.Errorf("重写统计文件失败: %w", err)
	}
	rep.Files = append(rep.Files, s.statsPath)

	converted, kept, err := s.logs.reseal(key)
	if err != nil {
		return rep, err
	}
	rep.SealedLines += converted
	rep.KeptLines += kept
	if converted+kept > 0 {
		rep.Files = append(rep.Files, s.logsPath)
	}

	// 轮转文件（<logs>.1）同样可能有明文，一并处理；不存在则跳过。
	rotated := s.logsPath + ".1"
	if c, k, existed, err := resealLogFile(rotated, key); err != nil {
		return rep, err
	} else if existed {
		rep.SealedLines += c
		rep.KeptLines += k
		rep.Files = append(rep.Files, rotated)
	}

	if err := verifySealedStateFile(s.path, key); err != nil {
		return rep, fmt.Errorf("封存校验失败: %w", err)
	}
	if err := verifySealedStateFile(s.statsPath, key); err != nil {
		return rep, fmt.Errorf("封存校验失败: %w", err)
	}
	if err := verifySealedLogFile(s.logsPath); err != nil {
		return rep, fmt.Errorf("封存校验失败: %w", err)
	}
	if err := verifySealedLogFile(rotated); err != nil {
		return rep, fmt.Errorf("封存校验失败: %w", err)
	}

	s.mu.Lock()
	s.lastSaved = append(s.lastSaved[:0], plain...)
	s.encrypted = true
	s.statsDirty = false
	s.mu.Unlock()

	return rep, nil
}

// reseal 重写日志文件并把文件大小同步回内存（轮转判定用）。
func (l *logStore) reseal(key []byte) (converted, kept int, err error) {
	if l == nil {
		return 0, 0, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	converted, kept, _, err = resealLogFile(l.path, key)
	if err != nil {
		return converted, kept, err
	}
	if fi, statErr := os.Stat(l.path); statErr == nil {
		l.size = fi.Size()
	} else {
		l.size = 0
	}
	return converted, kept, nil
}

// resealLogFile 逐行把 JSONL 里的明文封存；existed 表示文件是否存在。
//
// 无法解析成 JSON 的行原样保留：宁可留一行坏数据，也不能因为格式噪声
// 把排障日志整段丢掉。已加密的行会先用同一把钥匙解开校验，避免密钥错配
// 后"看起来封存成功、实际打不开"。
func resealLogFile(path string, key []byte) (converted, kept int, existed bool, err error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, true, err
	}

	var out bytes.Buffer
	out.Grow(len(raw))
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		if isEnvelope(trimmed) {
			if _, _, uerr := unseal(key, trimmed); uerr != nil {
				return converted, kept, true, fmt.Errorf("%s: %w", path, uerr)
			}
			out.Write(trimmed)
			out.WriteByte('\n')
			continue
		}
		if !json.Valid(trimmed) {
			kept++
			out.Write(trimmed)
			out.WriteByte('\n')
			continue
		}
		sealed, serr := sealLine(key, trimmed)
		if serr != nil {
			return converted, kept, true, serr
		}
		out.Write(sealed)
		out.WriteByte('\n')
		converted++
	}

	// 密钥为 nil 时 writeAtomicSealed 只做原子写，不再二次封存。
	if err := writeAtomicSealed(path, out.Bytes(), nil); err != nil {
		return converted, kept, true, err
	}
	return converted, kept, true, nil
}

// verifySealedStateFile 校验主/统计这类整文件 JSON：必须是可用当前钥匙解开的信封。
func verifySealedStateFile(path string, key []byte) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !isEnvelope(raw) {
		return fmt.Errorf("%s 仍是明文", path)
	}
	if _, _, err := unseal(key, raw); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// verifySealedLogFile 校验 JSONL：每一行都必须是信封。
func verifySealedLogFile(path string) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for i, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if !isEnvelope(line) {
			return fmt.Errorf("%s 第 %d 行仍是明文", path, i+1)
		}
	}
	return nil
}
