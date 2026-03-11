package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- mock implementations ---

type mockCheatFlagRepo struct {
	mu      sync.Mutex
	flags   []model.CheatFlag
	nextID  int64
	recent  map[string]bool // key: "userID:flagType"
	createErr error
	recentErr error
}

func newMockCheatFlagRepo() *mockCheatFlagRepo {
	return &mockCheatFlagRepo{
		nextID: 1,
		recent: make(map[string]bool),
	}
}

func (m *mockCheatFlagRepo) Create(_ context.Context, flag *model.CheatFlag) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	flag.ID = m.nextID
	m.nextID++
	flag.CreatedAt = time.Now()
	m.flags = append(m.flags, *flag)
	return nil
}

func (m *mockCheatFlagRepo) GetByID(_ context.Context, id int64) (*model.CheatFlag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range m.flags {
		if f.ID == id {
			return &f, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockCheatFlagRepo) List(_ context.Context, _ repository.ListCheatFlagsOptions) ([]model.CheatFlag, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.flags, int64(len(m.flags)), nil
}

func (m *mockCheatFlagRepo) Dismiss(_ context.Context, _, _ int64) error {
	return nil
}

func (m *mockCheatFlagRepo) HasRecentUndismissed(_ context.Context, userID int64, torrentID int64, flagType string, _ int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recentErr != nil {
		return false, m.recentErr
	}
	key := fmt.Sprintf("%d:%d:%s", userID, torrentID, flagType)
	return m.recent[key], nil
}

func (m *mockCheatFlagRepo) getFlags() []model.CheatFlag {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]model.CheatFlag, len(m.flags))
	copy(result, m.flags)
	return result
}

func (m *mockCheatFlagRepo) setRecent(userID int64, torrentID int64, flagType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d:%d:%s", userID, torrentID, flagType)
	m.recent[key] = true
}

func newTestCheatDetectionService(flagRepo *mockCheatFlagRepo, settings map[string]string) *CheatDetectionService {
	settingsRepo := newMockSiteSettingsRepo()
	for k, v := range settings {
		settingsRepo.settings[k] = &model.SiteSetting{Key: k, Value: v}
	}
	siteSettings := NewSiteSettingsService(settingsRepo, event.NewInMemoryBus())
	return NewCheatDetectionService(flagRepo, siteSettings, event.NewInMemoryBus())
}

func baseInput() AnnounceCheckInput {
	return AnnounceCheckInput{
		UserID:      1,
		Username:    "testuser",
		TorrentID:   100,
		TorrentName: "Test Torrent",
		Leechers:    5,
		Now:         time.Now(),
	}
}

// --- tests ---

func TestCheckAnnounce_DisabledDetection(t *testing.T) {
	repo := newMockCheatFlagRepo()
	svc := newTestCheatDetectionService(repo, map[string]string{
		"cheat_detection_enabled": "false",
	})

	input := baseInput()
	input.ExistingPeer = &model.Peer{
		Seeder:       true,
		LastAnnounce: time.Now().Add(-10 * time.Minute),
		Uploaded:     0,
	}
	input.UploadDelta = 500 * 1024 * 1024 * 1024 // 500GB — clearly impossible
	input.Leechers = 0

	// CheckAnnounce returns synchronously when detection is disabled (before spawning goroutine).
	svc.CheckAnnounce(context.Background(), input)
	// Small sleep to ensure no goroutine was spawned.
	time.Sleep(10 * time.Millisecond)

	if len(repo.getFlags()) != 0 {
		t.Fatal("expected no flags when detection is disabled")
	}
}

func TestCheckImpossibleUploadSpeed(t *testing.T) {
	tests := []struct {
		name        string
		uploadDelta int64
		timeDelta   time.Duration
		maxSpeedMB  string
		wantFlag    bool
	}{
		{
			name:        "speed exceeds threshold",
			uploadDelta: 200 * 1024 * 1024, // 200MB
			timeDelta:   1 * time.Minute,    // = ~3.3 MB/s over 60s, but 200MB/60s = 3.3 MB/s... need higher delta
			maxSpeedMB:  "1",               // set low threshold to trigger
			wantFlag:    true,
		},
		{
			name:        "speed within threshold",
			uploadDelta: 50 * 1024 * 1024, // 50MB
			timeDelta:   1 * time.Minute,   // ~0.83 MB/s
			maxSpeedMB:  "100",
			wantFlag:    false,
		},
		{
			name:        "time delta too short (< 30s)",
			uploadDelta: 200 * 1024 * 1024,
			timeDelta:   10 * time.Second,
			maxSpeedMB:  "100",
			wantFlag:    false,
		},
		{
			name:        "no existing peer",
			uploadDelta: 200 * 1024 * 1024,
			timeDelta:   0, // no existing peer
			maxSpeedMB:  "100",
			wantFlag:    false,
		},
		{
			name:        "zero upload delta",
			uploadDelta: 0,
			timeDelta:   5 * time.Minute,
			maxSpeedMB:  "100",
			wantFlag:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockCheatFlagRepo()
			svc := newTestCheatDetectionService(repo, map[string]string{
				"cheat_detection_enabled":   "true",
				"cheat_max_upload_speed_mb_s": tt.maxSpeedMB,
			})

			input := baseInput()
			if tt.timeDelta > 0 {
				input.ExistingPeer = &model.Peer{
					LastAnnounce: input.Now.Add(-tt.timeDelta),
					Uploaded:     0,
				}
			}
			input.UploadDelta = tt.uploadDelta

			svc.runChecks(context.Background(), input)

			flags := repo.getFlags()
			hasFlag := false
			for _, f := range flags {
				if f.FlagType == model.CheatFlagImpossibleUploadSpeed {
					hasFlag = true
				}
			}

			if hasFlag != tt.wantFlag {
				t.Errorf("got flag=%v, want flag=%v", hasFlag, tt.wantFlag)
			}
		})
	}
}

func TestCheckUploadNoDownloaders(t *testing.T) {
	tests := []struct {
		name        string
		isSeeder    bool
		uploadDelta int64
		leechers    int
		wantFlag    bool
	}{
		{
			name:        "seeder uploads with no leechers",
			isSeeder:    true,
			uploadDelta: 2 * 1024 * 1024, // 2MB
			leechers:    0,
			wantFlag:    true,
		},
		{
			name:        "seeder uploads with leechers present",
			isSeeder:    true,
			uploadDelta: 2 * 1024 * 1024,
			leechers:    3,
			wantFlag:    false,
		},
		{
			name:        "leecher (not seeder) — skip",
			isSeeder:    false,
			uploadDelta: 2 * 1024 * 1024,
			leechers:    0,
			wantFlag:    false,
		},
		{
			name:        "upload below 1MB threshold",
			isSeeder:    true,
			uploadDelta: 500 * 1024, // 500KB
			leechers:    0,
			wantFlag:    false,
		},
		{
			name:        "no existing peer",
			isSeeder:    true,
			uploadDelta: 2 * 1024 * 1024,
			leechers:    0,
			wantFlag:    false, // existingPeer is nil
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockCheatFlagRepo()
			svc := newTestCheatDetectionService(repo, map[string]string{
				"cheat_detection_enabled": "true",
			})

			input := baseInput()
			input.UploadDelta = tt.uploadDelta
			input.Leechers = tt.leechers

			if tt.name != "no existing peer" {
				input.ExistingPeer = &model.Peer{
					Seeder:       tt.isSeeder,
					LastAnnounce: input.Now.Add(-10 * time.Minute),
				}
			}

			svc.runChecks(context.Background(), input)

			flags := repo.getFlags()
			hasFlag := false
			for _, f := range flags {
				if f.FlagType == model.CheatFlagUploadNoDownloaders {
					hasFlag = true
				}
			}

			if hasFlag != tt.wantFlag {
				t.Errorf("got flag=%v, want flag=%v", hasFlag, tt.wantFlag)
			}
		})
	}
}

func TestCheckLeftMismatch(t *testing.T) {
	tests := []struct {
		name          string
		downloadDelta int64
		prevLeft      int64
		curLeft       int64
		tolerancePct  string
		wantFlag      bool
	}{
		{
			name:          "left doesn't decrease enough",
			downloadDelta: 10 * 1024 * 1024, // 10MB downloaded
			prevLeft:      100 * 1024 * 1024,
			curLeft:       99 * 1024 * 1024, // only 1MB decrease
			tolerancePct:  "10",
			wantFlag:      true,
		},
		{
			name:          "left decreases proportionally",
			downloadDelta: 10 * 1024 * 1024,
			prevLeft:      100 * 1024 * 1024,
			curLeft:       90 * 1024 * 1024, // 10MB decrease
			tolerancePct:  "10",
			wantFlag:      false,
		},
		{
			name:          "download complete (left=0) — skip",
			downloadDelta: 10 * 1024 * 1024,
			prevLeft:      10 * 1024 * 1024,
			curLeft:       0,
			tolerancePct:  "10",
			wantFlag:      false,
		},
		{
			name:          "client reset (left went up) — skip",
			downloadDelta: 10 * 1024 * 1024,
			prevLeft:      50 * 1024 * 1024,
			curLeft:       100 * 1024 * 1024,
			tolerancePct:  "10",
			wantFlag:      false,
		},
		{
			name:          "download below 1MB threshold",
			downloadDelta: 500 * 1024,
			prevLeft:      100 * 1024 * 1024,
			curLeft:       100 * 1024 * 1024,
			tolerancePct:  "10",
			wantFlag:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockCheatFlagRepo()
			svc := newTestCheatDetectionService(repo, map[string]string{
				"cheat_detection_enabled":           "true",
				"cheat_left_mismatch_tolerance_pct": tt.tolerancePct,
			})

			input := baseInput()
			input.DownloadDelta = tt.downloadDelta
			input.ReqLeft = tt.curLeft
			input.ExistingPeer = &model.Peer{
				LeftBytes:    tt.prevLeft,
				LastAnnounce: input.Now.Add(-10 * time.Minute),
			}

			svc.runChecks(context.Background(), input)

			flags := repo.getFlags()
			hasFlag := false
			for _, f := range flags {
				if f.FlagType == model.CheatFlagLeftMismatch {
					hasFlag = true
				}
			}

			if hasFlag != tt.wantFlag {
				t.Errorf("got flag=%v, want flag=%v", hasFlag, tt.wantFlag)
			}
		})
	}
}

func TestCooldownPreventsFlag(t *testing.T) {
	repo := newMockCheatFlagRepo()
	repo.setRecent(1, 100, model.CheatFlagUploadNoDownloaders)

	svc := newTestCheatDetectionService(repo, map[string]string{
		"cheat_detection_enabled":    "true",
		"cheat_flag_cooldown_hours": "6",
	})

	input := baseInput()
	input.ExistingPeer = &model.Peer{
		Seeder:       true,
		LastAnnounce: input.Now.Add(-10 * time.Minute),
	}
	input.UploadDelta = 5 * 1024 * 1024
	input.Leechers = 0

	svc.runChecks(context.Background(), input)

	flags := repo.getFlags()
	for _, f := range flags {
		if f.FlagType == model.CheatFlagUploadNoDownloaders {
			t.Fatal("expected no upload_no_downloaders flag due to cooldown")
		}
	}
}

func TestCreateFlagStoresDetails(t *testing.T) {
	repo := newMockCheatFlagRepo()
	svc := newTestCheatDetectionService(repo, map[string]string{
		"cheat_detection_enabled": "true",
	})

	input := baseInput()
	input.ExistingPeer = &model.Peer{
		Seeder:       true,
		LastAnnounce: input.Now.Add(-10 * time.Minute),
	}
	input.UploadDelta = 5 * 1024 * 1024
	input.Leechers = 0

	svc.runChecks(context.Background(), input)

	flags := repo.getFlags()
	if len(flags) == 0 {
		t.Fatal("expected at least one flag")
	}

	flag := flags[0]
	if flag.FlagType != model.CheatFlagUploadNoDownloaders {
		t.Fatalf("expected flag type %s, got %s", model.CheatFlagUploadNoDownloaders, flag.FlagType)
	}
	if flag.UserID != 1 {
		t.Fatalf("expected user_id 1, got %d", flag.UserID)
	}

	var details map[string]interface{}
	if err := json.Unmarshal([]byte(flag.Details), &details); err != nil {
		t.Fatalf("failed to unmarshal details: %v", err)
	}
	if _, ok := details["upload_delta_bytes"]; !ok {
		t.Fatal("expected upload_delta_bytes in details")
	}
}

func TestCreateFlagError_DoesNotPanic(t *testing.T) {
	repo := newMockCheatFlagRepo()
	repo.createErr = fmt.Errorf("db connection lost")

	svc := newTestCheatDetectionService(repo, map[string]string{
		"cheat_detection_enabled": "true",
	})

	input := baseInput()
	input.ExistingPeer = &model.Peer{
		Seeder:       true,
		LastAnnounce: input.Now.Add(-10 * time.Minute),
	}
	input.UploadDelta = 5 * 1024 * 1024
	input.Leechers = 0

	// Should not panic — errors are logged, not propagated
	svc.runChecks(context.Background(), input)

	if len(repo.getFlags()) != 0 {
		t.Fatal("expected no flags when create fails")
	}
}

func TestCooldownCheckError_DoesNotPanic(t *testing.T) {
	repo := newMockCheatFlagRepo()
	repo.recentErr = fmt.Errorf("db timeout")

	svc := newTestCheatDetectionService(repo, map[string]string{
		"cheat_detection_enabled": "true",
	})

	input := baseInput()
	input.ExistingPeer = &model.Peer{
		Seeder:       true,
		LastAnnounce: input.Now.Add(-10 * time.Minute),
	}
	input.UploadDelta = 5 * 1024 * 1024
	input.Leechers = 0

	// Should not panic — errors are logged, not propagated
	svc.runChecks(context.Background(), input)

	if len(repo.getFlags()) != 0 {
		t.Fatal("expected no flags when cooldown check fails")
	}
}
