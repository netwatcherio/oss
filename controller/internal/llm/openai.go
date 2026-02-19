package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OpenAIProvider implements Provider using the OpenAI API (also compatible with
// any OpenAI-compatible endpoint like Azure, Together, Groq, etc.)
type OpenAIProvider struct {
	apiURL string
	apiKey string
	model  string
	client *http.Client
}

// NewOpenAIProvider creates a new OpenAI-compatible provider
func NewOpenAIProvider(cfg Config) *OpenAIProvider {
	return &OpenAIProvider{
		apiURL: cfg.APIURL,
		apiKey: cfg.APIKey,
		model:  cfg.Model,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (p *OpenAIProvider) Available() bool { return p.apiKey != "" }
func (p *OpenAIProvider) Name() string    { return "openai" }

func (p *OpenAIProvider) Summarize(ctx context.Context, req SummarizeRequest) (string, error) {
	contextJSON, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshaling context: %w", err)
	}

	body := map[string]any{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "system", "content": SystemPrompt},
			{"role": "user", "content": fmt.Sprintf("Summarize this network analysis:\n\n```json\n%s\n```", string(contextJSON))},
		},
		"max_tokens":  256,
		"temperature": 0.3,
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.apiURL+"/chat/completions", bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return result.Choices[0].Message.Content, nil
}
