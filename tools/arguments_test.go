package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetString(t *testing.T) {
	t.Run("returns the string value", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"city": "Lisbon"})
		// when
		result, err := args.GetString("city")
		// then
		require.NoError(t, err)
		assert.Equal(t, "Lisbon", result)
	})

	t.Run("errors when the field is missing", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{})
		// when
		_, err := args.GetString("city")
		// then
		assert.Error(t, err)
	})

	t.Run("errors when the field is not a string", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"city": 42})
		// when
		_, err := args.GetString("city")
		// then
		assert.Error(t, err)
	})
}

func TestGetInt(t *testing.T) {
	t.Run("returns an int value", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"n": 7})
		// when
		result, err := args.GetInt("n")
		// then
		require.NoError(t, err)
		assert.Equal(t, 7, result)
	})

	t.Run("accepts a float64 and truncates toward zero", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"pos": 3.9, "neg": -3.9})
		// when
		pos, errPos := args.GetInt("pos")
		neg, errNeg := args.GetInt("neg")
		// then
		require.NoError(t, errPos)
		require.NoError(t, errNeg)
		assert.Equal(t, 3, pos)
		assert.Equal(t, -3, neg)
	})

	t.Run("errors when the field is missing", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{})
		// when
		_, err := args.GetInt("n")
		// then
		assert.Error(t, err)
	})

	t.Run("errors when the field is neither int nor float64", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"n": "7"})
		// when
		_, err := args.GetInt("n")
		// then
		assert.Error(t, err)
	})
}

func TestGetFloat64(t *testing.T) {
	t.Run("returns a float64 value", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"x": 2.5})
		// when
		result, err := args.GetFloat64("x")
		// then
		require.NoError(t, err)
		assert.Equal(t, 2.5, result)
	})

	t.Run("accepts an int and converts it", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"x": 4})
		// when
		result, err := args.GetFloat64("x")
		// then
		require.NoError(t, err)
		assert.Equal(t, 4.0, result)
	})

	t.Run("errors when the field is missing", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{})
		// when
		_, err := args.GetFloat64("x")
		// then
		assert.Error(t, err)
	})

	t.Run("errors when the field is neither float64 nor int", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"x": "2.5"})
		// when
		_, err := args.GetFloat64("x")
		// then
		assert.Error(t, err)
	})
}

func TestGetBool(t *testing.T) {
	t.Run("returns the bool value", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"ok": true})
		// when
		result, err := args.GetBool("ok")
		// then
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("errors when the field is missing", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{})
		// when
		_, err := args.GetBool("ok")
		// then
		assert.Error(t, err)
	})

	t.Run("errors when the field is not a bool", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"ok": "true"})
		// when
		_, err := args.GetBool("ok")
		// then
		assert.Error(t, err)
	})
}

func TestGetObject(t *testing.T) {
	t.Run("returns a nested Arguments that reads inner fields", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{
			"address": map[string]any{"city": "Porto"},
		})
		// when
		obj, err := args.GetObject("address")
		// then
		require.NoError(t, err)
		city, err := obj.GetString("city")
		require.NoError(t, err)
		assert.Equal(t, "Porto", city)
	})

	t.Run("errors when the field is missing", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{})
		// when
		_, err := args.GetObject("address")
		// then
		assert.Error(t, err)
	})

	t.Run("errors when the field is not an object", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"address": "Porto"})
		// when
		_, err := args.GetObject("address")
		// then
		assert.Error(t, err)
	})
}

func TestGetArrayOfStrings(t *testing.T) {
	t.Run("returns the string slice", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"tags": []any{"a", "b"}})
		// when
		result, err := args.GetArrayOfStrings("tags")
		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, result)
	})

	t.Run("returns an empty slice for an empty array", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"tags": []any{}})
		// when
		result, err := args.GetArrayOfStrings("tags")
		// then
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("errors when the field is missing", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{})
		// when
		_, err := args.GetArrayOfStrings("tags")
		// then
		assert.Error(t, err)
	})

	t.Run("errors when the field is not an array", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"tags": "a"})
		// when
		_, err := args.GetArrayOfStrings("tags")
		// then
		assert.Error(t, err)
	})

	t.Run("errors on a non-string element", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"tags": []any{"a", 1}})
		// when
		_, err := args.GetArrayOfStrings("tags")
		// then
		assert.Error(t, err)
	})
}

func TestGetArrayOfInts(t *testing.T) {
	t.Run("returns the int slice", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"ns": []any{1, 2, 3}})
		// when
		result, err := args.GetArrayOfInts("ns")
		// then
		require.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, result)
	})

	t.Run("accepts float64 elements and truncates them", func(t *testing.T) {
		// given JSON numbers decode as float64
		args := NewArguments(map[string]any{"ns": []any{1.0, 2.9, 3.2}})
		// when
		result, err := args.GetArrayOfInts("ns")
		// then
		require.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, result)
	})

	t.Run("errors when the field is missing", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{})
		// when
		_, err := args.GetArrayOfInts("ns")
		// then
		assert.Error(t, err)
	})

	t.Run("errors when the field is not an array", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"ns": 1})
		// when
		_, err := args.GetArrayOfInts("ns")
		// then
		assert.Error(t, err)
	})

	t.Run("errors on a non-int element", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"ns": []any{1, "2"}})
		// when
		_, err := args.GetArrayOfInts("ns")
		// then
		assert.Error(t, err)
	})
}

func TestGetArrayOfFloat64s(t *testing.T) {
	t.Run("returns the float64 slice", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"xs": []any{1.5, 2.5}})
		// when
		result, err := args.GetArrayOfFloat64s("xs")
		// then
		require.NoError(t, err)
		assert.Equal(t, []float64{1.5, 2.5}, result)
	})

	t.Run("accepts int elements by index, not by value", func(t *testing.T) {
		// given int values that would index out of bounds if used as the slice
		// index (regression guard for the shadowed loop variable)
		args := NewArguments(map[string]any{"xs": []any{5, 6, 7}})
		// when
		result, err := args.GetArrayOfFloat64s("xs")
		// then
		require.NoError(t, err)
		assert.Equal(t, []float64{5, 6, 7}, result)
	})

	t.Run("errors when the field is missing", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{})
		// when
		_, err := args.GetArrayOfFloat64s("xs")
		// then
		assert.Error(t, err)
	})

	t.Run("errors when the field is not an array", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"xs": 1.5})
		// when
		_, err := args.GetArrayOfFloat64s("xs")
		// then
		assert.Error(t, err)
	})

	t.Run("errors on a non-float64 element", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"xs": []any{1.5, "2.5"}})
		// when
		_, err := args.GetArrayOfFloat64s("xs")
		// then
		assert.Error(t, err)
	})
}

func TestGetArrayOfBools(t *testing.T) {
	t.Run("returns the bool slice", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"flags": []any{true, false}})
		// when
		result, err := args.GetArrayOfBools("flags")
		// then
		require.NoError(t, err)
		assert.Equal(t, []bool{true, false}, result)
	})

	t.Run("errors when the field is missing", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{})
		// when
		_, err := args.GetArrayOfBools("flags")
		// then
		assert.Error(t, err)
	})

	t.Run("errors when the field is not an array", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"flags": true})
		// when
		_, err := args.GetArrayOfBools("flags")
		// then
		assert.Error(t, err)
	})

	t.Run("errors on a non-bool element", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"flags": []any{true, "false"}})
		// when
		_, err := args.GetArrayOfBools("flags")
		// then
		assert.Error(t, err)
	})
}

func TestGetArrayOfObjects(t *testing.T) {
	t.Run("returns nested Arguments for each element", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{
			"people": []any{
				map[string]any{"name": "Ana"},
				map[string]any{"name": "Rui"},
			},
		})
		// when
		result, err := args.GetArrayOfObjects("people")
		// then
		require.NoError(t, err)
		require.Len(t, result, 2)
		first, err := result[0].GetString("name")
		require.NoError(t, err)
		second, err := result[1].GetString("name")
		require.NoError(t, err)
		assert.Equal(t, "Ana", first)
		assert.Equal(t, "Rui", second)
	})

	t.Run("errors when the field is missing", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{})
		// when
		_, err := args.GetArrayOfObjects("people")
		// then
		assert.Error(t, err)
	})

	t.Run("errors when the field is not an array", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"people": "Ana"})
		// when
		_, err := args.GetArrayOfObjects("people")
		// then
		assert.Error(t, err)
	})

	t.Run("errors on a non-object element", func(t *testing.T) {
		// given
		args := NewArguments(map[string]any{"people": []any{"Ana"}})
		// when
		_, err := args.GetArrayOfObjects("people")
		// then
		assert.Error(t, err)
	})
}
