package llm

import "errors"

var (
	// ErrMissingProvider is returned by [New] when Config.Provider is empty.
	ErrMissingProvider = errors.New("missing provider")
	// ErrUnsupportedProvider is returned by [New] when Config.Provider is not a recognized [Provider].
	ErrUnsupportedProvider = errors.New("unsupported provider")
	// ErrMissingAPIKey is returned by [New] when Config.APIKey is empty.
	ErrMissingAPIKey = errors.New("missing api_key")
	// ErrMissingModel is returned by [New] when Config.Model is empty.
	ErrMissingModel = errors.New("missing model")
	// ErrModelNotFound is returned by [LLM.ModelInfo] when the configured model
	// is not offered by the provider.
	ErrModelNotFound = errors.New("model not found")
	// ErrMissingContextLength is returned by [LLM.ModelInfo] when the provider's
	// response does not report a context length for the configured model.
	ErrMissingContextLength = errors.New("context length not found in model info")
)
