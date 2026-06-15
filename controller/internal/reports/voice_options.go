package reports

import (
	"strings"
)

// AgentReportOptions controls which optional sections the agent
// voice report PDF renders. Operators toggle these from the frontend
// modal; the backend parses them from the API request into this
// struct and the PDF generator dispatches on the bit set.
//
// Keeping the options as a single struct (rather than a bunch of
// booleans on the Generator) lets us add sections without changing
// every render call site and makes the test surface stable.
type AgentReportOptions struct {
	// All=true turns on every optional section. Used for the
	// "generate full report" path (and as the test default).
	All bool

	// Per-section flags. The defaults match the "Quick 7-day" preset
	// in the frontend modal.
	IncludeExecutive   bool
	IncludeTimeline    bool
	IncludeAggregate   bool
	IncludePerProbe    bool
	IncludeIssues      bool
	IncludeCorrelation bool
	IncludeAppendix    bool
	IncludeRawJSON     bool
}

// DefaultAgentReportOptions returns the operator-friendly defaults
// (executive + timeline + per-probe + issues; skip aggregate summary,
// correlation, methodology, raw JSON to keep the report focused).
func DefaultAgentReportOptions() AgentReportOptions {
	return AgentReportOptions{
		IncludeExecutive:   true,
		IncludeTimeline:    true,
		IncludeAggregate:   false,
		IncludePerProbe:    true,
		IncludeIssues:      true,
		IncludeCorrelation: true,
		IncludeAppendix:    false,
		IncludeRawJSON:     false,
	}
}

// FullAgentReportOptions returns the union of all sections — used
// for the "full" preset in the modal and as a convenience for
// internal callers / tests.
func FullAgentReportOptions() AgentReportOptions {
	return AgentReportOptions{
		All:                true,
		IncludeExecutive:   true,
		IncludeTimeline:    true,
		IncludeAggregate:   true,
		IncludePerProbe:    true,
		IncludeIssues:      true,
		IncludeCorrelation: true,
		IncludeAppendix:    true,
		IncludeRawJSON:     true,
	}
}

// ParseAgentReportSections interprets a comma-separated `sections=`
// query parameter (e.g. "summary,timeline,probes,issues"). Unknown
// tokens are ignored. The empty string yields DefaultAgentReportOptions.
// "all" yields FullAgentReportOptions. "none" yields an empty struct
// (cover page only — useful for debugging).
func ParseAgentReportSections(csv string) AgentReportOptions {
	out := DefaultAgentReportOptions()
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return out
	}
	parts := strings.Split(csv, ",")
	any := false
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		any = true
		switch strings.ToLower(p) {
		case "all":
			return FullAgentReportOptions()
		case "none":
			return AgentReportOptions{}
		case "summary", "executive":
			out.IncludeExecutive = true
		case "timeline":
			out.IncludeTimeline = true
		case "aggregate":
			out.IncludeAggregate = true
		case "probes", "per_probe":
			out.IncludePerProbe = true
		case "issues":
			out.IncludeIssues = true
		case "correlation", "workspace":
			out.IncludeCorrelation = true
		case "appendix", "methodology":
			out.IncludeAppendix = true
		case "raw", "json":
			out.IncludeRawJSON = true
		// Negation: "noissues" etc. — operator can suppress a default.
		case "noexecutive":
			out.IncludeExecutive = false
		case "notimeline":
			out.IncludeTimeline = false
		case "noaggregate":
			out.IncludeAggregate = false
		case "noprobes":
			out.IncludePerProbe = false
		case "noissues":
			out.IncludeIssues = false
		case "nocorrelation":
			out.IncludeCorrelation = false
		case "noappendix":
			out.IncludeAppendix = false
		case "noraw":
			out.IncludeRawJSON = false
		}
	}
	if !any {
		return DefaultAgentReportOptions()
	}
	return out
}

// IsAnySectionEnabled returns true if at least one optional section
// is on. Used to decide whether the "empty body" message replaces
// the per-section blocks.
func (o AgentReportOptions) IsAnySectionEnabled() bool {
	return o.IncludeExecutive || o.IncludeTimeline || o.IncludeAggregate ||
		o.IncludePerProbe || o.IncludeIssues || o.IncludeCorrelation ||
		o.IncludeAppendix || o.IncludeRawJSON
}
