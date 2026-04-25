package config

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

func MergeParams(defs []Parameter, fileParams, overrides map[string]string) map[string]string {
	merged := map[string]string{}
	for _, p := range defs {
		if p.Default != nil {
			merged[p.Name] = fmt.Sprint(p.Default)
		}
	}
	for k, v := range fileParams {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	return merged
}

func PromptMissing(defs []Parameter, params map[string]string, reader io.Reader, writer io.Writer) error {
	scanner := bufio.NewScanner(reader)
	for _, p := range defs {
		if params[p.Name] != "" || !p.Required {
			continue
		}
		label := p.Description
		if label == "" {
			label = p.Name
		}
		if _, err := fmt.Fprintf(writer, "%s: ", label); err != nil {
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
	for _, p := range defs {
		if p.Required && strings.TrimSpace(params[p.Name]) == "" {
			return fmt.Errorf("缺少必填参数 %s", p.Name)
		}
	}
	return nil
}

func ParseSetValues(values []string) (map[string]string, error) {
	out := map[string]string{}
	for _, item := range values {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("无效的 --set 值 %q，预期格式为 key=value", item)
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}
