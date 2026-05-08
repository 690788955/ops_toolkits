package config

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

func MergeParams(defs []Parameter, fileParams, overrides map[string]string) map[string]string {
	merged := MergeParamsValues(defs, StringMapToValues(fileParams), StringMapToValues(overrides))
	return ValuesToStringMap(merged)
}

func MergeParamsValues(defs []Parameter, fileParams, overrides map[string]interface{}) Values {
	return MergeValues(MergeParameterDefaults(defs), fileParams, overrides)
}

func PromptMissing(defs []Parameter, params map[string]string, reader io.Reader, writer io.Writer) error {
	scanner := bufio.NewScanner(reader)
	for _, p := range defs {
		if params[p.Name] != "" || !p.Required {
			continue
		}
		if _, err := fmt.Fprintf(writer, "%s: ", parameterPromptLabel(p)); err != nil {
			return err
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return fmt.Errorf("缺少必填参数 %s", p.Name)
		}
		value := strings.TrimSpace(scanner.Text())
		if value == "" {
			return fmt.Errorf("缺少必填参数 %s", p.Name)
		}
		params[p.Name] = value
	}
	return ValidateRequired(defs, params)
}

func ValidateRequired(defs []Parameter, params map[string]string) error {
	return ValidateRequiredValues(defs, StringMapToValues(params))
}

func parameterPromptLabel(param Parameter) string {
	parts := []string{param.Name}
	if param.Description != "" {
		parts = append(parts, param.Description)
	}
	meta := []string{}
	if param.Type != "" {
		meta = append(meta, "类型="+param.Type)
	}
	if param.Required {
		meta = append(meta, "必填")
	} else {
		meta = append(meta, "可选")
	}
	if param.Default != nil {
		meta = append(meta, fmt.Sprintf("默认值=%v", param.Default))
	}
	if len(meta) > 0 {
		parts = append(parts, "("+strings.Join(meta, ", ")+")")
	}
	return strings.Join(parts, " ")
}

func ParseSetValues(values []string) (map[string]string, error) {
	out, err := ParseSetValuesNested(values)
	if err != nil {
		return nil, err
	}
	return ValuesToStringMap(out), nil
}
