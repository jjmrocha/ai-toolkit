package classify

// Config configures a [Classifier]. Provider, APIKey and Model are required.
type Config struct {
	// Provider selects the classification backend. Required.
	Provider Provider
	// BaseURL overrides the provider's default endpoint. Optional.
	BaseURL string
	// APIKey authenticates requests to the provider. Required.
	APIKey string `json:"-"`
	// Model is the classification model to use. Required.
	Model string
}
