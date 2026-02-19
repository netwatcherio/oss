package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OllamaProvider implements Provider using a local Ollama instance.
// No external API calls — works in air-gapped environments.
type OllamaProvider struct {
	url    string
	model  string
	client *http.Client
}

// NewOllamaProvider creates a new Ollama provider for local/self-hosted models
func NewOllamaProvider(cfg Config) *OllamaProvider {
	return &OllamaProvider{
		url:    cfg.OllamaURL,
		model:  cfg.OllamaModel,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (p *OllamaProvider) Available() bool { return p.url != "" }
func (p *OllamaProvider) Name() string    { return "ollama" }

func (p *OllamaProvider) Summarize(ctx context.Context, req SummarizeRequest) (string, error) {
	contextJSON, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshaling context: %w", err)
	}

	body := map[string]any{
		"model":  p.model,
		"system": SystemPrompt,
		"prompt": fmt.Sprintf("Summarize this network analysis:\n\n```json\n%s\n```", string(contextJSON)),
		"stream": false,
		"options": map[string]any{
			"temperature": 0.3,
			"num_predict": 256,
		},
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.url+"/api/generate", bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("Ollama request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	return result.Response, nil
}
