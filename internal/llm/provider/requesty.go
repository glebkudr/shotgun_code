package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/tmc/langchaingo/llms"
	openai "github.com/tmc/langchaingo/llms/openai"
)

const defaultRequestyBaseURL = "https://router.requesty.ai/v1"

type requestyProvider struct {
	model   string
	client  *openai.LLM
	apiKey  string
	baseURL string
}

func newRequestyProvider(cfg Config) (LLMProvider, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("requesty provider requires an API key")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("requesty provider requires a model")
	}

	baseURL := defaultRequestyBaseURL
	if trimmed := strings.TrimSpace(cfg.BaseURL); trimmed != "" {
		baseURL = trimmed
	}

	opts := []openai.Option{
		openai.WithToken(strings.TrimSpace(cfg.APIKey)),
		openai.WithModel(strings.TrimSpace(cfg.Model)),
		openai.WithBaseURL(baseURL),
	}

	client, err := openai.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to init requesty client: %w", err)
	}

	return &requestyProvider{
		client:  client,
		model:   strings.TrimSpace(cfg.Model),
		apiKey:  strings.TrimSpace(cfg.APIKey),
		baseURL: baseURL,
	}, nil
}

func (o *requestyProvider) ListModels(_ context.Context) ([]ModelInfo, error) {
	return ModelCatalog("requesty")
}

func (o *requestyProvider) Generate(ctx context.Context, prompt string) (string, string, error) {
	if o.client == nil {
		return "", "", errors.New("requesty client is not configured")
	}

	// For GPT-5 family models use a manual Requesty Chat Completions API call
	// with explicit reasoning.effort and text.verbosity and without temperature.
	if isGPT5FamilyModel(o.model) {
		return o.generateViaRequestyAPI(ctx, prompt)
	}

	// For other models keep the current behavior via langchaingo.
	output, err := llms.GenerateFromSinglePrompt(ctx, o.client, prompt,
		llms.WithModel(o.model),
		llms.WithTemperature(0.1),
	)

	debug := o.buildGenericAPICallDebug()

	if err != nil {
		return "", debug, err
	}
	return output, debug, nil
}

type requestyChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type requestyChatChoice struct {
	Message requestyChatMessage `json:"message"`
}

type requestyChatResponse struct {
	Choices []requestyChatChoice `json:"choices"`
}

type requestyReasoningConfig struct {
	Effort string `json:"effort"`
}

type requestyTextConfig struct {
	Verbosity string `json:"verbosity"`
}

type requestyChatRequest struct {
	Model     string                  `json:"model"`
	Messages  []requestyChatMessage   `json:"messages"`
	Reasoning requestyReasoningConfig `json:"reasoning"`
	Text      requestyTextConfig      `json:"text"`
}

func (o *requestyProvider) generateViaRequestyAPI(ctx context.Context, prompt string) (string, string, error) {
	apiKey := strings.TrimSpace(o.apiKey)
	if apiKey == "" {
		return "", "", errors.New("requesty API key is required for GPT-5 models")
	}

	baseURL := strings.TrimSpace(o.baseURL)
	if baseURL == "" {
		baseURL = defaultRequestyBaseURL
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/chat/completions"

	payload := requestyChatRequest{
		Model: o.model,
		Messages: []requestyChatMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Reasoning: requestyReasoningConfig{
			Effort: "medium",
		},
		Text: requestyTextConfig{
			Verbosity: "high",
		},
	}

	// Build sanitized debug view BEFORE marshalling real payload.
	debugPayload := requestyChatRequest{
		Model: o.model,
		Messages: []requestyChatMessage{
			{
				Role:    "user",
				Content: "[request_text]",
			},
		},
		Reasoning: requestyReasoningConfig{
			Effort: "medium",
		},
		Text: requestyTextConfig{
			Verbosity: "high",
		},
	}

	debug := map[string]any{
		"provider": "requesty",
		"endpoint": endpoint,
		"method":   http.MethodPost,
		"headers": map[string]string{
			"Authorization": "Bearer [apikey]",
			"Content-Type":  "application/json",
		},
		"body": debugPayload,
	}
	debugBytes, err := json.MarshalIndent(debug, "", "  ")
	if err != nil {
		debugBytes = nil
	}
	debugString := string(debugBytes)

	body, err := json.Marshal(payload)
	if err != nil {
		return "", debugString, fmt.Errorf("failed to marshal Requesty chat payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", debugString, fmt.Errorf("failed to create Requesty chat request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("requesty chat request failed (model=%s): %v", o.model, err)
		return "", debugString, fmt.Errorf("requesty chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		limitedBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		log.Printf("requesty chat returned status %d for model %s: %s", resp.StatusCode, o.model, string(limitedBody))
		return "", debugString, fmt.Errorf("requesty chat API returned non-2xx status %d", resp.StatusCode)
	}

	var decoded requestyChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		log.Printf("failed to decode requesty chat payload for model %s: %v", o.model, err)
		return "", debugString, fmt.Errorf("failed to decode requesty chat payload: %w", err)
	}

	if len(decoded.Choices) == 0 {
		log.Printf("requesty chat response did not contain any choices for model %s", o.model)
		return "", debugString, errors.New("requesty chat response did not contain any choices")
	}

	text := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if text == "" {
		log.Printf("requesty chat response contained empty message content for model %s", o.model)
		return "", debugString, errors.New("requesty chat response did not contain text output")
	}

	return text, debugString, nil
}

// buildGenericAPICallDebug builds a high-level debug representation for SDK-based calls (non-GPT-5).
func (o *requestyProvider) buildGenericAPICallDebug() string {
	debug := map[string]any{
		"provider": "requesty",
		"model":    o.model,
		"baseURL":  o.baseURL,
		"sdk":      "langchaingo/llms.openai",
		"call":     "llms.GenerateFromSinglePrompt",
		"input":    "[request_text]",
		"headers": map[string]string{
			"Authorization": "Bearer [apikey]",
		},
	}

	data, err := json.MarshalIndent(debug, "", "  ")
	if err != nil {
		return ""
	}
	return string(data)
}
