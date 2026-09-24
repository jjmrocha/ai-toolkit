// Package classify provides a provider-agnostic client for classification
// models: models that answer typed questions about an input with probabilities
// instead of generating text. Construct a [Classifier] with [New], then call
// [Classifier.Classify] with a [YesNo], [Choice] or [Score] question and read the
// typed [Answer] it returns, each carrying the probabilities behind it.
// OpenRouter, which serves TypeSafe's Jev models, is the supported provider.
//
// Deciding what to do with an answer is the caller's job: the probabilities and
// the [ChoiceAnswer.Confidence] are reported, never thresholded here. An input
// assembled from untrusted content can steer an answer the way it can steer a
// chat model, so treat a classification as advice from a model, not as an
// authorization.
package classify
