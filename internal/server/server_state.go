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
	if record.Status != "running" || s.hasActiveRun(record.ID) {
		return record, nil
	}
	record.EndedAt = time.Now()
	record.Status = "failed"
	record.Error = "运行任务已异常退出或服务已重启，当前进程没有该任务的运行句柄。"
	for index := range record.Steps {
		if record.Steps[index].Status != "running" && record.Steps[index].Status != "waiting" {
			continue
		}
		record.Steps[index].Status = "failed"
		record.Steps[index].EndedAt = record.EndedAt
		record.Steps[index].Error = "运行任务已异常退出或服务已重启。"
	}
	if err := saveRunRecord(reg, &record); err != nil {
		return runner.RunRecord{}, err
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
		if item.Status != "running" || s.hasActiveRun(item.ID) {
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
