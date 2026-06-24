package llm

type Provider string

const (
	ProviderOpenRouter Provider = "openrouter"
)

type Config struct {
	Provider Provider
	BaseURL  string
	APIKey   string `json:"-"`
	Model    string
}
