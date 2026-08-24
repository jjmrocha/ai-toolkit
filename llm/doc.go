// Package llm provides a provider-agnostic client for chat-based large language
// models. Construct an [LLM] with [New], then call [LLM.Chat] to exchange
// messages and [LLM.ModelInfo] to query model metadata. OpenRouter, Ollama, and
// Anthropic are the supported providers.
package llm
