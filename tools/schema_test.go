package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestObjectBuilder(t *testing.T) {
	fieldTypes := []struct {
		name     string
		build    func(*ObjectBuilder)
		field    string
		expected map[string]any
	}{
		{
			name:     "string",
			build:    func(b *ObjectBuilder) { b.String("f", "desc", true) },
			field:    "f",
			expected: map[string]any{"type": "string", "description": "desc"},
		},
		{
			name:     "integer",
			build:    func(b *ObjectBuilder) { b.Integer("f", "desc", true) },
			field:    "f",
			expected: map[string]any{"type": "integer", "description": "desc"},
		},
		{
			name:     "number",
			build:    func(b *ObjectBuilder) { b.Number("f", "desc", true) },
			field:    "f",
			expected: map[string]any{"type": "number", "description": "desc"},
		},
		{
			name:     "boolean",
			build:    func(b *ObjectBuilder) { b.Boolean("f", "desc", true) },
			field:    "f",
			expected: map[string]any{"type": "boolean", "description": "desc"},
		},
		{
			name:  "array of strings",
			build: func(b *ObjectBuilder) { b.ArrayOfStrings("f", "desc", true) },
			field: "f",
			expected: map[string]any{
				"type":        "array",
				"description": "desc",
				"items":       map[string]any{"type": "string"},
			},
		},
		{
			name:  "array of integers",
			build: func(b *ObjectBuilder) { b.ArrayOfIntegers("f", "desc", true) },
			field: "f",
			expected: map[string]any{
				"type":        "array",
				"description": "desc",
				"items":       map[string]any{"type": "integer"},
			},
		},
		{
			name:  "array of numbers",
			build: func(b *ObjectBuilder) { b.ArrayOfNumbers("f", "desc", true) },
			field: "f",
			expected: map[string]any{
				"type":        "array",
				"description": "desc",
				"items":       map[string]any{"type": "number"},
			},
		},
		{
			name:  "array of booleans",
			build: func(b *ObjectBuilder) { b.ArrayOfBooleans("f", "desc", true) },
			field: "f",
			expected: map[string]any{
				"type":        "array",
				"description": "desc",
				"items":       map[string]any{"type": "boolean"},
			},
		},
	}

	for _, tc := range fieldTypes {
		t.Run(tc.name, func(t *testing.T) {
			// given
			b := NewObjectBuilder()
			tc.build(b)
			// when
			result := b.Build()
			// then
			properties := result["properties"].(map[string]any)
			assert.Equal(t, tc.expected, properties[tc.field])
		})
	}

	t.Run("empty builder omits the required key", func(t *testing.T) {
		// given
		b := NewObjectBuilder()
		// when
		result := b.Build()
		// then
		assert.Equal(t, "object", result["type"])
		assert.Empty(t, result["properties"])
		assert.NotContains(t, result, "required")
	})

	t.Run("lists required fields in declaration order", func(t *testing.T) {
		// given
		b := NewObjectBuilder().
			String("a", "", true).
			String("b", "", false).
			String("c", "", true)
		// when
		result := b.Build()
		// then
		expected := []string{"a", "c"}
		assert.Equal(t, expected, result["required"])
	})

	t.Run("omits the required key when no field is required", func(t *testing.T) {
		// given
		b := NewObjectBuilder().String("a", "", false)
		// when
		result := b.Build()
		// then
		assert.NotContains(t, result, "required")
	})

	t.Run("nests an object with its own properties and required", func(t *testing.T) {
		// given
		address := NewObjectBuilder().String("street", "street name", true)
		b := NewObjectBuilder().Object("address", "mailing address", true, address)
		// when
		result := b.Build()
		// then
		expected := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"address": map[string]any{
					"type":        "object",
					"description": "mailing address",
					"properties": map[string]any{
						"street": map[string]any{"type": "string", "description": "street name"},
					},
					"required": []string{"street"},
				},
			},
			"required": []string{"address"},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("nests the element schema for an array of objects", func(t *testing.T) {
		// given
		item := NewObjectBuilder().String("name", "the name", true)
		b := NewObjectBuilder().ArrayOfObjects("contacts", "contact list", false, item)
		// when
		result := b.Build()
		// then
		properties := result["properties"].(map[string]any)
		expected := map[string]any{
			"type":        "array",
			"description": "contact list",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "the name"},
				},
				"required": []string{"name"},
			},
		}
		assert.Equal(t, expected, properties["contacts"])
	})

	t.Run("returns an independent map on each call", func(t *testing.T) {
		// given
		b := NewObjectBuilder().String("a", "", true)
		first := b.Build()
		first["injected"] = true
		// when
		result := b.Build()
		// then
		assert.NotContains(t, result, "injected")
	})
}
