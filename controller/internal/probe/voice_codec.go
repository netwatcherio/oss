package probe

// voice_codec.go
//
// Codec-aware voice-quality threshold scaling.
//
// Different VoIP codecs tolerate loss and jitter differently. G.711
// (uncompressed PCM, 64 kbps) is the baseline — every packet is
// audible, so even 1% loss is noticeable. G.729 has PLC (packet loss
// concealment) and tolerates ~3% loss without audible degradation.
// Opus is even more tolerant (5%+) and adapts its bitrate to
// network conditions.
//
// This file provides `EffectiveThresholds` which returns a copy of
// the base thresholds scaled by the configured codec. Workspace or
// admin operators can also override `CodecTolerances` in the
// VoiceThresholds struct itself — those overrides win over the
// built-in table.

// CodecTolerance describes how much of a codec's tolerance envelope
// to apply when scaling a base threshold. Values > 1 make the
// threshold more lenient (e.g., a 2.0 loss multiplier means
// WarningLossPct doubles for this codec).
type CodecTolerance struct {
	LossMultiplier   float64 `json:"loss_multiplier"`
	JitterMultiplier float64 `json:"jitter_multiplier"`
	Description      string  `json:"description,omitempty"`
}

// Built-in codec tolerances. Loss and jitter are scaled
// multiplicatively against the base VoiceThresholds:
//
//	G.711  → 1.0× baseline (worst case)
//	G.729  → 1.5× loss tolerance (PLC hides short gaps)
//	G.723  → 1.4× loss tolerance (similar PLC profile to G.729)
//	Opus   → 2.0× loss tolerance (most forgiving; adaptively coded)
//
// Jitter is scaled the same way for simplicity — true jitter
// tolerance is codec-clock-recovery-dependent but the difference
// across the common codecs is small enough that a single
// multiplier is fine for warning/critical decision-making.
var builtInCodecTolerances = map[string]CodecTolerance{
	"G.711": {
		LossMultiplier:   1.0,
		JitterMultiplier: 1.0,
		Description:      "Uncompressed PCM — every packet is audible",
	},
	"G.729": {
		LossMultiplier:   1.5,
		JitterMultiplier: 1.1,
		Description:      "8 kbps CS-ACELP with PLC — tolerates short gaps",
	},
	"G.723": {
		LossMultiplier:   1.4,
		JitterMultiplier: 1.1,
		Description:      "Low-bitrate codec with PLC — similar to G.729",
	},
	"Opus": {
		LossMultiplier:   2.0,
		JitterMultiplier: 1.2,
		Description:      "Adaptive codec — most loss-tolerant of the common codecs",
	},
}

// EffectiveThresholds returns a copy of `base` with loss and jitter
// thresholds scaled by the codec tolerance. The codec is resolved
// first from `base.Codec`, then from `overrides`, then falls back to
// G.711. The original base is returned unchanged if the codec is
// unrecognized or the base thresholds are zero.
//
// Admin and workspace operators can attach their own
// `CodecTolerances` map to `VoiceThresholds` (see MergeOverlay in
// voice_thresholds.go) — those entries override the built-in table.
func EffectiveThresholds(base VoiceThresholds) VoiceThresholds {
	out := base.Clone()
	codec := out.Codec
	if codec == "" {
		codec = "G.711"
		out.Codec = codec // keep the resolved codec on the returned struct so callers can see it
	}

	tol, ok := builtInCodecTolerances[codec]
	if !ok {
		// Unknown codec — fall back to G.711 (safest baseline).
		tol = builtInCodecTolerances["G.711"]
	}
	// Workspace/admin overrides win over built-in tolerances.
	if out.CodecTolerances != nil {
		if override, ok := out.CodecTolerances[codec]; ok {
			tol = override
		}
	}

	if out.WarningLossPct > 0 {
		out.WarningLossPct *= tol.LossMultiplier
	}
	if out.CriticalLossPct > 0 {
		out.CriticalLossPct *= tol.LossMultiplier
	}
	if out.WarningJitterMs > 0 {
		out.WarningJitterMs *= tol.JitterMultiplier
	}
	if out.CriticalJitterMs > 0 {
		out.CriticalJitterMs *= tol.JitterMultiplier
	}
	return out
}

// DetectCodecFromPayload sniffs the codec name out of a TrafficSim
// payload's known fields. The agent doesn't currently report a
// `codec` field (the codec is implied by the RTP profile), so this is
// best-effort: when the payload includes a `codec` string we honour
// it, otherwise we default to G.711.
//
// Hooked into ComputeAgentVoiceQuality's worstForward selection so
// the workspace-level codec override (if any) is applied.
func DetectCodecFromPayload(payload map[string]interface{}) string {
	if payload == nil {
		return "G.711"
	}
	if c, ok := payload["codec"].(string); ok && c != "" {
		return c
	}
	// Probe DSCP EF (46) implies VoIP; assume the default for now.
	if v, ok := payload["dscpValue"]; ok {
		switch v.(type) {
		case int, int32, int64, float64:
			return "G.711"
		}
	}
	return "G.711"
}
