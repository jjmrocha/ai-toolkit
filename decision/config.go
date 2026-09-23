package decision

// Config configures a [Decision]. Provider, APIKey and Model are always
// required; BaseURL defaults to the provider's standard endpoint when empty.
type Config struct {
	// Provider selects the decision backend. Required.
	Provider Provider
	// BaseURL overrides the provider's default API endpoint. Optional.
	BaseURL string
	// APIKey authenticates requests to the provider. Required.
	APIKey string `json:"-"`
	// Model name selects the decision model to use. Required.
	Model string
}
