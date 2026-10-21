package remoteexec

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shell_ops/internal/runner"
)

type bundleEntry struct {
	Mode     int64
	Typeflag byte
	Data     []byte
}

func readBundle(t *testing.T, file *os.File) map[string]bundleEntry {
	t.Helper()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek bundle: %v", err)
	}
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("open gzip: %v", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	entries := map[string]bundleEntry{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar: %v", err)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read tar entry %s: %v", header.Name, err)
		}
		entries[strings.TrimSuffix(header.Name, "/")] = bundleEntry{
			Mode:     header.Mode,
			Typeflag: header.Typeflag,
			Data:     data,
		}
	}
	return entries
}

func TestBuildBundlePackagesToolAndParameters(t *testing.T) {
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "tools", "demo", "hello")
	if err := os.MkdirAll(filepath.Join(toolDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(toolDir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "bin", "run.sh"), []byte("#!/usr/bin/env bash\necho hello\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "data", "sample.txt"), []byte("sample\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paramFile := filepath.Join(dir, "params.conf")
	if err := os.WriteFile(paramFile, []byte("name=demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	globalEnv := filepath.Join(dir, "global-env.conf")
	if err := os.WriteFile(globalEnv, []byte("REGION=cn\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	bundle, err := buildBundle(runner.RemoteToolRequest{
		HostID:        "host-1",
		ToolID:        "demo.hello",
		ToolDir:       toolDir,
		Entry:         filepath.Join(toolDir, "bin", "run.sh"),
		Workdir:       toolDir,
		Args:          []string{"--name", "O'Reilly"},
		Env:           []string{"GREETING=hello world", "INVALID-KEY=x", "NO_EQUALS"},
		ParamFile:     paramFile,
		ParamName:     filepath.Base(paramFile),
		GlobalEnvFile: globalEnv,
	})
	if err != nil {
		t.Fatalf("buildBundle error: %v", err)
	}
	defer func() {
		_ = bundle.Close()
		_ = os.Remove(bundle.Name())
	}()

	entries := readBundle(t, bundle)
	script, ok := entries["run.sh"]
	if !ok {
		t.Fatalf("bundle missing run.sh: %v", entries)
	}
	if script.Mode != 0o755 {
		t.Fatalf("run.sh mode = %o, want 755", script.Mode)
	}
	text := string(script.Data)
	for _, want := range []string{
		"#!/bin/sh\n",
		"set -e\n",
		`BASE="$(cd "$(dirname "$0")" && pwd)"`,
		"cd \"$BASE\"/'tool'\n",
		"export GREETING='hello world'\n",
		"export OPS_PARAM_FILE=\"$BASE\"/'params/params.conf'\n",
		"export OPS_GLOBAL_ENV_FILE=\"$BASE\"/'global-env.conf'\n",
		`for candidate in bash /bin/bash /usr/bin/bash /usr/local/bin/bash /opt/bin/bash /opt/homebrew/bin/bash; do`,
		`if [ -z "$OPS_BASH" ]; then`,
		`exec "$OPS_BASH" "$BASE"/'tool/bin/run.sh' '--name' 'O'\''Reilly'` + "\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("run.sh missing %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"INVALID-KEY", "NO_EQUALS"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("run.sh should filter %q:\n%s", unwanted, text)
		}
	}

	for _, name := range []string{"tool/bin", "tool/data"} {
		entry, ok := entries[name]
		if !ok {
			t.Fatalf("bundle missing directory %s: %v", name, entries)
		}
		if entry.Typeflag != tar.TypeDir {
			t.Fatalf("%s typeflag = %d, want %d", name, entry.Typeflag, tar.TypeDir)
		}
	}
	if got := string(entries["tool/bin/run.sh"].Data); !strings.Contains(got, "echo hello") {
		t.Fatalf("tool script content = %q", got)
	}
	if got := string(entries["tool/data/sample.txt"].Data); got != "sample\n" {
		t.Fatalf("tool data content = %q", got)
	}

	param := entries["params/params.conf"]
	if param.Mode != 0o600 || string(param.Data) != "name=demo\n" {
		t.Fatalf("param entry = mode %o data %q", param.Mode, param.Data)
	}
	env := entries["global-env.conf"]
	if env.Mode != 0o600 || string(env.Data) != "REGION=cn\n" {
		t.Fatalf("global env entry = mode %o data %q", env.Mode, env.Data)
	}
}

func TestBuildBundleNormalizesCRLFForRemoteScripts(t *testing.T) {
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "tool")
	if err := os.MkdirAll(filepath.Join(toolDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "scripts", "run.sh"), []byte("#!/usr/bin/env bash\r\nset -euo pipefail\r\necho ok\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "scripts", "runner"), []byte("#!/bin/sh\r\necho ok\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "app.conf"), []byte("KEY=value\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	globalEnv := filepath.Join(dir, "global-env.conf")
	if err := os.WriteFile(globalEnv, []byte("REGION=cn\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle, err := buildBundle(runner.RemoteToolRequest{
		HostID:        "host-1",
		ToolID:        "demo.crlf",
		ToolDir:       toolDir,
		Entry:         filepath.Join(toolDir, "scripts", "run.sh"),
		Workdir:       toolDir,
		GlobalEnvFile: globalEnv,
	})
	if err != nil {
		t.Fatalf("buildBundle error: %v", err)
	}
	defer func() {
		_ = bundle.Close()
		_ = os.Remove(bundle.Name())
	}()
	entries := readBundle(t, bundle)
	for name, want := range map[string]string{
		"tool/scripts/run.sh": "#!/usr/bin/env bash\nset -euo pipefail\necho ok\n",
		"tool/scripts/runner": "#!/bin/sh\necho ok\n",
		"tool/app.conf":       "KEY=value\n",
		"global-env.conf":     "REGION=cn\n",
	} {
		if got := string(entries[name].Data); got != want {
			t.Fatalf("%s content = %q, want %q", name, got, want)
		}
	}
}

func TestBuildBundleExportsGlobalEnvValues(t *testing.T) {
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "tool")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "run.sh"), []byte("#!/usr/bin/env bash\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bundle, err := buildBundle(runner.RemoteToolRequest{
		HostID:  "host-1",
		ToolID:  "demo.env",
		ToolDir: toolDir,
		Entry:   filepath.Join(toolDir, "run.sh"),
		Workdir: toolDir,
		Env:     []string{"OPS_PARAM_NAME=demo"},
		GlobalEnv: []string{
			"REGION=cn",
			"WORD=你好",
			"INVALID-KEY=x",
			"NO_EQUALS",
		},
	})
	if err != nil {
		t.Fatalf("buildBundle error: %v", err)
	}
	defer func() {
		_ = bundle.Close()
		_ = os.Remove(bundle.Name())
	}()
	text := string(readBundle(t, bundle)["run.sh"].Data)
	for _, want := range []string{"export REGION='cn'\n", "export WORD='你好'\n", "export OPS_PARAM_NAME='demo'\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("run.sh missing %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"INVALID-KEY", "NO_EQUALS"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("run.sh should filter %q:\n%s", unwanted, text)
		}
	}
}

func TestBuildBundleRunsPOSIXShellEntryWithoutBash(t *testing.T) {
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "tool")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "run.sh"), []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bundle, err := buildBundle(runner.RemoteToolRequest{
		HostID:  "host-1",
		ToolID:  "demo.posix",
		ToolDir: toolDir,
		Entry:   filepath.Join(toolDir, "run.sh"),
		Workdir: toolDir,
	})
	if err != nil {
		t.Fatalf("buildBundle error: %v", err)
	}
	defer func() {
		_ = bundle.Close()
		_ = os.Remove(bundle.Name())
	}()
	text := string(readBundle(t, bundle)["run.sh"].Data)
	if !strings.Contains(text, `exec /bin/sh "$BASE"/'tool/run.sh'`) {
		t.Fatalf("run.sh should exec /bin/sh for POSIX entry:\n%s", text)
	}
	if strings.Contains(text, "OPS_BASH") {
		t.Fatalf("run.sh should not probe bash for POSIX entry:\n%s", text)
	}
}

func TestBuildBundleRejectsWindowsPaths(t *testing.T) {
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "tool")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "run.sh"), []byte("echo ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := runner.RemoteToolRequest{
		HostID:  "host-1",
		ToolID:  "demo.tool",
		ToolDir: toolDir,
		Entry:   filepath.Join(toolDir, "run.sh"),
	}

	withWorkdir := base
	withWorkdir.Workdir = `C:\work`
	if _, err := buildBundle(withWorkdir); err == nil || !strings.Contains(err.Error(), "Windows 本机工作目录") {
		t.Fatalf("windows workdir error = %v", err)
	}

	withEntry := base
	withEntry.Entry = `C:\tools\run.sh`
	if _, err := buildBundle(withEntry); err == nil || !strings.Contains(err.Error(), "Windows 本机路径") {
		t.Fatalf("windows entry error = %v", err)
	}

	withExe := base
	withExe.Entry = filepath.Join(toolDir, "run.exe")
	if _, err := buildBundle(withExe); err == nil || !strings.Contains(err.Error(), "Windows 可执行文件") {
		t.Fatalf("windows executable error = %v", err)
	}
}

func TestRemoteCommandUsesBundleBaseDir(t *testing.T) {
	fallback := remoteCommand("", false)
	for _, want := range []string{
		`base="${TMPDIR:-/tmp}"`,
		`mkdir -p "$base" || exit 1`,
		`d=$(mktemp -d "$base/opsctl.XXXXXX") || exit 1`,
		`trap '\''rm -rf "$d"'\'' EXIT`,
	} {
		if !strings.Contains(fallback, want) {
			t.Fatalf("default command missing %q:\n%s", want, fallback)
		}
	}

	custom := remoteCommand("/data/ops cache", false)
	if !strings.Contains(custom, `base='\''/data/ops cache'\''`) {
		t.Fatalf("command should use configured base dir:\n%s", custom)
	}
	if strings.Contains(custom, "TMPDIR") {
		t.Fatalf("configured base dir should not fall back to TMPDIR:\n%s", custom)
	}
	if !strings.Contains(custom, `mkdir -p "$base" || exit 1`) {
		t.Fatalf("command should create configured base dir:\n%s", custom)
	}
}

func TestRemoteCommandKeepBundle(t *testing.T) {
	keep := remoteCommand("", true)
	if strings.Contains(keep, "trap") {
		t.Fatalf("keep bundle command should not register cleanup trap:\n%s", keep)
	}
	if !strings.Contains(keep, "下发文件保留在 $d") {
		t.Fatalf("keep bundle command should print retained dir:\n%s", keep)
	}
	if !strings.Contains(keep, `tar -xzf - -C "$d" || exit 1`) {
		t.Fatalf("keep bundle command should still extract bundle:\n%s", keep)
	}
}

func TestBuildBundleRequiresToolDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	_, err := buildBundle(runner.RemoteToolRequest{ToolDir: missing, Entry: "run.sh"})
	if err == nil || !strings.Contains(err.Error(), "读取工具目录") {
		t.Fatalf("missing tool dir error = %v", err)
	}
}
