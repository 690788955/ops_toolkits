# Research: Ansible Playbook Plugin Patterns

- **Query**: Research how comparable automation/plugin systems support Ansible playbooks as executable units, and map the findings to this repo's plugin-first shell ops framework. Include 2-4 comparable patterns/tools, common conventions (playbook path, inventory, extra-vars, working dir, dependency assumptions), repo constraints (plugin-local paths, command/args, config_files, packaging), and a concise recommendation.
- **Scope**: mixed
- **Date**: 2026-05-07

## Findings

### Files Found

| File Path | Description |
|---|---|
| `README.md` | 项目级插件接入、`plugin.yaml` 示例、插件路径安全、`config_dir` / `config_files` 约束与 `opsctl package build` 打包内容。 |
| `plugins/plugin.template/plugin.yaml` | 当前插件工具 manifest 示例：`command`、`args`、`workdir`、`parameters`、`config_dir`、`config_files` 的实际写法。 |
| `plugins/plugin.template/README.md` | 插件模板指南，说明配置文件声明只用于 Web/API 编辑声明，不会自动传参、复制或生成文件；脚本需通过 args/默认路径自行读取。 |
| `internal/plugin/types.go` | 插件 manifest Go 结构，`Tool` 支持 `Command`、`Args`、`Workdir`、`PassMode`、`ConfigDir`、`ConfigFiles`、`Env` 等字段。 |
| `internal/plugin/load.go` | 插件加载校验：工具 ID 前缀、`command` 必填且必须存在、`command` / `workdir` / `config_dir` / `config_files` 不得逃逸插件目录。 |
| `internal/registry/registry.go` | 插件工具归一化为 shell 执行配置；默认 `workdir: .`，默认 `config_dir: config`，默认参数传递 `pass_mode.env=true`。 |
| `internal/runner/runner.go` | 工具执行逻辑：入口为 `tool.Dir + execution.entry`，工作目录为插件目录或插件内 `workdir`，渲染 `args`，写 `OPS_PARAM_*` 环境变量，可选 `OPS_PARAM_FILE`。 |
| `internal/packagebuild/packagebuild.go` | 离线包构建只复制 `configs/`、`plugins/` 和当前 `opsctl` 可执行文件，不打包系统级依赖。 |
| `.trellis/spec/backend/plugin-import-export.md` | 插件导入/导出和 host absolute config mapping 的可执行契约；导出 ZIP 只能包含插件目录内文件，host 绝对配置目标不得包含进插件包。 |

### Code Patterns

#### Comparable automation/plugin systems

1. **Ansible Automation Controller / AWX Job Template**
   - Pattern: 把 playbook 作为“项目(Project) + Job Template”的可复用执行单元。Job Template 固化 `Project`、`Inventory`、`Playbook`、`Credentials`、job type（Run/Check）、limit、verbosity、extra variables 等字段，并允许部分字段在 launch 时 prompt/override。
   - External reference: [Automation Controller User Guide v4.5 - Job Templates](https://legacy-controller-docs.ansible.com/automation-controller/4.5/html/userguide/job_templates.html) — 文档定义 Job Template 是“一组用于运行 Ansible job 的参数”，用于重复执行同一 job；创建字段包括 Job Type、Inventory、Project、SCM Branch、Execution Environment、Credentials、Variables 等。
   - Common conventions:
     - Playbook path: 从 Project 的源码目录中选择 playbook；通常是相对项目根目录的 YAML 文件。
     - Inventory: 作为一等资源选择，可在 launch 时 prompt。
     - Extra vars: Template/launch/survey 层传入，可被 prompt override；Workflow 也有 extra variables 合并语义。
     - Working dir: 隐含为 controller 同步后的 Project 目录，不要求用户输入任意 host 路径。
     - Dependency assumptions: 依赖 controller 的 execution environment / virtual environment、项目同步、credential store、inventory 资源、collections/roles 支持。

2. **Jenkins Ansible Plugin**
   - Pattern: 在 Jenkins job/pipeline 中提供 `ansiblePlaybook` step，把 `playbook`、`inventory`、`credentialsId`、`extraVars`、`extras` 等参数映射为 `ansible-playbook` CLI 参数。
   - External reference: [Jenkins Ansible plugin](https://plugins.jenkins.io/ansible/) — 文档列出 Ansible installation、inventory CLI arg `-i`、extraVars CLI arg `-e`、extras 作为原样追加参数，并说明通过 Jenkins credential store 支持 SSH private key、用户名密码、vault credential。
   - Common conventions:
     - Playbook path: Jenkins workspace 内路径，如 `my_playbook.yml`。
     - Inventory: workspace 内 inventory 文件或 host list，映射为 `-i`。
     - Extra vars: `extraVars` 映射到 `-e`；`extras` 允许附加原始 CLI flags。
     - Working dir: Jenkins workspace；Ansible executables 可通过 Global Tool Configuration 或 executor 用户 PATH 提供。
     - Dependency assumptions: Jenkins 节点预装或配置 Ansible；凭据来自 Jenkins credential store；若密码认证需要 `sshpass` 在 PATH。

3. **GitHub Action `dawidd6/action-ansible-playbook`**
   - Pattern: GitHub Actions step 输入 `playbook`、`directory`、`configuration`、`inventory`、`key`、`known_hosts`、`vault_password`、`requirements`、`options`，action 生成临时文件并调用 `ansible-playbook`。
   - External reference: [dawidd6/action-ansible-playbook](https://github.com/dawidd6/action-ansible-playbook) — README 示例显示 required `playbook: deploy.yml`，optional `directory: ./`、inline `ansible.cfg` content、inline inventory content、SSH key、known_hosts、vault_password、galaxy requirements filepath、`options` 中传 `--inventory .hosts --limit group1 --extra-vars hello=there --verbose`。
   - Common conventions:
     - Playbook path: `playbook` 输入，通常相对 checkout workspace 或 `directory`。
     - Inventory: 可以是 literal contents 生成临时 `.hosts`，也可通过 `options --inventory` 指向文件。
     - Extra vars: 通过 `options --extra-vars ...`，未抽象为强 schema。
     - Working dir: `directory` 输入指定 playbooks 所在目录。
     - Dependency assumptions: CI runner 可安装/运行 Ansible；可选 `requirements` 安装 Galaxy roles/collections；SSH key/vault password 来自 GitHub Secrets。

4. **Rundeck Ansible Plugin**
   - Pattern: Rundeck plugin 既能从 Ansible inventory 导入 nodes，也能将 playbook 作为 node/workflow step 执行；playbook 可为可访问路径或 inline playbook。
   - External reference: [rundeck-plugins/ansible-plugin README](https://raw.githubusercontent.com/rundeck-plugins/ansible-plugin/master/README.md) — README 说明插件 imports hosts from Ansible inventory，并 can run modules and playbooks；所有操作通过 `ansible` 或 `ansible-playbook` 运行；playbook step 可指定“path to a playbook file (must be accessible to Rundeck)”或 inline playbook；配置按 job、node、project、framework 层级解析。
   - Common conventions:
     - Playbook path: Rundeck 可访问的文件路径，或 inline playbook。
     - Inventory: default configured inventory 用于节点导入和执行。
     - Extra vars/config: 通过 job/node/project/framework attributes 层级解析。
     - Working dir: 由 Rundeck plugin/项目配置控制，核心要求是 playbook 文件对 Rundeck 运行环境可访问。
     - Dependency assumptions: Rundeck 运行节点安装 Ansible CLI；SSH key 可由 Ansible 或 Rundeck 管理；项目/框架配置提供默认值。

#### Baseline Ansible CLI conventions

External reference: [Ansible `ansible-playbook` CLI docs](https://docs.ansible.com/ansible/latest/cli/ansible-playbook.html)

- Playbook 是位置参数：`ansible-playbook ... playbook [playbook ...]`。
- Inventory 用 `-i` / `--inventory` / `--inventory-file` 指定 inventory host path 或逗号分隔 host list；可重复传入。
- Extra vars 用 `-e` / `--extra-vars`，支持 `key=value`、YAML/JSON，文件名前加 `@`；可重复传入。
- 常见运行控制包括 `--limit`、`--tags`、`--skip-tags`、`--syntax-check`、`--check`、`--diff`、`--private-key`、`--vault-password-file`、`--vault-id`、`--forks`、`--timeout`、`--become`、`--user`、verbosity `-v`。
- 环境变量包括 `ANSIBLE_INVENTORY`、`ANSIBLE_CONFIG` 等；默认文件包括 `/etc/ansible/hosts` 与 `/etc/ansible/ansible.cfg`，但可被项目/工作目录内配置覆盖。

#### Mapping to this repo's plugin-first shell ops framework

- A playbook fits the current model as a **plugin tool whose `command` is a plugin-local wrapper script** and whose `args` render declared parameters into `ansible-playbook` flags.
  - `README.md:161-181` shows plugin tools define `command`, `args`, `workdir`, `parameters`, `config_dir`, `config_files`, and `confirm` in `plugin.yaml`.
  - `plugins/plugin.template/plugin.yaml:34-58` shows a config-aware tool calling `scripts/with-config.sh` with `--config config/example.conf`, `workdir: .`, `config_dir: config`, and `config_files: [example.conf]`.
- Plugin-local safety boundary is already enforced and aligns with AWX/GitHub Action “project/workspace” conventions.
  - `internal/plugin/load.go:126-142` validates `command` is required, exists, is not a directory, and `workdir` is safe under the plugin directory.
  - `internal/registry/registry.go:342-385` normalizes plugin tools into `Execution{Type:"shell", Entry:<command>, Args:<args>, Workdir:<workdir>}` and defaults missing `workdir` to `.`.
  - `internal/runner/runner.go:422-424` executes `entry := filepath.Join(tool.Dir, execution.entry)` and `cmd.Dir = resolveWorkdir(tool.Dir, execution.workdir)`, so relative playbook/inventory/config paths in wrapper args resolve naturally from the plugin package.
- Parameter passing is sufficient for common Ansible flags.
  - `internal/runner/runner.go:455-468` renders `Execution.Args` templates, auto-appends `--params-file` when `pass_mode.param_file` is enabled, and invokes `.sh` through `bash` on Windows.
  - `internal/runner/runner.go:534-539` exposes params as `OPS_PARAM_<NAME>` env vars; `plugins/plugin.template/README.md:236-254` documents env vars, command args, and optional `OPS_PARAM_FILE`.
  - This supports declaring explicit parameters such as `inventory`, `limit`, `tags`, `skip_tags`, `extra_vars`, `check`, `diff`, `verbosity`, and forwarding them from wrapper script to `ansible-playbook`.
- `config_files` can expose plugin-owned Ansible assets for Web/API editing but does not execute or pass them automatically.
  - `plugins/plugin.template/README.md:123-131` states `config_files` only declares package-local files and does not grant host absolute access.
  - `plugins/plugin.template/README.md:129-131` and `.trellis/spec/backend/plugin-import-export.md:223-229` require `config_dir` / `config_files` entries to stay relative and prohibit absolute paths or `..` escape.
  - `plugins/plugin.template/README.md:131` and `:401-403` explicitly state `config_files` does not auto pass/copy/generate files; scripts choose whether to read them via args/default path/env.
  - Practical mapping: playbooks (`playbooks/site.yml`) should usually be normal plugin files referenced by wrapper args; editable inventories/vars/ansible.cfg can be under `config/` and declared via `config_dir: config`, `config_files: [inventory.ini, group_vars, ansible.cfg]` if Web editing is desired.
- Host absolute inventories or SSH/vault files are not plugin package defaults.
  - `README.md:191-194` and `.trellis/spec/backend/plugin-import-export.md:230-247` require host absolute config files to be enabled only through `host_config_files.allowed_dirs` plus `configs/plugins/<plugin-id>.mapping.yaml`; plugin packages must not directly grant arbitrary host file access.
  - This differs from Rundeck's “must be accessible to Rundeck” path flexibility and is closer to a packaged Project/workspace boundary.
- Packaging copies plugin content but not Ansible runtime dependencies.
  - `README.md:231-241` and `internal/packagebuild/packagebuild.go:19-29` show `opsctl package build` includes `configs/`, `plugins/`, and the current executable.
  - Therefore Ansible itself, Python interpreter, collections/roles installed outside the plugin, SSH client, `sshpass`, and system credential material are host/runtime dependencies unless packaged inside the plugin as plain files and invoked by wrapper logic.
  - `.trellis/spec/backend/plugin-import-export.md:46-52` and `:247` require plugin export/import to include only plugin-owned regular files; host absolute mapping targets are not exported.

### Common Conventions

| Concern | Comparable systems | Repo-compatible mapping |
|---|---|---|
| Playbook path | AWX Project-relative playbook; Jenkins/GitHub workspace-relative playbook; Rundeck accessible file or inline playbook. | Keep playbooks under plugin directory, e.g. `playbooks/site.yml`; invoke through plugin-local wrapper `scripts/run-playbook.sh`; `workdir: .` or `workdir: playbooks` must remain plugin-local. |
| Inventory | AWX inventory resource; Jenkins `inventory` maps to `-i`; GitHub Action inline inventory or `--inventory`; Rundeck default inventory / node source. | Prefer plugin-local editable inventory under `config/inventory.ini` or `inventories/dev.ini`; pass via `args`/wrapper as `--inventory config/inventory.ini`. Host absolute inventory only through host-side mapping/whitelist if needed for editing, but wrapper still needs an explicit path parameter or convention. |
| Extra vars | CLI `-e`; AWX extra variables/surveys; Jenkins `extraVars`; GitHub `options --extra-vars`. | Model common extra vars as typed `parameters`; wrapper can convert to repeated `--extra-vars` or `--extra-vars @config/vars.yml`. For structured/freeform extra vars, `pass_mode.param_file` can generate `OPS_PARAM_FILE`, but wrapper should decide whether to forward it as `--extra-vars @...`. |
| Working dir | Project/workspace directory is the implicit root; GitHub has explicit `directory`; AWX execution environment checks out project. | Use plugin directory as project root (`workdir: .`); relative ansible.cfg, inventory, roles, collections, and playbook paths then stay package-local and portable. |
| Dependencies | AWX uses execution environments; Jenkins/GitHub/Rundeck assume Ansible CLI on agent/runner or plugin-managed installation; Galaxy requirements are common. | Declare in README/description that target host must have `ansible-playbook`/Python and required collections/roles. If providing `requirements.yml`, keep it plugin-local and have wrapper optionally run/validate `ansible-galaxy install -r requirements.yml` only if that behavior is intended. `opsctl package build` will not install external dependencies. |
| Credentials | AWX/Jenkins/GitHub provide credential stores/secrets; Rundeck can share or isolate SSH keys. | Avoid putting secrets in plugin files. Use existing parameter/env mechanisms with `sensitive: true`, host-side config mapping for allowed host files, or OS/Ansible defaults. If SSH key/vault password files are needed, pass paths explicitly and keep host absolute access under admin control. |
| Dry-run / safety | AWX Job Type Check maps to Ansible check mode; CLI has `--check --diff`; many tools expose confirmation. | Expose `check`/`diff` parameters and set `confirm.required: true` for mutating playbooks; wrapper maps dry-run to `--check`/`--diff`. |

### Related Specs

- `.trellis/spec/backend/plugin-import-export.md` — plugin ZIP/import/export safety contract and host absolute config mapping; especially relevant to playbook/inventory/config files because plugin packages must remain plugin-local and host absolute config targets are never exported.
- `.trellis/spec/backend/directory-structure.md` — general backend directory conventions; no Ansible-specific contract found.
- `.trellis/tasks/05-05-plugin-tool-config-file-input/prd.md` — mentions Ansible vars/extra-vars as an example of config file + CLI override pattern, but not a runtime contract.
- `.trellis/tasks/04-27-external-tool-menu-integration/prd.md` — mentions Ansible modules/collections as comparable namespace/metadata inspiration, but not playbook execution details.

### External References

- [Ansible `ansible-playbook` CLI docs](https://docs.ansible.com/ansible/latest/cli/ansible-playbook.html) — baseline executable interface: positional playbook path, `-i/--inventory`, `-e/--extra-vars`, `--check`, `--diff`, tags, limit, vault, private key, config/env behavior.
- [Automation Controller v4.5 Job Templates](https://legacy-controller-docs.ansible.com/automation-controller/4.5/html/userguide/job_templates.html) — first-class playbook execution unit: Project + Inventory + Playbook + Credentials + Execution Environment + promptable launch parameters.
- [Jenkins Ansible plugin](https://plugins.jenkins.io/ansible/) — CI/plugin style mapping of playbook execution fields to `ansible-playbook` CLI plus credential store integration.
- [dawidd6/action-ansible-playbook](https://github.com/dawidd6/action-ansible-playbook) — action wrapper pattern with `playbook`, `directory`, inline inventory/configuration, SSH/vault inputs, requirements, and raw `options`.
- [Rundeck Ansible plugin README](https://raw.githubusercontent.com/rundeck-plugins/ansible-plugin/master/README.md) — runbook plugin pattern that imports Ansible inventory and runs playbooks as node/workflow steps through `ansible-playbook`.

## Concise Recommendation

Represent each supported Ansible playbook as a normal plugin tool using a plugin-local wrapper script, not as a new unrestricted executable type. Recommended package shape:

```text
plugins/vendor.ansible_app/
  plugin.yaml
  README.md
  scripts/run-playbook.sh
  playbooks/site.yml
  config/
    ansible.cfg
    inventory.ini
    vars.yml
  requirements.yml
```

Recommended manifest pattern:

```yaml
contributes:
  tools:
    - id: vendor.ansible_app.deploy
      name: 执行 Ansible 部署
      category: deploy
      command: scripts/run-playbook.sh
      workdir: .
      args:
        - --playbook
        - playbooks/site.yml
        - --inventory
        - config/inventory.ini
        - --extra-vars-file
        - config/vars.yml
        - --limit
        - "{{ .limit }}"
        - --check
        - "{{ .check }}"
      parameters:
        - name: limit
          type: string
          required: false
          description: Ansible limit pattern
        - name: check
          type: boolean
          required: false
          default: true
          description: 是否使用 ansible-playbook --check
      config_dir: config
      config_files:
        - ansible.cfg
        - inventory.ini
        - vars.yml
      confirm:
        required: true
        message: 确认执行 Ansible 部署？请确认 inventory、limit 和 check/diff 设置。
```

Wrapper responsibilities:

- Validate that playbook/inventory/vars paths are relative and exist under the plugin workdir.
- Convert typed `opsctl` parameters to safe `ansible-playbook` flags (`-i`, `-e @file`, `--limit`, `--tags`, `--check`, `--diff`).
- Set `ANSIBLE_CONFIG=config/ansible.cfg` if package-local config is used.
- Fail clearly if `ansible-playbook` is not on PATH or required Galaxy dependencies are missing.
- Treat credentials/vault/SSH keys as runtime inputs or admin-managed host mappings, not as default plugin package files.

## Caveats / Not Found

- No existing Ansible-specific plugin or code path was found in this repo; search hits for `ansible` are PRD/research references only, not implementation.
- External docs sometimes redirect or 404 for “latest” AWX/Controller pages; cited Controller URL is a versioned v4.5 legacy documentation page that was accessible during research.
- The repo currently has no native schema for `ansible_playbook`, `inventory`, or `extra_vars`; all mapping is through existing shell `command`/`args`/`parameters`/`config_files` mechanics.
- `config_files` declaration alone does not pass files to the runner. Any Ansible config/inventory/vars file must be referenced by wrapper defaults or explicit `args`.
- `opsctl package build` packages plugin files, but not Ansible/Python/system packages; dependency installation/validation must be documented or performed by plugin scripts.
