// Package llm provides optional LLM integration for enriching analysis summaries.
// When configured, it enhances the rule-based analysis engine with natural language
// summaries. When not configured, the system works identically without it.
package llm

import (
	"context"
	"os"
	"time"
)

// SummarizeRequest contains the pre-processed analysis context sent to the LLM.
// No raw probe data, IPs, or PII — only the structured analysis output.
type SummarizeRequest struct {
	// Pre-processed incidents from the rule engine
	Incidents []IncidentSummary `json:"incidents"`

	// High-level workspace context
	WorkspaceName string  `json:"workspace_name"`
	HealthScore   float64 `json:"health_score"`
	HealthGrade   string  `json:"health_grade"`
	Status        string  `json:"status"` // healthy, degraded, outage, unknown
	TotalAgents   int     `json:"total_agents"`
	OnlineAgents  int     `json:"online_agents"`
	TotalProbes   int     `json:"total_probes"`
}

// IncidentSummary is a simplified view of DetectedIncident for LLM context
type IncidentSummary struct {
	Title           string   `json:"title"`
	Severity        string   `json:"severity"`
	Scope           string   `json:"scope"`
	SuggestedCause  string   `json:"suggested_cause"`
	AffectedAgents  []string `json:"affected_agents"`
	AffectedTargets []string `json:"affected_targets"`
	Evidence        []string `json:"evidence"`
}

// Provider defines the interface for LLM providers.
// Implementations must be safe for concurrent use.
type Provider interface {
	// Summarize generates a natural language summary of the analysis.
	// Returns the enriched summary text, or error if the LLM call fails.
	// The caller should fall back to rule-based summary on error.
	Summarize(ctx context.Context, req SummarizeRequest) (string, error)

	// Available returns true if the provider is properly configured.
	Available() bool

	// Name returns the provider name (e.g., "openai", "ollama")
	Name() string
}

// Config holds LLM configuration loaded from environment
type Config struct {
	Provider    string // "openai", "ollama", or "" (disabled)
	APIKey      string // API key for OpenAI/Anthropic
	APIURL      string // API endpoint
	Model       string // Model name
	OllamaURL   string // Ollama endpoint
	OllamaModel string // Ollama model name
	Timeout     time.Duration
}

// LoadConfig loads LLM configuration from environment variables
func LoadConfig() Config {
	return Config{
		Provider:    os.Getenv("LLM_PROVIDER"),
		APIKey:      os.Getenv("LLM_API_KEY"),
		APIURL:      envOrDefault("LLM_API_URL", "https://api.openai.com/v1"),
		Model:       envOrDefault("LLM_MODEL", "gpt-4o-mini"),
		OllamaURL:   envOrDefault("OLLAMA_URL", "http://localhost:11434"),
		OllamaModel: envOrDefault("OLLAMA_MODEL", "llama3.2"),
		Timeout:     30 * time.Second,
	}
}

// NewProvider creates an LLM provider based on configuration.
// Returns nil if LLM is not configured (disabled by default).
func NewProvider(cfg Config) Provider {
	switch cfg.Provider {
	case "openai":
		if cfg.APIKey == "" {
			return nil
		}
		return NewOpenAIProvider(cfg)
	case "ollama":
		return NewOllamaProvider(cfg)
	default:
		return nil // LLM disabled
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// SystemPrompt is the base prompt for network analysis summarization
const SystemPrompt = `You are a network operations assistant for NetWatcher, a network monitoring platform. 
Your job is to take structured incident data and produce a clear, concise, actionable summary for a network administrator.

Rules:
- Be concise: 2-3 sentences maximum for the overall summary
- Use networking terminology appropriately but avoid unnecessary jargon
- If multiple incidents share a root cause, correlate them
- Focus on impact and action, not raw numbers
- If everything is healthy, say so briefly
- Never fabricate data or metrics not provided in the context`
