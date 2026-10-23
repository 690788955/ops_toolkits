package runbundle

import (
	"shell_ops/internal/config"
	"shell_ops/internal/runner"
)

func recordSensitivePaths(record runner.RunRecord) []string {
	if record.Snapshot == nil || record.Snapshot.Workflow == nil {
		return nil
	}
	return config.SensitivePathsFromParams(record.Snapshot.Workflow.Parameters)
}

// redactedSnapshot 只修改导出副本，保留运行快照供后续恢复使用。
func redactedSnapshot(snapshot *runner.RunSnapshot) *runner.RunSnapshot {
	if snapshot == nil {
		return nil
	}
	clone := *snapshot
	var paths []string
	if snapshot.Workflow != nil {
		paths = config.SensitivePathsFromParams(snapshot.Workflow.Parameters)
	}
	clone.Params = redactFlatParams(snapshot.Params, paths...)
	if snapshot.Workflow == nil {
		return &clone
	}
	wf := *snapshot.Workflow
	wf.Parameters = append([]config.Parameter(nil), wf.Parameters...)
	for index := range wf.Parameters {
		parameter := &wf.Parameters[index]
		if parameter.Default != nil && (parameter.Sensitive || config.IsSensitivePath(parameter.Name)) {
			parameter.Default = "******"
		}
	}
	wf.Stages = append([]config.WorkflowStage(nil), wf.Stages...)
	for index := range wf.Stages {
		wf.Stages[index].ExtraFields = config.RedactSensitive(wf.Stages[index].ExtraFields, nil)
	}
	wf.Edges = append([]config.WorkflowEdge(nil), wf.Edges...)
	for index := range wf.Edges {
		wf.Edges[index].ExtraFields = config.RedactSensitive(wf.Edges[index].ExtraFields, nil)
	}
	wf.Nodes = append([]config.WorkflowNode(nil), wf.Nodes...)
	for index := range wf.Nodes {
		node := &wf.Nodes[index]
		node.Params = config.RedactSensitive(node.Params, nil)
		node.Loop.Params = config.RedactSensitive(node.Loop.Params, nil)
		node.HTTP.Headers = redactedHeaders(node.HTTP.Headers)
		node.Wait.Headers = redactedHeaders(node.Wait.Headers)
		node.Script.Env = redactedHeaders(node.Script.Env)
		node.ExtraFields = config.RedactSensitive(node.ExtraFields, nil)
	}
	if wf.Recovery != nil {
		policy := *wf.Recovery
		if policy.Notification != nil {
			notification := *policy.Notification
			notification.Headers = redactedHeaders(notification.Headers)
			policy.Notification = &notification
		}
		wf.Recovery = &policy
	}
	wf.ExtraFields = config.RedactSensitive(wf.ExtraFields, nil)
	clone.Workflow = &wf
	return &clone
}

func redactedHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	clone := make(map[string]string, len(headers))
	for name := range headers {
		clone[name] = "******"
	}
	return clone
}
