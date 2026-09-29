// Package classify is a client for classification models: models that answer
// typed questions about an input with probabilities instead of text. Create a
// [Classifier] with [New] and call [Classifier.Classify] with a [YesNo],
// [Choice] or [Score] question. Each typed [Answer] carries the probabilities
// behind it. The one provider is OpenRouter, which serves TypeSafe's Jev models.
//
// What to do with an answer is up to the caller: the package reports the
// probabilities and [ChoiceAnswer.Confidence] and applies no threshold. Input
// built from untrusted content can steer an answer just as it can steer a chat
// model, so treat a classification as a model's advice, not as authorization.
package classify
