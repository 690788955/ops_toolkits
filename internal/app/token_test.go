package app

import (
	"encoding/hex"
	"testing"
)

func TestGenerateTokenReturnsStrongRandomValue(t *testing.T) {
	token, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken() 意外报错: %v", err)
	}
	// 16 字节的十六进制编码固定 32 个字符。
	if len(token) != 32 {
		t.Fatalf("token 长度 = %d, want 32 (token=%q)", len(token), token)
	}
	if _, err := hex.DecodeString(token); err != nil {
		t.Fatalf("token 不是合法十六进制: %v (token=%q)", err, token)
	}
}

func TestGenerateTokenIsNotDerivedFromPID(t *testing.T) {
	// 回归保护：历史上随机源失败时会退化成 pid，
	// 这里校验两次调用结果必须不同，且不是纯数字。
	first, err := generateToken()
	if err != nil {
		t.Fatalf("第一次 generateToken() 报错: %v", err)
	}
	second, err := generateToken()
	if err != nil {
		t.Fatalf("第二次 generateToken() 报错: %v", err)
	}
	if first == second {
		t.Fatalf("两次生成的 token 相同，随机性不足: %q", first)
	}
	if isAllDigits(first) {
		t.Fatalf("token 不应是纯数字（疑似降级为 pid）: %q", first)
	}
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
