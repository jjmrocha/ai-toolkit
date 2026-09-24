package classify

// Config configures a [Classifier]. Provider, APIKey and Model are always
// required; BaseURL defaults to the provider's standard endpoint when empty.
type Config struct {
	// Provider selects the classification backend. Required.
	Provider Provider
	// BaseURL overrides the provider's default API endpoint. Optional.
	BaseURL string
	// APIKey authenticates requests to the provider. Required.
	APIKey string `json:"-"`
	// Model name selects the classification model to use. Required.
	Model string
}
