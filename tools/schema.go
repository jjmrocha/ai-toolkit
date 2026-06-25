package tools

type field struct {
	name     string
	spec     map[string]any
	required bool
}

// ObjectBuilder builds the JSON Schema for an object-typed set of tool
// parameters. Add fields with the Add* methods (each returns the builder for
// chaining) and call Build to produce the schema map for llm.Tool.Schema.
// Nested objects and arrays of objects are described with their own
// ObjectBuilder, so schemas of any depth compose without hand-written maps.
//
// The zero value is not usable; create one with NewObjectBuilder.
type ObjectBuilder struct {
	fields []field
}

// NewObjectBuilder returns an empty ObjectBuilder.
func NewObjectBuilder() *ObjectBuilder {
	return &ObjectBuilder{
		fields: make([]field, 0),
	}
}

// String adds a string field named name. desc documents the field for the
// model; required marks it as a required property.
func (sb *ObjectBuilder) String(name string, desc string, required bool) *ObjectBuilder {
	f := field{
		name: name,
		spec: map[string]any{
			"type":        "string",
			"description": desc,
		},
		required: required,
	}
	sb.fields = append(sb.fields, f)

	return sb
}

// Integer adds an integer field named name (JSON Schema type "integer").
func (sb *ObjectBuilder) Integer(name string, desc string, required bool) *ObjectBuilder {
	f := field{
		name: name,
		spec: map[string]any{
			"type":        "integer",
			"description": desc,
		},
		required: required,
	}
	sb.fields = append(sb.fields, f)

	return sb
}

// Number adds a floating-point field named name (JSON Schema type "number").
func (sb *ObjectBuilder) Number(name string, desc string, required bool) *ObjectBuilder {
	f := field{
		name: name,
		spec: map[string]any{
			"type":        "number",
			"description": desc,
		},
		required: required,
	}
	sb.fields = append(sb.fields, f)

	return sb
}

// Boolean adds a boolean field named name.
func (sb *ObjectBuilder) Boolean(name string, desc string, required bool) *ObjectBuilder {
	f := field{
		name: name,
		spec: map[string]any{
			"type":        "boolean",
			"description": desc,
		},
		required: required,
	}
	sb.fields = append(sb.fields, f)

	return sb
}

// Object adds a nested object field named name, whose properties are
// described by spec. The nested object's own required fields are preserved.
func (sb *ObjectBuilder) Object(name string, desc string, required bool, spec *ObjectBuilder) *ObjectBuilder {
	s := spec.Build()
	s["description"] = desc

	f := field{
		name:     name,
		spec:     s,
		required: required,
	}
	sb.fields = append(sb.fields, f)

	return sb
}

// ArrayOfStrings adds a field named name that is an array of strings.
func (sb *ObjectBuilder) ArrayOfStrings(name string, desc string, required bool) *ObjectBuilder {
	s := map[string]any{
		"type":        "array",
		"description": desc,
		"items": map[string]any{
			"type": "string",
		},
	}
	f := field{
		name:     name,
		spec:     s,
		required: required,
	}
	sb.fields = append(sb.fields, f)

	return sb
}

// ArrayOfIntegers adds a field named name that is an array of integers.
func (sb *ObjectBuilder) ArrayOfIntegers(name string, desc string, required bool) *ObjectBuilder {
	s := map[string]any{
		"type":        "array",
		"description": desc,
		"items": map[string]any{
			"type": "integer",
		},
	}
	f := field{
		name:     name,
		spec:     s,
		required: required,
	}
	sb.fields = append(sb.fields, f)

	return sb
}

// ArrayOfNumbers adds a field named name that is an array of numbers.
func (sb *ObjectBuilder) ArrayOfNumbers(name string, desc string, required bool) *ObjectBuilder {
	s := map[string]any{
		"type":        "array",
		"description": desc,
		"items": map[string]any{
			"type": "number",
		},
	}
	f := field{
		name:     name,
		spec:     s,
		required: required,
	}
	sb.fields = append(sb.fields, f)

	return sb
}

// ArrayOfBooleans adds a field named name that is an array of booleans.
func (sb *ObjectBuilder) ArrayOfBooleans(name string, desc string, required bool) *ObjectBuilder {
	s := map[string]any{
		"type":        "array",
		"description": desc,
		"items": map[string]any{
			"type": "boolean",
		},
	}
	f := field{
		name:     name,
		spec:     s,
		required: required,
	}
	sb.fields = append(sb.fields, f)

	return sb
}

// ArrayOfObjects adds a field named name that is an array whose elements are
// objects described by spec.
func (sb *ObjectBuilder) ArrayOfObjects(name string, desc string, required bool, spec *ObjectBuilder) *ObjectBuilder {
	s := map[string]any{
		"type":        "array",
		"description": desc,
		"items":       spec.Build(),
	}
	f := field{
		name:     name,
		spec:     s,
		required: required,
	}
	sb.fields = append(sb.fields, f)

	return sb
}

// Build assembles the accumulated fields into a JSON Schema object of the form
// {"type":"object","properties":{...},"required":[...]}, suitable for
// llm.Tool.Schema. The "required" key is omitted when no field is required.
// Build can be called more than once and returns a new map each time.
func (sb *ObjectBuilder) Build() map[string]any {
	fields := make(map[string]any)
	required := make([]string, 0)

	for _, f := range sb.fields {
		fields[f.name] = f.spec

		if f.required {
			required = append(required, f.name)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": fields,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}
