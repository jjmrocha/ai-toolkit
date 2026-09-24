package classify

import "errors"

// Errors returned by the classify package.
var (
	// ErrMissingProvider is returned by [New] when Config.Provider is empty.
	ErrMissingProvider = errors.New("missing provider")
	// ErrUnsupportedProvider is returned by [New] when Config.Provider is not a recognized [Provider].
	ErrUnsupportedProvider = errors.New("unsupported provider")
	// ErrMissingAPIKey is returned by [New] when Config.APIKey is empty.
	ErrMissingAPIKey = errors.New("missing api_key")
	// ErrMissingModel is returned by [New] when Config.Model is empty.
	ErrMissingModel = errors.New("missing model")
	// ErrNoQuestions is returned by [Classifier.Classify] when
	// Request.Questions is empty.
	ErrNoQuestions = errors.New("no questions")
	// ErrMissingAnswer is returned by [Classifier.Classify] when the provider
	// answers fewer questions than were asked.
	ErrMissingAnswer = errors.New("missing answer")
)
