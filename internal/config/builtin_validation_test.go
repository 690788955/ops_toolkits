package config

import (
	"math"
	"strings"
	"testing"
)

func TestValidateScriptConfigRejectsInvalidTimeoutAndEnvironment(t *testing.T) {
	for _, timeout := range []string{"0s", "-1s", "bad"} {
		t.Run(timeout, func(t *testing.T) {
			if _, err := ValidateScriptConfig("shell", WorkflowScript{Script: "true", Timeout: timeout}); err == nil {
				t.Fatal("无效超时不应回落到默认执行时长")
			}
		})
	}
	for _, name := range []string{"", " A", "A=B", "A-B", "1A", "A\nB"} {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateScriptConfig("shell", WorkflowScript{Script: "true", Env: map[string]string{name: "value"}}); err == nil {
				t.Fatal("Shell 不能接收非法环境变量名")
			}
		})
	}
	if got, err := ValidateScriptConfig("shell", WorkflowScript{Script: "true", Timeout: " 2s ", Env: map[string]string{"_APP_1": "value"}}); err != nil || got.Timeout != "2s" {
		t.Fatalf("有效配置未归一化: %+v, %v", got, err)
	}
}

func TestValidateChecksumConfigPreservesTemplateCase(t *testing.T) {
	const expected = "{{ .steps.Hash.stdout }}"
	got, err := ValidateChecksumConfig("digest", WorkflowChecksum{Path: "file", Algorithm: " SHA256 ", Expected: expected})
	if err != nil || got.Expected != expected || got.Algorithm != "sha256" {
		t.Fatalf("模板引用被拒绝或改变: %+v, %v", got, err)
	}
	got, err = ValidateChecksumConfig("digest", WorkflowChecksum{Path: "file", Algorithm: "MD5", Expected: strings.Repeat("A", 32)})
	if err != nil || got.Expected != strings.Repeat("a", 32) {
		t.Fatalf("字面摘要归一化失败: %+v, %v", got, err)
	}
	if _, err := ValidateChecksumConfig("digest", WorkflowChecksum{Path: "file", Expected: "invalid"}); err == nil {
		t.Fatal("无效字面摘要应被拒绝")
	}
}

func TestValidateApprovalConfigNormalizesAndRejectsBadTimeout(t *testing.T) {
	for _, timeout := range []string{"0s", "-1m", "later"} {
		t.Run(timeout, func(t *testing.T) {
			if _, err := ValidateApprovalConfig("gate", WorkflowApproval{Message: "确认", Timeout: timeout}); err == nil {
				t.Fatal("无效等待时长不应被接受")
			}
		})
	}
	got, err := ValidateApprovalConfig("gate", WorkflowApproval{Message: "  改好配置后继续  ", Timeout: " 30m "})
	if err != nil || got.Message != "改好配置后继续" || got.Timeout != "30m" {
		t.Fatalf("有效审批配置未归一化: %+v, %v", got, err)
	}
	// 留空表示一直等待，不能按“0 秒超时”理解。
	if got, err := ValidateApprovalConfig("gate", WorkflowApproval{Message: "确认"}); err != nil || got.Timeout != "" {
		t.Fatalf("空超时被拒绝: %+v, %v", got, err)
	}
	long := strings.Repeat("审", approvalMessageMaxRunes+1)
	if _, err := ValidateApprovalConfig("gate", WorkflowApproval{Message: long}); err == nil {
		t.Fatal("超长提示应被拒绝")
	}
}

func TestValidateWaitConfigRejectsInvalidLimits(t *testing.T) {
	for name, wait := range map[string]WorkflowWait{
		"zero_interval":     {PollURL: "https://example.invalid", Interval: "0s"},
		"negative_interval": {PollFile: "ready", Interval: "-1s"},
		"zero_max_wait":     {PollFile: "ready", MaxWait: "0s"},
		"negative_max_wait": {PollFile: "ready", MaxWait: "-1s"},
		"negative_attempts": {PollFile: "ready", MaxAttempts: -1},
		"negative_seconds":  {PollFile: "ready", Seconds: -1},
		"nan_seconds":       {Seconds: math.NaN()},
		"infinite_seconds":  {Seconds: math.Inf(1)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateWaitConfig("poll", wait); err == nil {
				t.Fatal("无效等待边界应被拒绝")
			}
		})
	}
	if _, err := ValidateWaitConfig("poll", WorkflowWait{PollFile: "ready", Interval: " 1ms ", MaxWait: " 1s "}); err != nil {
		t.Fatalf("有效等待配置被拒绝: %v", err)
	}
}
