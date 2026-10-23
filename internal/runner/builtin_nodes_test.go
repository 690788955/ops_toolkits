package runner

import (
	"archive/zip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"shell_ops/internal/config"
)

func TestRunWorkflowShellNodeExecutesScriptOnTarget(t *testing.T) {
	r, dir := newHTTPTestRunner(t)
	outFile := filepath.ToSlash(filepath.Join(dir, "shell-out.txt"))
	wf := &config.WorkflowConfig{
		ID:     "demo.shell",
		Stages: []config.WorkflowStage{{ID: "prepare"}},
		Nodes: []config.WorkflowNode{{
			ID:      "script",
			Type:    config.WorkflowNodeTypeShell,
			StageID: "prepare",
			Order:   10,
			Script: config.WorkflowScript{
				Script: "printf '%s-%s' \"$MODE\" \"$OPS_PARAM_TARGET\" > \"$OUT_FILE\"\necho done",
				Env:    map[string]string{"MODE": "{{ .mode }}", "OUT_FILE": outFile},
			},
		}},
	}
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, map[string]string{"mode": "fast"}, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	if record.Status != "succeeded" || len(record.Steps) != 1 || record.Steps[0].Status != "succeeded" {
		t.Fatalf("record = %+v", record)
	}
	content := readFile(t, outFile)
	if content != "fast-" {
		t.Fatalf("script output = %q, want fast-", content)
	}
}

func TestRunWorkflowShellNodeReportsNonZeroExit(t *testing.T) {
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID:     "demo.shell-fail",
		Stages: []config.WorkflowStage{{ID: "prepare"}},
		Nodes: []config.WorkflowNode{{
			ID:      "script",
			Type:    config.WorkflowNodeTypeShell,
			StageID: "prepare",
			Order:   10,
			Script:  config.WorkflowScript{Script: "echo boom >&2\nexit 7"},
		}},
	}
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err == nil || record.Status != "failed" {
		t.Fatalf("err = %v status = %s, want failure", err, record.Status)
	}
	stderr := readTextFile(filepath.Join(r.RunsDir, record.ID, "script", "stderr.log"))
	if !strings.Contains(stderr, "boom") {
		t.Fatalf("stderr = %q, want command output", stderr)
	}
}

func TestRunWorkflowChecksumNodeComparesDigest(t *testing.T) {
	r, dir := newHTTPTestRunner(t)
	target := filepath.ToSlash(filepath.Join(dir, "payload.txt"))
	if err := os.WriteFile(target, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wf := &config.WorkflowConfig{
		ID:     "demo.checksum",
		Stages: []config.WorkflowStage{{ID: "prepare"}},
		Nodes: []config.WorkflowNode{
			{ID: "ok", Type: config.WorkflowNodeTypeChecksum, StageID: "prepare", Order: 10, Checksum: config.WorkflowChecksum{Path: target}},
		},
	}
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	checksumPath := filepath.Join(r.RunsDir, record.ID, "ok", "checksum.json")
	data, readErr := os.ReadFile(checksumPath)
	if readErr != nil {
		t.Fatalf("read checksum.json: %v", readErr)
	}
	if !strings.Contains(string(data), `"matched": true`) {
		t.Fatalf("checksum summary = %s", data)
	}

	wf.Nodes[0].Checksum = config.WorkflowChecksum{Path: target, Algorithm: "sha256", Expected: strings.Repeat("0", 64)}
	record, err = r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err == nil || record.Status != "failed" {
		t.Fatalf("mismatched digest should fail, err = %v status = %s", err, record.Status)
	}
	if !strings.Contains(err.Error(), "摘要不一致") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunWorkflowArchiveNodeExtractsZipWithoutEscape(t *testing.T) {
	r, dir := newHTTPTestRunner(t)
	archivePath := filepath.Join(dir, "bundle.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, _ := writer.Create("pkg/app.conf")
	_, _ = entry.Write([]byte("port=8080\n"))
	escape, _ := writer.Create("../escape.txt")
	_, _ = escape.Write([]byte("bad"))
	_ = writer.Close()
	_ = file.Close()

	target := filepath.Join(dir, "out")
	wf := &config.WorkflowConfig{
		ID:     "demo.archive",
		Stages: []config.WorkflowStage{{ID: "prepare"}},
		Nodes: []config.WorkflowNode{{
			ID:      "unzip",
			Type:    config.WorkflowNodeTypeArchive,
			StageID: "prepare",
			Order:   10,
			Archive: config.WorkflowArchive{Path: filepath.ToSlash(archivePath), TargetDir: filepath.ToSlash(target)},
		}},
	}
	record, runErr := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if runErr == nil {
		t.Fatalf("zip entry escaping the target should fail the node, record status = %s", record.Status)
	}
	if !strings.Contains(runErr.Error(), "逃逸目标目录") {
		t.Fatalf("runErr = %v", runErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "escape.txt")); statErr == nil {
		t.Fatalf("archive entry escaped the target directory")
	}
}

func TestRunWorkflowArchiveNodeExtractsFiles(t *testing.T) {
	r, dir := newHTTPTestRunner(t)
	archivePath := filepath.Join(dir, "bundle.zip")
	file, _ := os.Create(archivePath)
	writer := zip.NewWriter(file)
	entry, _ := writer.Create("pkg/app.conf")
	_, _ = entry.Write([]byte("port=8080\n"))
	_ = writer.Close()
	_ = file.Close()

	target := filepath.Join(dir, "out")
	wf := &config.WorkflowConfig{
		ID:     "demo.archive-ok",
		Stages: []config.WorkflowStage{{ID: "prepare"}},
		Nodes: []config.WorkflowNode{{
			ID:      "unzip",
			Type:    config.WorkflowNodeTypeArchive,
			StageID: "prepare",
			Order:   10,
			Archive: config.WorkflowArchive{Path: filepath.ToSlash(archivePath), TargetDir: filepath.ToSlash(target), Strip: 1},
		}},
	}
	if _, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{}); err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	if content := readFile(t, filepath.Join(target, "app.conf")); content != "port=8080\n" {
		t.Fatalf("extracted content = %q", content)
	}
}

func TestRunWorkflowArchiveNodeTrimsRenderedPath(t *testing.T) {
	r, dir := newHTTPTestRunner(t)
	archivePath := filepath.Join(dir, "bundle-trim.zip")
	file, _ := os.Create(archivePath)
	writer := zip.NewWriter(file)
	entry, _ := writer.Create("app.conf")
	_, _ = entry.Write([]byte("port=8080\n"))
	_ = writer.Close()
	_ = file.Close()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "path.txt"), []byte(archivePath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(dir, "out-trim")
	wf := &config.WorkflowConfig{
		ID:     "demo.archive-trim",
		Stages: []config.WorkflowStage{{ID: "prepare"}},
		Nodes: []config.WorkflowNode{{
			ID:      "extract",
			Type:    config.WorkflowNodeTypeShell,
			StageID: "prepare",
			Order:   10,
			Script: config.WorkflowScript{
				Script: "cat \"$PATH_FILE\"",
				Env:    map[string]string{"PATH_FILE": filepath.ToSlash(filepath.Join(dir, "src", "path.txt"))},
			},
		}, {
			ID:      "unzip",
			Type:    config.WorkflowNodeTypeArchive,
			StageID: "prepare",
			Order:   20,
			Archive: config.WorkflowArchive{Path: "{{ .steps.extract.stdout }}\n", TargetDir: " " + filepath.ToSlash(target) + " "},
		}},
	}
	if _, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{}); err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	if content := readFile(t, filepath.Join(target, "app.conf")); content != "port=8080\n" {
		t.Fatalf("extracted content = %q", content)
	}
}

func TestRunWorkflowWaitNodePollsUntilHealthy(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID:     "demo.wait",
		Stages: []config.WorkflowStage{{ID: "verify"}},
		Nodes: []config.WorkflowNode{{
			ID:      "poll",
			Type:    config.WorkflowNodeTypeWait,
			StageID: "verify",
			Order:   10,
			Wait:    config.WorkflowWait{PollURL: server.URL, Interval: "10ms", MaxAttempts: 20, ExpectStatus: []int{200}},
		}},
	}
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, nilWriter{}, nilWriter{})
	if err != nil {
		t.Fatalf("RunWorkflow error: %v", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("poll calls = %d, want 3", calls)
	}
	meta := readWaitMeta(filepath.Join(r.RunsDir, record.ID, "poll", waitStepMetaFile))
	if !meta.Matched || meta.Attempts != 3 {
		t.Fatalf("wait meta = %+v", meta)
	}
}

func TestRunWorkflowWaitNodeCancelsImmediately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{
		ID:     "demo.wait-cancel",
		Stages: []config.WorkflowStage{{ID: "verify"}},
		Nodes: []config.WorkflowNode{{
			ID:      "poll",
			Type:    config.WorkflowNodeTypeWait,
			StageID: "verify",
			Order:   10,
			Wait:    config.WorkflowWait{PollURL: server.URL, Interval: "50ms", MaxAttempts: 200},
		}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := time.Now()
	record, err := r.RunWorkflowConfigWithConfirmation(ctx, wf, nil, false, nilWriter{}, nilWriter{})
	if err == nil {
		t.Fatalf("cancelled wait should fail")
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("cancel took %v, want immediate effect", elapsed)
	}
	if record.Status != "cancelled" && record.Steps[0].Status != "cancelled" {
		t.Fatalf("status = %s step = %s, want cancelled", record.Status, record.Steps[0].Status)
	}
}
