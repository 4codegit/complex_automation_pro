package simulator

import (
	"testing"

	"cap/internal/store"
)

func ptr(f float64) *float64 { return &f }

// TestEvalAlarmRationalisedPrecedence checks that a stored rationalised limit
// is authoritative over the demo envelope, that enabled=false shelves the
// alarm, and that multi-band thresholds return the most severe breach.
func TestEvalAlarmRationalisedPrecedence(t *testing.T) {
	m := &Manager{}
	params := &profileParams{} // empty profile: demo envelope is the fallback

	tag := "plant-a.crushing.particle_size"

	limits := map[string]*store.AlarmLimit{
		tag: {
			TagID:    tag,
			Enabled:  true,
			Hi:       ptr(6.0), // demo envelope: max=12 -> rationalised 6 wins
			Severity: "high",
		},
	}

	cases := []struct {
		name    string
		value   float64
		limits  map[string]*store.AlarmLimit
		want    bool
		wantSev string
	}{
		{"rationalised breach high", 6.5, limits, true, "high"},
		{"rationalised in-range", 5.5, limits, false, ""},
		{"rationalised disabled shelves alarm", 7.0,
			map[string]*store.AlarmLimit{tag: {TagID: tag, Enabled: false, Hi: ptr(6.0), Severity: "high"}},
			false, ""},
		{"hi_hi wins over hi", 100.0,
			map[string]*store.AlarmLimit{tag: {TagID: tag, Enabled: true, Hi: ptr(6.0), HiHi: ptr(8.0), Severity: "critical"}},
			true, "critical"},
		{"lo_lo wins over lo", -1.0,
			map[string]*store.AlarmLimit{tag: {TagID: tag, Enabled: true, Lo: ptr(2.0), LoLo: ptr(0.0), Severity: "critical"}},
			true, "critical"},
		{"fallback demo envelope out-of-range", 50.0, map[string]*store.AlarmLimit{}, true, "critical"},
		{"fallback demo envelope in-range", 5.5, map[string]*store.AlarmLimit{}, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			alert, sev, _ := m.evalAlarm(c.limits, params, tag, "particle_size", c.value)
			if alert != c.want {
				t.Errorf("alert = %v, want %v", alert, c.want)
			}
			if c.want && sev != c.wantSev {
				t.Errorf("severity = %q, want %q", sev, c.wantSev)
			}
		})
	}
}
