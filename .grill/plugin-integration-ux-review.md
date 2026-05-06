# Grill: 插件接入体验优化审查
Date: 2026-05-07

## Intent
用户希望框架重点优化“内部运维同事自己写工具后，能通过模板按规范建目录、编写 plugin.yaml 并顺利接入”的体验。框架不应约束插件脚本内部业务编码逻辑，而应提供清晰的插件接入协议、校验器和交付壳子。

## Constraints
- 插件作者主要是内部运维同事：会写 Shell，熟业务系统，不一定理解 Go 或框架内部。
- 不做强约束脚本开发框架；只管插件能被发现、展示、传参、执行和报错。
- 第一阶段不做 `opsctl plugin init` 脚手架，继续依赖模板/开发包 + validate 反馈。
- Warning 不阻断接入，不影响运行，不受 `plugins.strict` 影响。

## Key decisions
- Decision: 第一优先级放在插件接入质量 warning 机制。 Reason: 当前框架核心价值是让运维同事低摩擦接入已有工具；最近配置机制反复收敛也说明插件接入心智模型是主要风险。 Alternative considered: 优先优化 Web 体验、运行稳定性、架构拆分、交付部署能力。
- Decision: 框架只管接入契约，不管脚本内部质量。 Reason: 用户明确不希望规范插件作者的编码逻辑，只要求顺利接入。 Alternative considered: 强制脚本参数解析、日志格式、错误码等规范。
- Decision: README 缺失作为 Warning，不作为 Error。 Reason: README 是推荐交付标准，但不应阻断插件接入。 Alternative considered: README 必须存在才允许 validate 通过。
- Decision: validate 输出 Error / Warning 两级。 Reason: Error 表示无法安全或正确接入；Warning 表示能接入但交付质量可改进。 Alternative considered: 只有成功/失败，不做 warning。
- Decision: 第一版 Warning 覆盖 README 缺失、插件/工具 description 缺失、参数 description 缺失、confirm message 为空或太短、config_files 声明文件不存在。 Reason: 这些问题不阻断执行，但直接影响插件作者自助接入和页面可理解性。 Alternative considered: 做更细的脚本质量检查。
- Decision: Warning 暴露在 `opsctl validate` 和插件上传结果中，不在工具运行时提示。 Reason: Warning 是接入质量问题，不是执行时问题。 Alternative considered: Web 各处常驻显示 warning。
- Decision: Warning 应挂到 registry/catalog 数据流中复用。 Reason: 避免 CLI、上传 API、Web 各自重复实现检查规则导致不一致。 Alternative considered: 只在 validate 命令中临时检查。
- Decision: `plugins.strict` 只影响 Error，不影响 Warning。 Reason: strict 当前语义是加载失败是否阻断；把 README/description 类问题纳入 strict 会混淆语义。 Alternative considered: strict 下 warning 也失败。

## Surfaced assumptions
- “插件合格”不等于“脚本写得规范”，而是插件包满足框架接入契约。
- 配置文件、参数、全局环境的边界是插件作者最容易困惑的地方，应通过模板和 validate 信息持续强化。
- 对内部运维同事而言，明确的错误/警告文案比新增复杂 SDK 或脚手架更有价值。

## Out of scope
- 第一阶段不做 CLI 或 Web 脚手架生成器。
- 不强制检查脚本内部参数解析、日志格式、dry-run 实现或回滚逻辑。
- 不让 warning 影响 `run tool` / `run workflow`。
