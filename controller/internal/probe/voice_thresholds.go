package probe

import (
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
)

// VoiceThresholds holds the voice-quality heuristic thresholds used by
// the analysis engine and surfaced in the voice report PDF. Operators
// can override these at three levels (lowest priority first):
//
//  1. Built-in defaults (VoiceDefaultThresholds)
//  2. Admin global override (admin_settings.voice_thresholds)
//  3. Per-workspace override (workspace.settings.voice_thresholds)
//
// Use ResolveVoiceThresholds to merge them in priority order.
//
// Why are these configurable? The defaults target typical broadband
// paths. A site with MPLS, satellite uplink, or a 4G failover can
// generate noise that's expected for its link type and not actually a
// problem. Letting admins / workspace owners tune the rules keeps the
// report signal-to-noise high for everyone.
type VoiceThresholds struct {
	// Jitter (ms). >= WarningJitterMs fires a warning;
	// >= CriticalJitterMs fires a critical.
	WarningJitterMs  float64 `json:"warning_jitter_ms"`
	CriticalJitterMs float64 `json:"critical_jitter_ms"`

	// Sudden jitter increase: current > baseline * JitterSpikeMultiplier
	// fires a warning (independent of absolute thresholds).
	JitterSpikeMultiplier float64 `json:"jitter_spike_multiplier"`

	// Packet loss (%). >= WarningLossPct fires a warning;
	// >= CriticalLossPct fires a critical.
	WarningLossPct  float64 `json:"warning_loss_pct"`
	CriticalLossPct float64 `json:"critical_loss_pct"`

	// "New" loss = baseline near zero (< NewLossBaselineMaxPct) and
	// current >= NewLossCurrentMinPct. Surfaces loss that appeared
	// after a quiet baseline period.
	NewLossBaselineMaxPct float64 `json:"new_loss_baseline_max_pct"`
	NewLossCurrentMinPct  float64 `json:"new_loss_current_min_pct"`

	// Asymmetry: return/forward MOS ratio below AsymmetryMosRatioMin
	// (with forward MOS > AsymmetryMinForwardMos) fires a warning.
	AsymmetryMosRatioMin   float64 `json:"asymmetry_mos_ratio_min"`
	AsymmetryMinForwardMos float64 `json:"asymmetry_min_forward_mos"`

	// Latency-only degradation: latency >= LatencyOnlyMinMs with loss
	// < LatencyOnlyMaxLossPct and MOS < LatencyOnlyMaxMos.
	LatencyOnlyMinMs      float64 `json:"latency_only_min_ms"`
	LatencyOnlyMaxLossPct float64 `json:"latency_only_max_loss_pct"`
	LatencyOnlyMaxMos     float64 `json:"latency_only_max_mos"`

	// Out-of-sequence packet reordering (% of packets).
	OutOfSequencePct float64 `json:"out_of_sequence_pct"`

	// MOS grade boundaries (mirrored from voiceGradeFromMos so the
	// table is in one place).
	ExcellentMos float64 `json:"excellent_mos"`
	GoodMos      float64 `json:"good_mos"`
	FairMos      float64 `json:"fair_mos"`
	PoorMos      float64 `json:"poor_mos"`

	// Congestion classification.
	CongestionJitterMs  float64 `json:"congestion_jitter_ms"`
	CongestionLossPct   float64 `json:"congestion_loss_pct"`
	CongestionLatencyMs float64 `json:"congestion_latency_ms"`

	// Codec label for the LLM narrative (e.g., "G.711", "G.729", "Opus").
	// Affects the recommendation text ("G.711 needs 64kbps" vs
	// "Opus tolerates higher loss"); does not change the MOS calc.
	Codec string `json:"codec"`

	// CodecTolerances, when non-empty, overrides the built-in
	// codec-aware scaling table in voice_codec.go. Keys are codec
	// names ("G.711", "G.729", "Opus", ...). Admin global and
	// per-workspace threshold overlays can set this to tune loss /
	// jitter tolerance per codec without forking the defaults.
	CodecTolerances map[string]CodecTolerance `json:"codec_tolerances,omitempty"`
}

// VoiceDefaultThresholds are the built-in defaults. They assume a
// typical broadband link with G.711 codec and ITU-T G.107 E-model MOS.
// Admins can override globally; workspaces can override per-workspace.
var VoiceDefaultThresholds = VoiceThresholds{
	WarningJitterMs:        15.0,
	CriticalJitterMs:       25.0,
	JitterSpikeMultiplier:  2.0,
	WarningLossPct:         2.0,
	CriticalLossPct:        5.0,
	NewLossBaselineMaxPct:  0.5,
	NewLossCurrentMinPct:   2.0,
	AsymmetryMosRatioMin:   0.75,
	AsymmetryMinForwardMos: 3.5,
	LatencyOnlyMinMs:       100.0,
	LatencyOnlyMaxLossPct:  0.5,
	LatencyOnlyMaxMos:      4.0,
	OutOfSequencePct:       1.0,
	ExcellentMos:           4.3,
	GoodMos:                4.0,
	FairMos:                3.6,
	PoorMos:                3.1,
	CongestionJitterMs:     15.0,
	CongestionLossPct:      1.0,
	CongestionLatencyMs:    80.0,
	Codec:                  "G.711",
}

// Clone returns a deep copy. Useful when applying a partial override
// on top of defaults without mutating the source.
func (t VoiceThresholds) Clone() VoiceThresholds {
	return t
}

// MergeOverlay overlays a non-zero override on top of the base. A zero
// value in `overlay` keeps the base value (so callers can set just the
// thresholds they want to override).
func (t VoiceThresholds) MergeOverlay(overlay *VoiceThresholds) VoiceThresholds {
	if overlay == nil {
		return t
	}
	out := t
	if overlay.WarningJitterMs > 0 {
		out.WarningJitterMs = overlay.WarningJitterMs
	}
	if overlay.CriticalJitterMs > 0 {
		out.CriticalJitterMs = overlay.CriticalJitterMs
	}
	if overlay.JitterSpikeMultiplier > 0 {
		out.JitterSpikeMultiplier = overlay.JitterSpikeMultiplier
	}
	if overlay.WarningLossPct > 0 {
		out.WarningLossPct = overlay.WarningLossPct
	}
	if overlay.CriticalLossPct > 0 {
		out.CriticalLossPct = overlay.CriticalLossPct
	}
	if overlay.NewLossBaselineMaxPct > 0 {
		out.NewLossBaselineMaxPct = overlay.NewLossBaselineMaxPct
	}
	if overlay.NewLossCurrentMinPct > 0 {
		out.NewLossCurrentMinPct = overlay.NewLossCurrentMinPct
	}
	if overlay.AsymmetryMosRatioMin > 0 {
		out.AsymmetryMosRatioMin = overlay.AsymmetryMosRatioMin
	}
	if overlay.AsymmetryMinForwardMos > 0 {
		out.AsymmetryMinForwardMos = overlay.AsymmetryMinForwardMos
	}
	if overlay.LatencyOnlyMinMs > 0 {
		out.LatencyOnlyMinMs = overlay.LatencyOnlyMinMs
	}
	if overlay.LatencyOnlyMaxLossPct > 0 {
		out.LatencyOnlyMaxLossPct = overlay.LatencyOnlyMaxLossPct
	}
	if overlay.LatencyOnlyMaxMos > 0 {
		out.LatencyOnlyMaxMos = overlay.LatencyOnlyMaxMos
	}
	if overlay.OutOfSequencePct > 0 {
		out.OutOfSequencePct = overlay.OutOfSequencePct
	}
	if overlay.ExcellentMos > 0 {
		out.ExcellentMos = overlay.ExcellentMos
	}
	if overlay.GoodMos > 0 {
		out.GoodMos = overlay.GoodMos
	}
	if overlay.FairMos > 0 {
		out.FairMos = overlay.FairMos
	}
	if overlay.PoorMos > 0 {
		out.PoorMos = overlay.PoorMos
	}
	if overlay.CongestionJitterMs > 0 {
		out.CongestionJitterMs = overlay.CongestionJitterMs
	}
	if overlay.CongestionLossPct > 0 {
		out.CongestionLossPct = overlay.CongestionLossPct
	}
	if overlay.CongestionLatencyMs > 0 {
		out.CongestionLatencyMs = overlay.CongestionLatencyMs
	}
	if overlay.Codec != "" {
		out.Codec = overlay.Codec
	}
	return out
}

// MarshalJSON / UnmarshalJSON let VoiceThresholds round-trip through
// Postgres JSONB columns and workspace.settings JSON blobs without
// callers having to think about it.
func (t VoiceThresholds) MarshalJSON() ([]byte, error) {
	type alias VoiceThresholds
	return json.Marshal(alias(t))
}

// ── AdminSettings persistence ─────────────────────────────────────────────

// AdminSettings is a single-row key/value table used to hold
// site-wide operator overrides. Today the only key we use is
// "voice_thresholds"; future defaults / branding / etc. can reuse
// the same row.
type AdminSettings struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value []byte `gorm:"type:jsonb" json:"value"`
}

func (AdminSettings) TableName() string { return "admin_settings" }

// EnsureAdminSettingsTable creates the admin_settings table if it
// does not yet exist. Called at startup; idempotent.
func EnsureAdminSettingsTable(db *gorm.DB) error {
	return db.AutoMigrate(&AdminSettings{})
}

// GetAdminVoiceThresholds returns the admin-global voice threshold
// override, or nil if none is set. Errors other than "not found" are
// returned to the caller; we use the gorm.ErrRecordNotFound sentinel
// to mean "no override" (the common case for fresh installs).
func GetAdminVoiceThresholds(db *gorm.DB) (*VoiceThresholds, error) {
	var row AdminSettings
	if err := db.Where("key = ?", "voice_thresholds").First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	if len(row.Value) == 0 {
		return nil, nil
	}
	var out VoiceThresholds
	if err := json.Unmarshal(row.Value, &out); err != nil {
		return nil, fmt.Errorf("admin voice_thresholds JSON parse: %w", err)
	}
	return &out, nil
}

// SetAdminVoiceThresholds persists an admin-global override. Pass nil
// to clear it.
func SetAdminVoiceThresholds(db *gorm.DB, t *VoiceThresholds) error {
	row := AdminSettings{Key: "voice_thresholds"}
	if t == nil {
		row.Value = []byte("null")
	} else {
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		row.Value = b
	}
	return db.Save(&row).Error
}

// ── Workspace settings resolver ──────────────────────────────────────────

// ResolveVoiceThresholds builds the effective VoiceThresholds for a
// workspace by layering (lowest to highest priority):
//
//	defaults → admin global override → workspace override
//
// `workspaceSettingsJSON` is the raw `Workspace.Settings` blob. The
// caller doesn't need to parse it; we look for a `voice_thresholds`
// key here so workspace owners can override without knowing the rest
// of the settings schema.
func ResolveVoiceThresholds(db *gorm.DB, workspaceSettingsJSON []byte) (VoiceThresholds, error) {
	out := VoiceDefaultThresholds

	admin, err := GetAdminVoiceThresholds(db)
	if err != nil {
		return out, fmt.Errorf("resolve admin thresholds: %w", err)
	}
	out = out.MergeOverlay(admin)

	if len(workspaceSettingsJSON) > 0 {
		var ws struct {
			VoiceThresholds *VoiceThresholds `json:"voice_thresholds"`
		}
		if err := json.Unmarshal(workspaceSettingsJSON, &ws); err == nil && ws.VoiceThresholds != nil {
			out = out.MergeOverlay(ws.VoiceThresholds)
		}
		// Malformed workspace settings shouldn't break the report —
		// fall through with the merged defaults.
	}
	return out, nil
}
