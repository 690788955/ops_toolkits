package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var safePluginConfigIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type Values map[string]interface{}

func SafePluginConfigID(id string) bool {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" || trimmed != id || trimmed == "." || trimmed == ".." {
		return false
	}
	if strings.ContainsAny(trimmed, "/\\%\"'; \t\r\n") {
		return false
	}
	return safePluginConfigIDPattern.MatchString(trimmed)
}

func StringMapToValues(in map[string]string) Values {
	out := Values{}
	for k, v := range in {
		if strings.Contains(k, ".") {
			_ = SetPathValue(out, k, v)
			continue
		}
		out[k] = v
	}
	return out
}

func InterfaceMapToValues(in map[string]interface{}) Values {
	out := Values{}
	for k, v := range in {
		if strings.Contains(k, ".") {
			_ = SetPathValue(out, k, v)
			continue
		}
		out[k] = normalizeYAMLValue(v)
	}
	return out
}

func CopyValues(in map[string]interface{}) Values {
	out := Values{}
	for k, v := range in {
		out[k] = copyValue(v)
	}
	return out
}

func MergeValues(layers ...map[string]interface{}) Values {
	out := Values{}
	for _, layer := range layers {
		DeepMerge(out, layer)
	}
	return out
}

func DeepMerge(dst map[string]interface{}, src map[string]interface{}) {
	if src == nil {
		return
	}
	for k, v := range src {
		v = normalizeYAMLValue(v)
		if srcMap, ok := asStringMap(v); ok {
			if dstMap, ok := asStringMap(dst[k]); ok {
				merged := CopyValues(dstMap)
				DeepMerge(merged, srcMap)
				dst[k] = merged
				continue
			}
			dst[k] = CopyValues(srcMap)
			continue
		}
		dst[k] = copyValue(v)
	}
}

func SetPathValue(dst map[string]interface{}, dotted string, value interface{}) error {
	parts := strings.Split(dotted, ".")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return fmt.Errorf("无效的点路径 %q", dotted)
		}
	}
	current := dst
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = normalizeYAMLValue(value)
			return nil
		}
		next, ok := asStringMap(current[part])
		if !ok {
			next = Values{}
			current[part] = next
		}
		current = next
	}
	return nil
}

func FlattenValues(in map[string]interface{}) map[string]string {
	out := map[string]string{}
	flattenInto(out, "", in)
	return out
}

func MergeParameterDefaults(defs []Parameter) Values {
	out := Values{}
	for _, p := range defs {
		if p.Default != nil {
			if strings.Contains(p.Name, ".") {
				_ = SetPathValue(out, p.Name, p.Default)
			} else {
				out[p.Name] = normalizeYAMLValue(p.Default)
			}
		}
	}
	return out
}

func ValidateRequiredValues(defs []Parameter, params map[string]interface{}) error {
	flat := FlattenValues(params)
	for _, p := range defs {
		value, ok := valueAtPath(params, p.Name)
		if !ok {
			value, ok = flat[p.Name]
		}
		if p.Required && (!ok || strings.TrimSpace(fmt.Sprint(value)) == "") {
			return fmt.Errorf("缺少必填参数 %s", p.Name)
		}
	}
	return nil
}

func SensitivePathsFromParams(defs []Parameter) []string {
	paths := []string{}
	for _, p := range defs {
		if p.Sensitive {
			paths = append(paths, p.Name)
		}
	}
	return paths
}

func RedactSensitive(in map[string]interface{}, explicitPaths []string) Values {
	explicit := map[string]bool{}
	for _, path := range explicitPaths {
		path = strings.TrimSpace(path)
		if path != "" {
			explicit[path] = true
		}
	}
	out := Values{}
	redactInto(out, "", in, explicit)
	return out
}

func ParseSetValuesNested(values []string) (Values, error) {
	out := Values{}
	for _, item := range values {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("无效的 --set 值 %q，预期格式为 key=value", item)
		}
		if strings.Contains(parts[0], ".") {
			if err := SetPathValue(out, parts[0], parts[1]); err != nil {
				return nil, err
			}
			continue
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}

func ValuesToStringMap(in map[string]interface{}) map[string]string {
	return FlattenValues(in)
}

func DeletePathValue(dst map[string]interface{}, dotted string) {
	if _, ok := dst[dotted]; ok {
		delete(dst, dotted)
		return
	}
	parts := strings.Split(dotted, ".")
	current := dst
	for i, part := range parts {
		value, ok := current[part]
		if !ok {
			return
		}
		if i == len(parts)-1 {
			delete(current, part)
			return
		}
		next, ok := asStringMap(value)
		if !ok {
			return
		}
		current = next
	}
}

func normalizeYAMLValue(v interface{}) interface{} {
	switch typed := v.(type) {
	case map[string]interface{}:
		return CopyValues(typed)
	case map[interface{}]interface{}:
		out := Values{}
		for k, val := range typed {
			out[fmt.Sprint(k)] = normalizeYAMLValue(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, item := range typed {
			out[i] = normalizeYAMLValue(item)
		}
		return out
	default:
		return v
	}
}

func copyValue(v interface{}) interface{} {
	v = normalizeYAMLValue(v)
	if m, ok := asStringMap(v); ok {
		return CopyValues(m)
	}
	if arr, ok := v.([]interface{}); ok {
		out := make([]interface{}, len(arr))
		for i, item := range arr {
			out[i] = copyValue(item)
		}
		return out
	}
	return v
}

func asStringMap(v interface{}) (map[string]interface{}, bool) {
	switch typed := v.(type) {
	case map[string]interface{}:
		return typed, true
	case Values:
		return map[string]interface{}(typed), true
	default:
		return nil, false
	}
}

func flattenInto(out map[string]string, prefix string, in map[string]interface{}) {
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		if m, ok := asStringMap(in[k]); ok {
			flattenInto(out, path, m)
			continue
		}
		out[path] = fmt.Sprint(in[k])
	}
}

func valueAtPath(in map[string]interface{}, dotted string) (interface{}, bool) {
	if v, ok := in[dotted]; ok {
		return v, true
	}
	current := in
	parts := strings.Split(dotted, ".")
	for i, part := range parts {
		value, ok := current[part]
		if !ok {
			return nil, false
		}
		if i == len(parts)-1 {
			return value, true
		}
		next, ok := asStringMap(value)
		if !ok {
			return nil, false
		}
		current = next
	}
	return nil, false
}

func redactInto(out map[string]interface{}, prefix string, in map[string]interface{}, explicit map[string]bool) {
	for k, v := range in {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		if m, ok := asStringMap(v); ok {
			nested := Values{}
			redactInto(nested, path, m, explicit)
			out[k] = nested
			continue
		}
		if isSensitivePath(path, k, explicit) || isSecretReference(v) {
			out[k] = "******"
			continue
		}
		out[k] = copyValue(v)
	}
}

func isSensitivePath(path, key string, explicit map[string]bool) bool {
	if explicit[path] || explicit[key] {
		return true
	}
	lower := strings.ToLower(path)
	for _, marker := range []string{"password", "passwd", "secret", "token", "key"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func isSecretReference(v interface{}) bool {
	text, ok := v.(string)
	if !ok {
		return false
	}
	return strings.HasPrefix(text, "secret:env:") || strings.HasPrefix(text, "secret:file:")
}
