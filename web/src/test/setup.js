// Vitest 全局初始化。
// React 18+ 在没有设置 IS_REACT_ACT_ENVIRONMENT 时会对每个 act() 调用
// 打印 "The current testing environment is not configured to support act(...)"，
// 大量噪声会掩盖真正需要关注的状态更新告警。
globalThis.IS_REACT_ACT_ENVIRONMENT = true
