package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"shotgun_code/internal/llm/provider"
)

type cachedProvider struct {
	cfg      provider.Config
	instance provider.LLMProvider
}

func normalizeProviderName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case LLMProviderOpenAI:
		return LLMProviderOpenAI
	case LLMProviderOpenRouter:
		return LLMProviderOpenRouter
	case LLMProviderRequesty:
		return LLMProviderRequesty
	case LLMProviderGemini:
		return LLMProviderGemini
	default:
		return ""
	}
}

func defaultModelForProvider(providerName string) string {
	switch providerName {
	case LLMProviderOpenAI:
		return "gpt-5"
	case LLMProviderGemini:
		return "gemini-2.5-pro"
	case LLMProviderOpenRouter:
		return "openai/gpt-5"
	case LLMProviderRequesty:
		return "openai/gpt-4o-mini"
	default:
		return ""
	}
}

func (l LLMSettings) keyForProvider(providerName string) string {
	switch normalizeProviderName(providerName) {
	case LLMProviderOpenAI:
		return strings.TrimSpace(l.OpenAIKey)
	case LLMProviderOpenRouter:
		return strings.TrimSpace(l.OpenRouterKey)
	case LLMProviderRequesty:
		return strings.TrimSpace(l.RequestyKey)
	case LLMProviderGemini:
		return strings.TrimSpace(l.GeminiKey)
	default:
		return ""
	}
}

func (a *App) ensureLLMSettingsDefaults() {
	settings := &a.settings.LLMSettings
	settings.ActiveProvider = normalizeProviderName(settings.ActiveProvider)
	settings.Model = strings.TrimSpace(settings.Model)
	settings.BaseURL = strings.TrimSpace(settings.BaseURL)
	settings.OpenAIKey = strings.TrimSpace(settings.OpenAIKey)
	settings.OpenRouterKey = strings.TrimSpace(settings.OpenRouterKey)
	settings.RequestyKey = strings.TrimSpace(settings.RequestyKey)
	settings.GeminiKey = strings.TrimSpace(settings.GeminiKey)

	if settings.ActiveProvider != "" && settings.keyForProvider(settings.ActiveProvider) == "" {
		runtime.LogWarning(a.ctx, "Active LLM provider is missing an API key; disabling auto-context.")
		settings.ActiveProvider = ""
		settings.Model = ""
	}
	if settings.ActiveProvider != "" && settings.Model == "" {
		settings.Model = defaultModelForProvider(settings.ActiveProvider)
	}
}

func (a *App) HasActiveLlmKey() bool {
	settings := a.settings.LLMSettings
	return settings.ActiveProvider != "" && settings.keyForProvider(settings.ActiveProvider) != ""
}

func (a *App) GetLlmSettings() LLMSettings {
	return a.settings.LLMSettings
}

func (a *App) SetLlmApiKey(providerName, apiKey string) error {
	providerName = normalizeProviderName(providerName)
	if providerName == "" {
		return errors.New("unknown provider")
	}
	apiKey = strings.TrimSpace(apiKey)
	switch providerName {
	case LLMProviderOpenAI:
		a.settings.LLMSettings.OpenAIKey = apiKey
	case LLMProviderOpenRouter:
		a.settings.LLMSettings.OpenRouterKey = apiKey
	case LLMProviderRequesty:
		a.settings.LLMSettings.RequestyKey = apiKey
	case LLMProviderGemini:
		a.settings.LLMSettings.GeminiKey = apiKey
	}
	if a.settings.LLMSettings.ActiveProvider == providerName && strings.TrimSpace(a.settings.LLMSettings.Model) == "" {
		a.settings.LLMSettings.Model = defaultModelForProvider(providerName)
	}
	a.ensureLLMSettingsDefaults()
	if err := a.saveSettings(); err != nil {
		return fmt.Errorf("failed to persist API key: %w", err)
	}
	a.invalidateProviderCache()
	return nil
}

func (a *App) SetLlmProvider(providerName string) error {
	providerName = normalizeProviderName(providerName)
	if providerName == "" {
		a.settings.LLMSettings.ActiveProvider = ""
		a.settings.LLMSettings.Model = ""
		a.invalidateProviderCache()
		return a.saveSettings()
	}
	if a.settings.LLMSettings.keyForProvider(providerName) == "" {
		return fmt.Errorf("set API key for %s before activating it", providerName)
	}
	a.settings.LLMSettings.ActiveProvider = providerName
	if strings.TrimSpace(a.settings.LLMSettings.Model) == "" {
		a.settings.LLMSettings.Model = defaultModelForProvider(providerName)
	}
	a.ensureLLMSettingsDefaults()
	if err := a.saveSettings(); err != nil {
		return fmt.Errorf("failed to save provider: %w", err)
	}
	a.invalidateProviderCache()
	return nil
}

func (a *App) SetLlmModel(providerName, model string) error {
	providerName = normalizeProviderName(providerName)
	if providerName == "" {
		return errors.New("unknown provider")
	}
	if a.settings.LLMSettings.keyForProvider(providerName) == "" {
		return fmt.Errorf("set API key for %s before selecting a model", providerName)
	}
	if strings.TrimSpace(model) == "" {
		return errors.New("model name is required")
	}
	a.settings.LLMSettings.ActiveProvider = providerName
	a.settings.LLMSettings.Model = strings.TrimSpace(model)
	a.ensureLLMSettingsDefaults()

	if err := a.saveSettings(); err != nil {
		return fmt.Errorf("failed to save model selection: %w", err)
	}
	a.invalidateProviderCache()
	return nil
}

func (a *App) SetLlmBaseURL(baseURL string) error {
	a.settings.LLMSettings.BaseURL = strings.TrimSpace(baseURL)
	if err := a.saveSettings(); err != nil {
		return fmt.Errorf("failed to save base URL: %w", err)
	}
	a.invalidateProviderCache()
	return nil
}

func (a *App) ListLlmModels(providerName string) ([]provider.ModelInfo, error) {
	providerName = normalizeProviderName(providerName)
	if providerName == "" {
		return nil, errors.New("unknown provider")
	}
	return provider.ModelCatalog(providerName)
}

func (a *App) invalidateProviderCache() {
	a.llmCache = cachedProvider{}
}

func (a *App) getOrCreateProvider(cfg provider.Config) (provider.LLMProvider, error) {
	if cfg.Provider == "" || cfg.APIKey == "" || cfg.Model == "" {
		return nil, errors.New("incomplete provider configuration")
	}
	if a.llmCache.instance != nil && a.llmCache.cfg == cfg {
		return a.llmCache.instance, nil
	}
	instance, err := provider.Factory(cfg)
	if err != nil {
		return nil, err
	}
	a.llmCache = cachedProvider{
		cfg:      cfg,
		instance: instance,
	}
	return instance, nil
}
