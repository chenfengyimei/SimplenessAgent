package worker

import (
	"fmt"
	"reflect"
)

// validateSchema supports the bounded JSON Schema subset used by first-party
// tools: object properties/required/additionalProperties, array items,
// primitive type and enum. Unsupported keywords fail closed so a tool schema
// can never appear more restrictive than the Worker actually enforces.
func validateSchema(value interface{}, schema map[string]interface{}, path string) error {
	typeName, ok := schema["type"].(string)
	if !ok || typeName == "" {
		return fmt.Errorf("schema at %s requires a type", path)
	}
	if err := knownSchemaKeywords(schema, typeName); err != nil {
		return err
	}
	if rawEnum, exists := schema["enum"]; exists {
		enum, ok := rawEnum.([]interface{})
		if !ok || len(enum) == 0 {
			return fmt.Errorf("schema enum at %s is invalid", path)
		}
		matched := false
		for _, candidate := range enum {
			if reflect.DeepEqual(value, candidate) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s is not an allowed value", path)
		}
	}
	switch typeName {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be a string", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
	case "number":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("%s must be a number", path)
		}
	case "integer":
		number, ok := value.(float64)
		if !ok || number != float64(int64(number)) {
			return fmt.Errorf("%s must be an integer", path)
		}
	case "array":
		items, ok := value.([]interface{})
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		if itemSchema, ok := schema["items"].(map[string]interface{}); ok {
			for index, item := range items {
				if err := validateSchema(item, itemSchema, fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		}
	case "object":
		object, ok := value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		properties := map[string]interface{}{}
		if rawProperties, exists := schema["properties"]; exists {
			var valid bool
			properties, valid = rawProperties.(map[string]interface{})
			if !valid {
				return fmt.Errorf("schema properties at %s are invalid", path)
			}
		}
		required, err := schemaStrings(schema, "required")
		if err != nil {
			return err
		}
		for _, rawRequired := range required {
			if _, exists := object[rawRequired]; !exists {
				return fmt.Errorf("%s.%s is required", path, rawRequired)
			}
		}
		allowAdditional := false
		explicitlySet := false
		if rawAdditional, exists := schema["additionalProperties"]; exists {
			var valid bool
			allowAdditional, valid = rawAdditional.(bool)
			if !valid {
				return fmt.Errorf("schema additionalProperties at %s is invalid", path)
			}
			explicitlySet = true
		}
		for key, item := range object {
			rawProperty, found := properties[key]
			if !found {
				if explicitlySet && !allowAdditional {
					return fmt.Errorf("%s.%s is not allowed", path, key)
				}
				continue
			}
			property, ok := rawProperty.(map[string]interface{})
			if !ok {
				return fmt.Errorf("schema for %s.%s is invalid", path, key)
			}
			if err := validateSchema(item, property, path+"."+key); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("schema type %q is unsupported", typeName)
	}
	return nil
}

func knownSchemaKeywords(schema map[string]interface{}, typeName string) error {
	allowed := map[string]bool{"type": true, "enum": true, "description": true}
	switch typeName {
	case "object":
		allowed["properties"] = true
		allowed["required"] = true
		allowed["additionalProperties"] = true
	case "array":
		allowed["items"] = true
	case "string", "boolean", "number", "integer":
	default:
		return fmt.Errorf("schema type %q is unsupported", typeName)
	}
	for key := range schema {
		if !allowed[key] {
			return fmt.Errorf("schema keyword %q is unsupported", key)
		}
	}
	return nil
}

func schemaStrings(schema map[string]interface{}, key string) ([]string, error) {
	raw, exists := schema[key]
	if !exists {
		return nil, nil
	}
	if values, ok := raw.([]string); ok {
		return values, nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("schema %s is invalid", key)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("schema %s is invalid", key)
		}
		result = append(result, text)
	}
	return result, nil
}
