package provider

import (
	"context"
	"errors"
	"fmt"
)

// Config describes the minimum information required to instantiate a provider implementation.
type Config struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
}

// ModelInfo contains provider specific model metadata.
type ModelInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// LLMProvider describes the common capabilities we need from each vendor specific client.
type LLMProvider interface {
	// ListModels returns the list of models available for the configured provider/key combination.
	ListModels(ctx context.Context) ([]ModelInfo, error)
	// Generate executes the provided prompt with the configured model and returns:
	// - the raw LLM text output,
	// - a sanitized debug representation of the API call (no API keys, no raw prompt; placeholders instead),
	// - and an error if the call failed.
	Generate(ctx context.Context, prompt string) (string, string, error)
}

// Factory builds provider implementations based on the given configuration.
func Factory(cfg Config) (LLMProvider, error) {
	switch cfg.Provider {
	case "", "none":
		return nil, errors.New("provider is not configured")
	case "openai":
		return newOpenAIProvider(cfg)
	case "openrouter":
		return newOpenRouterProvider(cfg)
	case "requesty":
		return newRequestyProvider(cfg)
	case "gemini":
		return newGeminiProvider(cfg)
	default:
		return nil, fmt.Errorf("provider %s is not supported", cfg.Provider)
	}
}
