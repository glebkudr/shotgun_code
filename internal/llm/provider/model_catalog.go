package provider

import "fmt"

var openAIModelCatalog = []ModelInfo{
	// GPT-5 family (latest reasoning-capable models)
	{Name: "gpt-5.1", Description: "Latest GPT-5.1 flagship for complex reasoning and coding tasks"},
	{Name: "gpt-5", Description: "Previous GPT-5 flagship reasoning model"},
	{Name: "gpt-5-mini", Description: "Cost-optimized GPT-5 mini model"},
	{Name: "gpt-5-nano", Description: "High-throughput GPT-5 nano model"},

	// GPT-4 family
	{Name: "gpt-4o-mini", Description: "Latest GPT-4o mini for general reasoning"},
	{Name: "gpt-4.1-mini", Description: "GPT-4.1 mini tier"},
	{Name: "o4-mini", Description: "Reasoning optimized 04-mini"},
	{Name: "gpt-4o", Description: "Full GPT-4o"},
	{Name: "gpt-4.1", Description: "Full GPT-4.1"},
}

var openRouterModelCatalog = []ModelInfo{
	{Name: "openai/gpt-5", Description: "GPT-5 family routed via OpenRouter"},
	{Name: "anthropic/claude-4.5-sonnet", Description: "Claude 4.5 Sonnet via OpenRouter"},
	{Name: "google/gemini-2.5-pro", Description: "Gemini 2.5 Pro via OpenRouter"},
	{Name: "google/gemini-2.5-flash", Description: "Gemini 2.5 Flash via OpenRouter"},
	{Name: "google/gemini-2.0-flash", Description: "Gemini 2.0 Flash via OpenRouter"},
	{Name: "openai/gpt-4o-mini", Description: "GPT-4o mini from OpenRouter catalog"},
	{Name: "meta-llama/llama-3.1-70b-instruct", Description: "Llama 3.1 70B Instruct via OpenRouter"},
	{Name: "x-ai/grok-code-fast-1", Description: "Grok Code Fast 1 via OpenRouter"},
	{Name: "x-ai/grok-4-fast", Description: "Grok 4 Fast via OpenRouter"},
	{Name: "minimax/minimax-m2", Description: "Minimax M2 via OpenRouter"},
	{Name: "z-ai/glm-4.6", Description: "GLM 4.6 via OpenRouter"},
}

var requestyModelCatalog = []ModelInfo{
	{Name: "openai/gpt-4o-mini", Description: "GPT-4o mini via Requesty"},
	{Name: "openai/gpt-4o", Description: "GPT-4o via Requesty"},
	{Name: "openai/gpt-4.1", Description: "GPT-4.1 via Requesty"},
	{Name: "anthropic/claude-sonnet-4-5", Description: "Claude Sonnet 4.5 via Requesty"},
	{Name: "google/gemini-2.5-flash", Description: "Gemini 2.5 Flash via Requesty"},
	{Name: "google/gemini-2.5-pro", Description: "Gemini 2.5 Pro via Requesty"},
	{Name: "deepseek/deepseek-chat", Description: "DeepSeek Chat via Requesty"},
	{Name: "x-ai/grok-4-fast", Description: "Grok 4 Fast via Requesty"},
}

var geminiModelCatalog = []ModelInfo{
	{Name: "gemini-2.5-pro", Description: "Most capable Gemini 2.5 Pro"},
	{Name: "gemini-2.5-flash", Description: "Flash"},
}

func cloneModelCatalog(models []ModelInfo) []ModelInfo {
	if len(models) == 0 {
		return nil
	}
	out := make([]ModelInfo, len(models))
	copy(out, models)
	return out
}

// ModelCatalog returns a provider specific list of models without requiring the provider to be fully configured.
func ModelCatalog(providerName string) ([]ModelInfo, error) {
	switch providerName {
	case "openai":
		return cloneModelCatalog(openAIModelCatalog), nil
	case "openrouter":
		return cloneModelCatalog(openRouterModelCatalog), nil
	case "requesty":
		return cloneModelCatalog(requestyModelCatalog), nil
	case "gemini":
		return cloneModelCatalog(geminiModelCatalog), nil
	default:
		return nil, fmt.Errorf("provider %s is not supported", providerName)
	}
}
