package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// 使用独立类型，防止调用 JSON 编解码时递归进入工作流自身的方法。
type workflowConfigFields WorkflowConfig
type workflowStageFields WorkflowStage
type workflowNodeFields WorkflowNode
type workflowEdgeFields WorkflowEdge

func (wf WorkflowConfig) MarshalJSON() ([]byte, error) {
	return marshalWorkflowFields(workflowConfigFields(wf), wf.ExtraFields)
}

func (wf WorkflowConfig) MarshalYAML() (interface{}, error) {
	value := workflowConfigFields(wf)
	value.ExtraFields = workflowUnknownFields(reflect.TypeOf(value), value.ExtraFields)
	return value, nil
}

func (wf *WorkflowConfig) UnmarshalJSON(data []byte) error {
	value := workflowConfigFields(*wf)
	fields, err := unmarshalWorkflowFields(data, &value)
	if err != nil {
		return err
	}
	value.ExtraFields = fields
	*wf = WorkflowConfig(value)
	return nil
}

func (stage WorkflowStage) MarshalJSON() ([]byte, error) {
	return marshalWorkflowFields(workflowStageFields(stage), stage.ExtraFields)
}

func (stage WorkflowStage) MarshalYAML() (interface{}, error) {
	value := workflowStageFields(stage)
	value.ExtraFields = workflowUnknownFields(reflect.TypeOf(value), value.ExtraFields)
	return value, nil
}

func (stage *WorkflowStage) UnmarshalJSON(data []byte) error {
	value := workflowStageFields(*stage)
	fields, err := unmarshalWorkflowFields(data, &value)
	if err != nil {
		return err
	}
	value.ExtraFields = fields
	*stage = WorkflowStage(value)
	return nil
}

func (node WorkflowNode) MarshalJSON() ([]byte, error) {
	return marshalWorkflowFields(workflowNodeFields(node), node.ExtraFields)
}

func (node WorkflowNode) MarshalYAML() (interface{}, error) {
	value := workflowNodeFields(node)
	value.ExtraFields = workflowUnknownFields(reflect.TypeOf(value), value.ExtraFields)
	return value, nil
}

func (node *WorkflowNode) UnmarshalJSON(data []byte) error {
	value := workflowNodeFields(*node)
	fields, err := unmarshalWorkflowFields(data, &value)
	if err != nil {
		return err
	}
	value.ExtraFields = fields
	*node = WorkflowNode(value)
	return nil
}

func (edge WorkflowEdge) MarshalJSON() ([]byte, error) {
	return marshalWorkflowFields(workflowEdgeFields(edge), edge.ExtraFields)
}

func (edge WorkflowEdge) MarshalYAML() (interface{}, error) {
	value := workflowEdgeFields(edge)
	value.ExtraFields = workflowUnknownFields(reflect.TypeOf(value), value.ExtraFields)
	return value, nil
}

func (edge *WorkflowEdge) UnmarshalJSON(data []byte) error {
	value := workflowEdgeFields(*edge)
	fields, err := unmarshalWorkflowFields(data, &value)
	if err != nil {
		return err
	}
	value.ExtraFields = fields
	*edge = WorkflowEdge(value)
	return nil
}

func marshalWorkflowFields(value interface{}, fields map[string]interface{}) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil || len(fields) == 0 {
		return data, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	for key, field := range workflowUnknownFields(reflect.TypeOf(value), fields) {
		encoded, err := json.Marshal(normalizeYAMLValue(field))
		if err != nil {
			return nil, err
		}
		object[key] = encoded
	}
	return json.Marshal(object)
}

func unmarshalWorkflowFields(data []byte, target interface{}) (map[string]interface{}, error) {
	if err := json.Unmarshal(data, target); err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	known := workflowFieldNames(reflect.TypeOf(target).Elem())
	var fields map[string]interface{}
	for key, field := range object {
		if workflowKnownField(known, key) {
			continue
		}
		var value interface{}
		decoder := json.NewDecoder(bytes.NewReader(field))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if fields == nil {
			fields = map[string]interface{}{}
		}
		fields[key] = workflowFieldNumbers(value)
	}
	return fields, nil
}

func workflowFieldNames(value reflect.Type) map[string]bool {
	known := map[string]bool{}
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		for _, format := range []string{"json", "yaml"} {
			name := strings.Split(field.Tag.Get(format), ",")[0]
			if name != "" && name != "-" {
				known[name] = true
			}
		}
	}
	return known
}

// 与 encoding/json 使用相同的 Unicode 大小写折叠，避免扩展字段在 JSON 克隆时覆盖执行配置。
func workflowKnownField(known map[string]bool, name string) bool {
	for field := range known {
		if strings.EqualFold(name, field) {
			return true
		}
	}
	return false
}

func workflowUnknownFields(value reflect.Type, fields map[string]interface{}) map[string]interface{} {
	if len(fields) == 0 {
		return nil
	}
	known := workflowFieldNames(value)
	unknown := map[string]interface{}{}
	for key, field := range fields {
		if !workflowKnownField(known, key) {
			unknown[key] = field
		}
	}
	return unknown
}

// JSON 的数字先保留原文，输出 YAML 时仍使用数值标量，避免整数经 float64 丢失精度。
func workflowFieldNumbers(value interface{}) interface{} {
	switch value := value.(type) {
	case json.Number:
		return workflowFieldNumber(value.String())
	case map[string]interface{}:
		for key, field := range value {
			value[key] = workflowFieldNumbers(field)
		}
	case []interface{}:
		for index, field := range value {
			value[index] = workflowFieldNumbers(field)
		}
	}
	return value
}

type workflowFieldNumber string

func (number workflowFieldNumber) MarshalJSON() ([]byte, error) {
	return json.Marshal(json.Number(number))
}

func (number workflowFieldNumber) MarshalYAML() (interface{}, error) {
	tag := "!!int"
	if strings.ContainsAny(string(number), ".eE") {
		tag = "!!float"
	}
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: string(number)}
	// 保存前按 yaml.v3 的标量解析规则校验，拒绝写出只能编码却无法重载的数值。
	var decoded interface{}
	if err := node.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("扩展字段数值 %s 超出 YAML 支持范围: %w", number, err)
	}
	return node, nil
}
