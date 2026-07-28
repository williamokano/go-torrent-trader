package service

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
)

// settingsFrom builds a settings service holding exactly the given rows, so each
// case states only what it is about and everything else falls to its default.
func settingsFrom(t *testing.T, values map[string]string) *SiteSettingsService {
	t.Helper()
	repo := newMockSiteSettingsRepo()
	for k, v := range values {
		if err := repo.Set(context.Background(), k, v); err != nil {
			t.Fatalf("seeding %s: %v", k, err)
		}
	}
	return NewSiteSettingsService(repo, event.NewInMemoryBus())
}

// The table is written from the operator's point of view — what they set, what
// else is on, and what the site should then actually do — because that is the
// question every caller of this is really asking.
func TestResolveAnnounceRetention(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]string
		// All three are asserted, not just the effective window. ConfiguredDays is
		// what cmd/server feeds the prune and what the admin note quotes back as
		// "not N"; FloorDays is what the prune clamps to. A mutation that set
		// ConfiguredDays = FloorDays inside the override branch passed the whole
		// suite when only EffectiveDays was checked — it would have shipped the
		// operator "kept for 31 days, not 31" and corrupted the prune's input, in
		// a package excluded from the coverage denominator.
		wantConfigured int
		wantFloor      int
		wantEffective  int
		wantOverride   bool
	}{
		{
			name:           "the default with promotion off is simply the default",
			settings:       map[string]string{},
			wantConfigured: DefaultAnnounceLogRetentionDays,
			wantFloor:      0,
			wantEffective:  DefaultAnnounceLogRetentionDays,
		},
		{
			name: "a short window with promotion off is honoured exactly",
			settings: map[string]string{
				SettingAnnounceLogRetentionDays: "7",
				SettingPromotionEnabled:         "false",
			},
			wantConfigured: 7,
			wantFloor:      0,
			wantEffective:  7,
		},
		{
			// The case #255 is about. An operator sets 7; promotion's default 30-day
			// seeding window plus a day of headroom holds the log open at 31.
			name: "a window shorter than promotion needs is raised, and says so",
			settings: map[string]string{
				SettingAnnounceLogRetentionDays: "7",
				SettingPromotionEnabled:         "true",
			},
			wantConfigured: 7,
			wantFloor:      31,
			wantEffective:  31,
			wantOverride:   true,
		},
		{
			name: "a window longer than the floor is untouched",
			settings: map[string]string{
				SettingAnnounceLogRetentionDays: "90",
				SettingPromotionEnabled:         "true",
			},
			wantConfigured: 90,
			wantFloor:      31,
			wantEffective:  90,
		},
		{
			// Exactly at the floor is not an override: the operator gets what they
			// asked for, and telling them otherwise would be noise.
			name: "a window exactly at the floor is not reported as overridden",
			settings: map[string]string{
				SettingAnnounceLogRetentionDays: "31",
				SettingPromotionEnabled:         "true",
			},
			wantConfigured: 31,
			wantFloor:      31,
			wantEffective:  31,
		},
		{
			// Keeping everything forever cannot breach a floor, so there is nothing
			// to report — and an operator who chose this does not need telling that
			// promotion would have liked 31 days.
			name: "pruning disabled stays disabled even with a floor in play",
			settings: map[string]string{
				SettingAnnounceLogRetentionDays: "0",
				SettingPromotionEnabled:         "true",
			},
			wantConfigured: 0,
			wantFloor:      31,
			wantEffective:  0,
		},
		{
			// A negative value is a misconfiguration, not a window in the past. The
			// prune treats non-positive as disabled; every surface must agree.
			name: "a negative window reads as disabled, not as a cutoff in the future",
			settings: map[string]string{
				SettingAnnounceLogRetentionDays: "-5",
				SettingPromotionEnabled:         "true",
			},
			wantConfigured: 0,
			wantFloor:      31,
			wantEffective:  0,
		},
		{
			// Asserting FloorDays is what gives the days < 1 guard a job. Without
			// it a negative seeding window yields a negative floor, which never
			// wins the max() and so leaves EffectiveDays right — the case passed
			// either way until the floor itself was asserted.
			name: "a zero seeding window creates no floor",
			settings: map[string]string{
				SettingAnnounceLogRetentionDays: "7",
				SettingPromotionEnabled:         "true",
				SettingPromotionSeedWindowDays:  "0",
			},
			wantConfigured: 7,
			wantFloor:      0,
			wantEffective:  7,
		},
		{
			name: "a negative seeding window reports no floor, not a negative one",
			settings: map[string]string{
				SettingAnnounceLogRetentionDays: "7",
				SettingPromotionEnabled:         "true",
				SettingPromotionSeedWindowDays:  "-30",
			},
			wantConfigured: 7,
			wantFloor:      0,
			wantEffective:  7,
		},
		{
			name: "a longer seeding window raises the floor with it",
			settings: map[string]string{
				SettingAnnounceLogRetentionDays: "30",
				SettingPromotionEnabled:         "true",
				SettingPromotionSeedWindowDays:  "60",
			},
			wantConfigured: 30,
			wantFloor:      61,
			wantEffective:  61,
			wantOverride:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveAnnounceRetention(context.Background(), settingsFrom(t, tc.settings))

			if got.ConfiguredDays != tc.wantConfigured {
				t.Errorf("ConfiguredDays = %d, want %d", got.ConfiguredDays, tc.wantConfigured)
			}
			if got.FloorDays != tc.wantFloor {
				t.Errorf("FloorDays = %d, want %d", got.FloorDays, tc.wantFloor)
			}
			if got.EffectiveDays != tc.wantEffective {
				t.Errorf("EffectiveDays = %d, want %d", got.EffectiveDays, tc.wantEffective)
			}
			if got.Overridden() != tc.wantOverride {
				t.Errorf("Overridden() = %v, want %v", got.Overridden(), tc.wantOverride)
			}
			if tc.wantOverride && got.FloorReason == "" {
				t.Error("reported an override with no reason; the operator cannot act on that")
			}
			if !tc.wantOverride && got.FloorReason != "" {
				t.Errorf("reported reason %q without an override", got.FloorReason)
			}
		})
	}
}

// Nothing wired must not mean "no retention". A handler constructed without
// settings answering 0 would tell every member their announces are kept forever.
func TestResolveAnnounceRetentionWithoutSettings(t *testing.T) {
	got := ResolveAnnounceRetention(context.Background(), nil)
	if got.EffectiveDays != DefaultAnnounceLogRetentionDays {
		t.Errorf("EffectiveDays = %d with no settings, want the default %d",
			got.EffectiveDays, DefaultAnnounceLogRetentionDays)
	}
	if got.Overridden() {
		t.Error("Overridden() = true with no settings")
	}
}

// promotion_seed_window_days sets the floor under retention, so it is now
// reported to operators as the effective window and to members as how long their
// announces are kept. Past the int64 overflow in days→duration the prune reads
// the wrapped value as "no floor" and keeps the short window, while both
// surfaces report the enormous number — the setting lying in two places rather
// than one. maxAnnounceLogRetentionDays exists for exactly this on the sibling
// setting; this pins that the same bound now applies here.
func TestSetRejectsASeedWindowThatWouldOverflowTheDuration(t *testing.T) {
	svc := settingsFrom(t, nil)
	ctx := context.Background()

	for _, value := range []string{"106752", "1000000000"} {
		if err := svc.Set(ctx, SettingPromotionSeedWindowDays, value, event.Actor{}); err == nil {
			t.Errorf("Set(%s) was accepted; days*24h overflows int64 and the prune would silently ignore the floor", value)
		}
	}
	if err := svc.Set(ctx, SettingPromotionSeedWindowDays, "-1", event.Actor{}); err == nil {
		t.Error("Set(-1) was accepted; a negative seeding window is meaningless")
	}
	// And a sane value still works, so the bound is not simply refusing everything.
	if err := svc.Set(ctx, SettingPromotionSeedWindowDays, "60", event.Actor{}); err != nil {
		t.Errorf("Set(60) was rejected: %v", err)
	}
}
