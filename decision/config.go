package decision

// Config configures a [Decision]. Provider, APIKey and Model are required.
type Config struct {
	// Provider selects the decision backend. Required.
	Provider Provider
	// BaseURL overrides the provider's default endpoint. Optional.
	BaseURL string
	// APIKey authenticates requests to the provider. Required.
	APIKey string `json:"-"`
	// Model is the decision model to use. Required.
	Model string
}
