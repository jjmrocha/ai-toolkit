// Package decision is a client for decision models: models that answer
// typed questions about a state with probabilities instead of text. Create a
// [Decision] with [New] and call [Decision.Ask] with a [YesNo],
// [Choice] or [Score] question. Each typed [Answer] carries the probabilities
// behind it. The one provider is OpenRouter.
//
// What to do with an answer is up to the caller: the package reports the
// probabilities and [ChoiceAnswer.Confidence] and applies no threshold. State
// built from untrusted content can steer an answer just as it can steer a chat
// model, so treat an answer as a model's advice, not as authorization.
package decision
