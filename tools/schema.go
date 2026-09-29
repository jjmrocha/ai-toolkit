package tools

const (
	keyType        = "type"
	keyDescription = "description"
	keyItems       = "items"
	typeArray      = "array"
)

type field struct {
	name     string
	spec     map[string]any
	required bool
}

// ObjectBuilder builds the JSON Schema of a tool's parameters. Each field method
// returns the builder for chaining, and [ObjectBuilder.Build] returns the map
// for llm.Tool.Schema. Nested objects and arrays of objects take their own
// ObjectBuilder, so schemas nest to any depth.
//
// The zero value is not usable; create one with [NewObjectBuilder].
type ObjectBuilder struct {
	fields []field
}

// NewObjectBuilder returns an empty [ObjectBuilder].
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
			keyType:        "string",
			keyDescription: desc,
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
			keyType:        "integer",
			keyDescription: desc,
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
			keyType:        "number",
			keyDescription: desc,
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
			keyType:        "boolean",
			keyDescription: desc,
		},
		required: required,
	}
	sb.fields = append(sb.fields, f)

	return sb
}

// Object adds an object field named name whose properties, required ones
// included, come from spec.
func (sb *ObjectBuilder) Object(name string, desc string, required bool, spec *ObjectBuilder) *ObjectBuilder {
	s := spec.Build()
	s[keyDescription] = desc

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
		keyType:        typeArray,
		keyDescription: desc,
		keyItems: map[string]any{
			keyType: "string",
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
		keyType:        typeArray,
		keyDescription: desc,
		keyItems: map[string]any{
			keyType: "integer",
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
		keyType:        typeArray,
		keyDescription: desc,
		keyItems: map[string]any{
			keyType: "number",
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
		keyType:        typeArray,
		keyDescription: desc,
		keyItems: map[string]any{
			keyType: "boolean",
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

// ArrayOfObjects adds a field named name that is an array of objects described
// by spec.
func (sb *ObjectBuilder) ArrayOfObjects(name string, desc string, required bool, spec *ObjectBuilder) *ObjectBuilder {
	s := map[string]any{
		keyType:        typeArray,
		keyDescription: desc,
		keyItems:       spec.Build(),
	}
	f := field{
		name:     name,
		spec:     s,
		required: required,
	}
	sb.fields = append(sb.fields, f)

	return sb
}

// Build returns the schema as {"type":"object","properties":{...},
// "required":[...]}, for llm.Tool.Schema. "required" is left out when no field
// is required. Each call returns a new map.
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
		keyType:      "object",
		"properties": fields,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}
