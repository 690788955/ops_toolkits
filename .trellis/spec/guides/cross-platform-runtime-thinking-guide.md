# Cross-Platform Runtime Thinking Guide

> **Purpose**: Prevent runtime bugs caused by Windows, Git Bash, WSL, and stale runtime path assumptions.

---

## The Problem

Shell execution bugs are often environment bugs, not business logic bugs. A failure that looks like a missing script can be caused by the shell resolver, WSL configuration, or stale docs that still point at the pre-plugin runtime layout.

Common symptoms:

- `WSL getpwnam(<user>) failed`
- `/bin/bash: C:Users...run.sh: No such file or directory`
- A command refers to legacy root paths such as `tools/demo/hello/bin/run.sh`
- A manual verification command uses root `opsctl.exe` instead of `./bin/opsctl.exe`

---

## Current Runtime Contract

This project is plugin-first:

- Runtime config lives at `configs/ops.yaml`.
- Local binaries are built under `bin/`, for example `./bin/opsctl.exe`.
- Runtime tools live under `plugins/<plugin-id>/` and are declared in `plugins/<plugin-id>/plugin.yaml`.
- Demo tool IDs use the plugin namespace, for example `plugin.demo.greet` and `plugin.demo.confirmed`.

Do not treat these legacy paths as current runtime structure:

- Root `ops.yaml`
- Root `opsctl.exe`
- Root `tools/`
- Root `workflows/`
- `tools/demo/hello`
- `tools/demo/sample-tool`
- `workflows/demo-hello`

---

## Before Changing Runtime Commands or Docs

### 1. Identify the Shell Boundary

When a `.sh` file runs on Windows, the framework may invoke `bash` from `PATH`. That `bash` can be Git Bash, MSYS2, Cygwin, or WSL. These shells do not handle Windows paths the same way.

Ask:

- Which `bash` is first on `PATH` for the failing process?
- Is it Git Bash or WSL bash?
- If it is WSL, is the default WSL user valid?
- Is the command path passed as a Windows path, a POSIX path, or a malformed hybrid path?

### 2. Separate Environment Failures from Framework Path Failures

For Windows `.sh` errors, classify the failure before editing code:

| Symptom | First Suspect | Check |
|---------|---------------|-------|
| `getpwnam(<user>) failed` | Broken WSL default user | Validate WSL user configuration before changing runner logic |
| `/bin/bash: C:Users...: No such file or directory` | Windows path passed to WSL/Git Bash incorrectly | Check shell type and path conversion |
| Path contains `tools/demo/...` | Stale runtime path, old binary, old config, or stale scaffold docs | Search repo docs/config/templates before changing runner logic |
| Root `opsctl.exe` is used | Stale command | Use `./bin/opsctl.exe` |

### 3. Convert Windows Paths Deliberately

Do not assume a Windows path can be passed directly to WSL bash.

- Windows path: `C:\Users\Jonathan\Desktop\ccb\ops_toolkits\plugins\demo\scripts\run.sh`
- WSL path: `/mnt/c/Users/Jonathan/Desktop/ccb/ops_toolkits/plugins/demo/scripts/run.sh`
- Git Bash path: `/c/Users/Jonathan/Desktop/ccb/ops_toolkits/plugins/demo/scripts/run.sh`

If a command may run through WSL bash, convert paths explicitly for WSL or avoid WSL bash for project-local scripts.

### 4. Search for Legacy Runtime References

Before concluding that runtime code is wrong, search for stale references in docs, tests, scaffold examples, generated dev kits, and local command notes.

Search terms to include:

```bash
tools/demo/hello
tools/demo/sample-tool
workflows/demo-hello
root opsctl.exe
./opsctl.exe
ops.yaml
```

Then decide whether the failing path came from:

- an old binary,
- an old config file,
- a stale scaffold/dev-kit example,
- stale documentation,
- or the current runner.

---

## Safe Manual Verification Commands

Use plugin-first commands when reproducing runtime behavior:

```bash
GOTOOLCHAIN=local go build -o "bin/opsctl.exe" "./cmd/opsctl"
./bin/opsctl.exe validate
./bin/opsctl.exe list
./bin/opsctl.exe run tool plugin.demo.greet --set name=Tester --set message=Hello --no-prompt
./bin/opsctl.exe run tool plugin.demo.confirmed --set target=demo --no-prompt
```

Do not use legacy commands that imply root runtime structure.

---

## Checklist

Before changing runtime execution logic, docs, scaffolds, or verification commands:

- [ ] Confirmed the command uses `./bin/opsctl.exe`, not root `opsctl.exe`.
- [ ] Confirmed the tool/workflow ID is plugin-first, for example `plugin.demo.*`.
- [ ] Searched for legacy runtime paths in docs, tests, scaffolds, templates, and config.
- [ ] Determined whether `bash` resolves to Git Bash, WSL, or another shell.
- [ ] If WSL is involved, checked default user health before blaming project code.
- [ ] Verified whether Windows paths need conversion to WSL or Git Bash form.
- [ ] Avoided reintroducing root `tools/`, root `workflows/`, root `ops.yaml`, or root `opsctl.exe` assumptions.
