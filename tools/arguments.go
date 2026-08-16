package tools

import "fmt"

// Arguments wraps the decoded arguments of a tool call (the map[string]any a
// [Handler] receives) and provides typed accessors that return each field
// already converted to a Go type, with an error instead of a panic on a type
// mismatch: every accessor reports [ErrFieldNotFound] or [ErrInvalidFieldType].
// Because the values arrive with JSON types, numbers are float64; the numeric
// accessors also accept int so the same code works for programmatically built
// maps.
type Arguments struct {
	data map[string]any
}

// NewArguments wraps data in an [Arguments] for typed field access.
func NewArguments(data map[string]any) *Arguments {
	return &Arguments{data: data}
}

// GetString returns the string field named key. It returns [ErrFieldNotFound]
// if the field is missing and [ErrInvalidFieldType] if it is not a string.
func (a *Arguments) GetString(key string) (string, error) {
	value, ok := a.data[key]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrFieldNotFound, key)
	}

	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%w: %s is not a string", ErrInvalidFieldType, key)
	}

	return str, nil
}

// GetInt returns the field named key as an int. JSON numbers decode as float64,
// so a float64 is accepted and truncated toward zero; an int is returned as is.
// It returns [ErrFieldNotFound] if the field is missing and
// [ErrInvalidFieldType] if it is neither an int nor a float64.
func (a *Arguments) GetInt(key string) (int, error) {
	value, ok := a.data[key]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrFieldNotFound, key)
	}

	switch v := value.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("%w: %s is not an int", ErrInvalidFieldType, key)
	}
}

// GetFloat64 returns the field named key as a float64. An int is accepted and
// converted. It returns [ErrFieldNotFound] if the field is missing and
// [ErrInvalidFieldType] if it is neither a float64 nor an int.
func (a *Arguments) GetFloat64(key string) (float64, error) {
	value, ok := a.data[key]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrFieldNotFound, key)
	}

	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("%w: %s is not a float64", ErrInvalidFieldType, key)
	}
}

// GetBool returns the bool field named key. It returns [ErrFieldNotFound] if
// the field is missing and [ErrInvalidFieldType] if it is not a bool.
func (a *Arguments) GetBool(key string) (bool, error) {
	value, ok := a.data[key]
	if !ok {
		return false, fmt.Errorf("%w: %s", ErrFieldNotFound, key)
	}

	b, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%w: %s is not a bool", ErrInvalidFieldType, key)
	}

	return b, nil
}

// GetObject returns the nested object field named key wrapped in its own
// [Arguments]. It returns [ErrFieldNotFound] if the field is missing and
// [ErrInvalidFieldType] if it is not an object.
func (a *Arguments) GetObject(key string) (*Arguments, error) {
	value, ok := a.data[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrFieldNotFound, key)
	}

	obj, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s is not an object", ErrInvalidFieldType, key)
	}

	return NewArguments(obj), nil
}

// GetArrayOfStrings returns the field named key as a []string. It returns
// [ErrFieldNotFound] if the field is missing and [ErrInvalidFieldType] if it is
// not an array or contains a non-string element.
func (a *Arguments) GetArrayOfStrings(key string) ([]string, error) {
	value, ok := a.data[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrFieldNotFound, key)
	}

	arr, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s is not an array", ErrInvalidFieldType, key)
	}

	result := make([]string, len(arr))
	for i, v := range arr {
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%w: %s contains a non-string value at index %d", ErrInvalidFieldType, key, i)
		}

		result[i] = str
	}

	return result, nil
}

// GetArrayOfInts returns the field named key as a []int, accepting float64
// elements (truncated toward zero) as well as int. It returns
// [ErrFieldNotFound] if the field is missing and [ErrInvalidFieldType] if it is
// not an array or contains an element that is neither.
func (a *Arguments) GetArrayOfInts(key string) ([]int, error) {
	value, ok := a.data[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrFieldNotFound, key)
	}

	arr, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s is not an array", ErrInvalidFieldType, key)
	}

	result := make([]int, len(arr))
	for i, v := range arr {
		switch n := v.(type) {
		case int:
			result[i] = n
		case float64:
			result[i] = int(n)
		default:
			return nil, fmt.Errorf("%w: %s contains a non-int value at index %d", ErrInvalidFieldType, key, i)
		}
	}

	return result, nil
}

// GetArrayOfFloat64s returns the field named key as a []float64, accepting int
// elements (converted) as well as float64. It returns [ErrFieldNotFound] if the
// field is missing and [ErrInvalidFieldType] if it is not an array or contains
// an element that is neither.
func (a *Arguments) GetArrayOfFloat64s(key string) ([]float64, error) {
	value, ok := a.data[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrFieldNotFound, key)
	}

	arr, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s is not an array", ErrInvalidFieldType, key)
	}

	result := make([]float64, len(arr))
	for i, v := range arr {
		switch n := v.(type) {
		case float64:
			result[i] = n
		case int:
			result[i] = float64(n)
		default:
			return nil, fmt.Errorf("%w: %s contains a non-float64 value at index %d", ErrInvalidFieldType, key, i)
		}
	}

	return result, nil
}

// GetArrayOfBools returns the field named key as a []bool. It returns
// [ErrFieldNotFound] if the field is missing and [ErrInvalidFieldType] if it is
// not an array or contains a non-bool element.
func (a *Arguments) GetArrayOfBools(key string) ([]bool, error) {
	value, ok := a.data[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrFieldNotFound, key)
	}

	arr, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s is not an array", ErrInvalidFieldType, key)
	}

	result := make([]bool, len(arr))
	for i, v := range arr {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("%w: %s contains a non-bool value at index %d", ErrInvalidFieldType, key, i)
		}

		result[i] = b
	}

	return result, nil
}

// GetArrayOfObjects returns the field named key as a slice of [Arguments], one
// per object element. It returns [ErrFieldNotFound] if the field is missing and
// [ErrInvalidFieldType] if it is not an array or contains a non-object
// element.
func (a *Arguments) GetArrayOfObjects(key string) ([]*Arguments, error) {
	value, ok := a.data[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrFieldNotFound, key)
	}

	arr, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s is not an array", ErrInvalidFieldType, key)
	}

	result := make([]*Arguments, len(arr))
	for i, v := range arr {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: %s contains a non-object value at index %d", ErrInvalidFieldType, key, i)
		}

		result[i] = NewArguments(obj)
	}

	return result, nil
}
