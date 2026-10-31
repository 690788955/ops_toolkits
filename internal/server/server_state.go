package server

// 本文件是 serverState 的生命周期与运行记录持久化。
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"shell_ops/internal/market"
	"shell_ops/internal/registry"
	"shell_ops/internal/remoteexec"
	"shell_ops/internal/runbundle"
	"shell_ops/internal/runner"
	"shell_ops/internal/sshmgr"
)

func newServerState(reg *registry.Registry) *serverState {
	client := market.NewClient()
	client.BaseDir = reg.BaseDir
	return &serverState{
		reg:    reg,
		market: client,
		ssh:    sshmgr.NewManager(reg.BaseDir),
		active: map[string]context.CancelFunc{},
	}
}

func (s *serverState) registry() *registry.Registry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reg
}

func (s *serverState) newRunner(reg *registry.Registry) *runner.Runner {
	r := runner.New(reg)
	r.SetRemoteExecutor(remoteexec.New(s.ssh))
	return r
}

func (s *serverState) swap(reg *registry.Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reg = reg
}

func (s *serverState) registerRun(id string, cancel context.CancelFunc) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.active[id] = cancel
}

func (s *serverState) finishRun(id string) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	delete(s.active, id)
}

func (s *serverState) hasActiveRun(id string) bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	return s.active[id] != nil
}

// claimRun 原子领取运行上的恢复动作；重复请求或运行中的任务会被拒绝。
func (s *serverState) claimRun(id string) (context.Context, context.CancelFunc, bool) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.active[id] != nil {
		return nil, nil, false
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.active[id] = cancel
	return ctx, cancel, true
}

func (s *serverState) cancelRun(id string) bool {
	s.runMu.Lock()
	cancel := s.active[id]
	s.runMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (s *serverState) reconcileRunRecord(reg *registry.Registry, id string) (runner.RunRecord, error) {
	s.runFileMu.Lock()
	defer s.runFileMu.Unlock()
	record, err := runbundle.LoadRecord(reg, id)
	if err != nil {
		return runner.RunRecord{}, err
	}
	if s.hasActiveRun(record.ID) {
		return record, nil
	}
	interrupted := markInterruptedRecoveryAttempts(&record)
	if record.Status != "running" {
		if interrupted {
			if err := saveRunRecord(reg, &record); err != nil {
				return runner.RunRecord{}, err
			}
		}
		return record, nil
	}
	record.EndedAt = time.Now()
	record.Status = "failed"
	record.Error = "运行任务不在当前进程执行：可能已异常退出、服务已重启，或该运行由 CLI 等其它进程启动；当前进程没有它的运行句柄。"
	for index := range record.Steps {
		if record.Steps[index].Status != "running" && record.Steps[index].Status != "waiting" {
			continue
		}
		record.Steps[index].Status = "failed"
		record.Steps[index].EndedAt = record.EndedAt
		record.Steps[index].Error = "运行任务已异常退出或服务已重启。"
	}
	// 审批闸门只存在于进程内，重启后没有可确认的对象，按作废记录，避免前端继续显示“等待审批”。
	for index := range record.Approvals {
		if record.Approvals[index].Status != runner.ApprovalStatusWaiting {
			continue
		}
		record.Approvals[index].Status = runner.ApprovalStatusCancelled
		record.Approvals[index].DecidedAt = record.EndedAt
		record.Approvals[index].Note = "该运行不在当前进程（已异常退出、服务已重启，或由 CLI 等其它进程启动），等待中的审批已作废"
	}
	if err := saveRunRecord(reg, &record); err != nil {
		return runner.RunRecord{}, err
	}
	return record, nil
}

func markInterruptedRecoveryAttempts(record *runner.RunRecord) bool {
	changed := false
	for index := range record.Attempts {
		if record.Attempts[index].Status != "running" {
			continue
		}
		record.Attempts[index].Status = "failed"
		record.Attempts[index].EndedAt = time.Now()
		record.Attempts[index].Error = "恢复动作已异常退出或服务已重启，请检查本次日志后继续处理。"
		changed = true
	}
	return changed
}

// 仅在已领取运行后调用，此时遗留 running attempt 一定来自已中断的恢复动作。
func loadClaimedRunRecord(state *serverState, reg *registry.Registry, id string) (runner.RunRecord, error) {
	record, err := loadRunRecord(state, reg, id)
	if err != nil {
		return record, err
	}
	if markInterruptedRecoveryAttempts(&record) {
		if err := saveRunRecord(reg, &record); err != nil {
			return record, err
		}
	}
	return record, nil
}

func (s *serverState) reconcileOrphanRuns(reg *registry.Registry) {
	if s == nil || reg == nil {
		return
	}
	items, err := s.listRunSummaries(reg)
	if err != nil {
		return
	}
	for _, item := range items {
		if (item.Status != "running" && item.Status != runner.RunStatusPending) || s.hasActiveRun(item.ID) {
			continue
		}
		_, _ = s.reconcileRunRecord(reg, item.ID)
	}
}

func (s *serverState) startRunReconciler() {
	go func() {
		s.reconcileOrphanRuns(s.registry())
	}()
}

func saveRunRecord(reg *registry.Registry, record *runner.RunRecord) error {
	if record == nil || strings.TrimSpace(record.ID) == "" {
		return fmt.Errorf("运行记录 ID 不能为空")
	}
	runDir, err := runbundle.RunDir(reg, record.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	tempPath := filepath.Join(runDir, fmt.Sprintf("result.json.%d.tmp", time.Now().UnixNano()))
	resultPath := filepath.Join(runDir, "result.json")
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < 25; attempt++ {
		if err := os.Rename(tempPath, resultPath); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if err := os.Remove(resultPath); err != nil && !os.IsNotExist(err) {
			lastErr = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if err := os.Rename(tempPath, resultPath); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = os.Remove(tempPath)
	return lastErr
}
