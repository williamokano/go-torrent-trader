package worker

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// settingsRepoStub is the minimum SiteSettingsRepository the setting helpers
// read through. Only GetAll carries behaviour.
type settingsRepoStub struct {
	values map[string]string
	err    error
}

func (s *settingsRepoStub) Get(_ context.Context, key string) (*model.SiteSetting, error) {
	if v, ok := s.values[key]; ok {
		return &model.SiteSetting{Key: key, Value: v}, nil
	}
	return nil, nil
}
func (s *settingsRepoStub) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}
func (s *settingsRepoStub) GetAll(_ context.Context) ([]model.SiteSetting, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([]model.SiteSetting, 0, len(s.values))
	for k, v := range s.values {
		out = append(out, model.SiteSetting{Key: k, Value: v})
	}
	return out, nil
}

func newSettingsService(values map[string]string) *service.SiteSettingsService {
	return service.NewSiteSettingsService(&settingsRepoStub{values: values}, event.NewInMemoryBus())
}

// These settings drive whether a user gets warned or banned, so a parse slip
// must fall back to the safe default rather than to a zero that would, say,
// treat every ratio as below a threshold of 0.

func TestGetFloatSetting(t *testing.T) {
	ctx := context.Background()

	t.Run("stored value is parsed", func(t *testing.T) {
		svc := newSettingsService(map[string]string{"ratio_warning_threshold": "0.75"})
		if got := getFloatSetting(ctx, svc, "ratio_warning_threshold", 0.3); got != 0.75 {
			t.Errorf("got %v, want 0.75", got)
		}
	})

	t.Run("missing key falls back to default", func(t *testing.T) {
		svc := newSettingsService(map[string]string{})
		if got := getFloatSetting(ctx, svc, "ratio_warning_threshold", 0.3); got != 0.3 {
			t.Errorf("got %v, want the default 0.3", got)
		}
	})

	t.Run("unparseable value falls back to default", func(t *testing.T) {
		svc := newSettingsService(map[string]string{"ratio_warning_threshold": "not-a-float"})
		if got := getFloatSetting(ctx, svc, "ratio_warning_threshold", 0.3); got != 0.3 {
			t.Errorf("got %v, want the default 0.3 for a bad value", got)
		}
	})
}

func TestGetIntSetting(t *testing.T) {
	ctx := context.Background()

	t.Run("stored value is parsed", func(t *testing.T) {
		svc := newSettingsService(map[string]string{"ratio_warn_days": "10"})
		if got := getIntSetting(ctx, svc, "ratio_warn_days", 7); got != 10 {
			t.Errorf("got %d, want 10", got)
		}
	})

	t.Run("missing key falls back to default", func(t *testing.T) {
		svc := newSettingsService(map[string]string{})
		if got := getIntSetting(ctx, svc, "ratio_warn_days", 7); got != 7 {
			t.Errorf("got %d, want the default 7", got)
		}
	})

	t.Run("unparseable value falls back to default", func(t *testing.T) {
		svc := newSettingsService(map[string]string{"ratio_warn_days": "soon"})
		if got := getIntSetting(ctx, svc, "ratio_warn_days", 7); got != 7 {
			t.Errorf("got %d, want the default 7 for a bad value", got)
		}
	})
}

func TestGetStringSetting(t *testing.T) {
	ctx := context.Background()

	t.Run("stored value is returned", func(t *testing.T) {
		svc := newSettingsService(map[string]string{"ratio_warning_message": "improve or else"})
		if got := getStringSetting(ctx, svc, "ratio_warning_message", "default"); got != "improve or else" {
			t.Errorf("got %q, want the stored message", got)
		}
	})

	t.Run("missing key falls back to default", func(t *testing.T) {
		svc := newSettingsService(map[string]string{})
		if got := getStringSetting(ctx, svc, "ratio_warning_message", "default"); got != "default" {
			t.Errorf("got %q, want the default", got)
		}
	})
}

// The handler must no-op safely when its dependencies are absent, rather than
// panicking a background worker.
func TestRatioWarningHandlerSkipsWhenDepsMissing(t *testing.T) {
	handler := NewRatioWarningHandler(&WorkerDeps{})
	if err := handler(context.Background(), nil); err != nil {
		t.Errorf("handler with no deps returned %v, want a clean skip", err)
	}
}

// The task descriptor must be constructible.
func TestNewRatioWarningTask(t *testing.T) {
	task, err := NewRatioWarningTask()
	if err != nil {
		t.Fatalf("NewRatioWarningTask: %v", err)
	}
	if task.Type() != TaskRatioWarning {
		t.Errorf("task type = %q, want %q", task.Type(), TaskRatioWarning)
	}
}
