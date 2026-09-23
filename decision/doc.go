// Package decision provides a provider-agnostic client for decision models:
// models that answer typed questions about a state instead of generating text.
// Construct a [Decision] with [New], then call [Decision.Ask] with a [Noul],
// [Choice] or [Score] question and read the typed [Answer] it returns, each
// carrying the probabilities behind it. OpenRouter, which serves TypeSafe's Jev
// models, is the supported provider.
//
// Deciding what to do with an answer is the caller's job: the probabilities and
// the [ChoiceAnswer.Confidence] are reported, never thresholded here. A state
// assembled from untrusted input can steer an answer the way it can steer a
// chat model, so treat a decision as advice from a model, not as an
// authorization.
package decision
