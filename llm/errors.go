package llm

import "errors"

// Errors returned by the llm package.
var (
	// ErrMissingProvider is returned by [New] when Config.Provider is empty.
	ErrMissingProvider = errors.New("missing provider")
	// ErrUnsupportedProvider is returned by [New] when Config.Provider is not a
	// known [Provider].
	ErrUnsupportedProvider = errors.New("unsupported provider")
	// ErrMissingAPIKey is returned by [New] when Config.APIKey is empty.
	ErrMissingAPIKey = errors.New("missing api_key")
	// ErrMissingModel is returned by [New] when Config.Model is empty.
	ErrMissingModel = errors.New("missing model")
	// ErrInvalidEffort is returned by [New] and [LLM.ChangeEffort] when the effort
	// is not a known [Effort].
	ErrInvalidEffort = errors.New("invalid effort")
	// ErrModelNotFound is returned by [LLM.ModelInfo] when the provider does not
	// offer the model, and by [LLM.ChangeModel] when the model is not in
	// [LLM.AvailableModels].
	ErrModelNotFound = errors.New("model not found")
	// ErrMissingContextLength is returned by [LLM.ModelInfo] when the provider
	// reports no context length for the model.
	ErrMissingContextLength = errors.New("context length not found in model info")
)
