package llm

type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
}
