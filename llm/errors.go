package llm

import "errors"

var (
	ErrMissingProvider     = errors.New("missing provider")
	ErrUnsupportedProvider = errors.New("unsupported provider")
	ErrMissingAPIKey       = errors.New("missing api_key")
	ErrMissingModel        = errors.New("missing model")
	ErrModelNotFound       = errors.New("model not found")
)
