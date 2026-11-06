package decision

// Config configures a [Decision]. Provider and Model are required, and APIKey
// is required for [ProviderOpenRouter].
type Config struct {
	// Provider selects the decision backend. Required.
	Provider Provider
	// BaseURL overrides the provider's default endpoint. Optional.
	BaseURL string
	// APIKey authenticates requests to the provider. Required for
	// [ProviderOpenRouter]; [ProviderOllama] ignores it.
	APIKey string `json:"-"`
	// Model is the decision model to use. Required.
	Model string
}
