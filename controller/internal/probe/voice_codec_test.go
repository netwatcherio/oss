package probe

import "testing"

// TestEffectiveThresholdsG711Baseline verifies the G.711 codec
// (the default) leaves thresholds unchanged.
func TestEffectiveThresholdsG711Baseline(t *testing.T) {
	base := VoiceDefaultThresholds
	out := EffectiveThresholds(base)
	if out.Codec != "G.711" {
		t.Errorf("codec should be G.711, got %q", out.Codec)
	}
	if out.WarningLossPct != base.WarningLossPct {
		t.Errorf("G.711 WarningLossPct should equal base %v, got %v",
			base.WarningLossPct, out.WarningLossPct)
	}
}

// TestEffectiveThresholdsOpusLenient verifies that Opus (the most
// tolerant codec) scales loss thresholds up by 2x.
func TestEffectiveThresholdsOpusLenient(t *testing.T) {
	base := VoiceDefaultThresholds
	base.Codec = "Opus"
	out := EffectiveThresholds(base)
	wantWarn := base.WarningLossPct * 2.0
	wantCrit := base.CriticalLossPct * 2.0
	if out.WarningLossPct != wantWarn {
		t.Errorf("Opus WarningLossPct = %v, want %v", out.WarningLossPct, wantWarn)
	}
	if out.CriticalLossPct != wantCrit {
		t.Errorf("Opus CriticalLossPct = %v, want %v", out.CriticalLossPct, wantCrit)
	}
}

// TestEffectiveThresholdsG729 verifies G.729's 1.5x loss multiplier.
func TestEffectiveThresholdsG729(t *testing.T) {
	base := VoiceDefaultThresholds
	base.Codec = "G.729"
	out := EffectiveThresholds(base)
	wantWarn := base.WarningLossPct * 1.5
	if out.WarningLossPct != wantWarn {
		t.Errorf("G.729 WarningLossPct = %v, want %v", out.WarningLossPct, wantWarn)
	}
}

// TestEffectiveThresholdsUnknownCodecFallsBack verifies that an
// unrecognized codec falls back to G.711 (1.0x multipliers).
func TestEffectiveThresholdsUnknownCodecFallsBack(t *testing.T) {
	base := VoiceDefaultThresholds
	base.Codec = "MysteryCodec"
	out := EffectiveThresholds(base)
	if out.WarningLossPct != base.WarningLossPct {
		t.Errorf("unknown codec should not scale; got %v vs base %v",
			out.WarningLossPct, base.WarningLossPct)
	}
}

// TestEffectiveThresholdsOverride verifies that a workspace/admin
// codec tolerance override wins over the built-in table.
func TestEffectiveThresholdsOverride(t *testing.T) {
	base := VoiceDefaultThresholds
	base.Codec = "G.711"
	base.CodecTolerances = map[string]CodecTolerance{
		"G.711": {
			LossMultiplier:   3.0,
			JitterMultiplier: 1.5,
		},
	}
	out := EffectiveThresholds(base)
	wantWarn := base.WarningLossPct * 3.0
	if out.WarningLossPct != wantWarn {
		t.Errorf("override WarningLossPct = %v, want %v", out.WarningLossPct, wantWarn)
	}
}

// TestEffectiveThresholdsEmptyCodecDefaults verifies that an empty
// codec name defaults to G.711.
func TestEffectiveThresholdsEmptyCodecDefaults(t *testing.T) {
	base := VoiceDefaultThresholds
	base.Codec = ""
	out := EffectiveThresholds(base)
	if out.Codec != "G.711" {
		t.Errorf("empty codec should default to G.711, got %q", out.Codec)
	}
}

// TestDetectCodecFromPayload verifies the payload sniffer's behavior
// when a `codec` field is present.
func TestDetectCodecFromPayload(t *testing.T) {
	if got := DetectCodecFromPayload(map[string]interface{}{"codec": "G.729"}); got != "G.729" {
		t.Errorf("expected G.729, got %q", got)
	}
	if got := DetectCodecFromPayload(nil); got != "G.711" {
		t.Errorf("nil payload should default to G.711, got %q", got)
	}
	if got := DetectCodecFromPayload(map[string]interface{}{"dscpValue": float64(46)}); got != "G.711" {
		t.Errorf("dscp only payload should default to G.711, got %q", got)
	}
}
