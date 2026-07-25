package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrConnectorNotFound = errors.New("connector not found")
	// ErrConnectorUnknownKind means the kind has no compile-time implementation.
	ErrConnectorUnknownKind = errors.New("unknown connector kind")
	// ErrConnectorSingleton means this kind already has its one allowed instance.
	ErrConnectorSingleton = errors.New("this connector kind allows only one instance")
	// ErrConnectorKindImmutable guards against reinterpreting a stored config
	// under a different connector.
	ErrConnectorKindImmutable = errors.New("connector kind cannot be changed")
	// ErrConnectorInvalid marks rejected admin input, as opposed to a storage
	// failure, so the handler can answer 400 rather than 500.
	ErrConnectorInvalid = errors.New("invalid connector")
)

const (
	maxConnectorNameLength = 100
	// testSendTimeout bounds the synchronous test-send so a black-holed endpoint
	// cannot hold the admin's request open.
	testSendTimeout = 10 * time.Second
)

// ConnectorService owns admin CRUD for notification connector instances,
// including the write-only handling of their secrets.
type ConnectorService struct {
	repo       repository.ConnectorRepository
	deliveries repository.ConnectorDeliveryRepository
	registry   *connector.Registry
	settings   *SiteSettingsService
	baseURL    string
}

// NewConnectorService creates a ConnectorService. baseURL is used to build the
// link in the test-send sample announcement.
func NewConnectorService(
	repo repository.ConnectorRepository,
	deliveries repository.ConnectorDeliveryRepository,
	registry *connector.Registry,
	settings *SiteSettingsService,
	baseURL string,
) *ConnectorService {
	return &ConnectorService{
		repo:       repo,
		deliveries: deliveries,
		registry:   registry,
		settings:   settings,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

// ConnectorInput is an admin create/update submission.
type ConnectorInput struct {
	Kind    string          `json:"kind"`
	Name    string          `json:"name"`
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config"`
	Filters json.RawMessage `json:"filters"`
}

// ConnectorView is the API shape of an instance. Config is the redacted form:
// secret keys are removed entirely and only named in SecretsSet, so a
// credential never travels back to the browser.
type ConnectorView struct {
	ID           int64           `json:"id"`
	Kind         string          `json:"kind"`
	Name         string          `json:"name"`
	Enabled      bool            `json:"enabled"`
	Singleton    bool            `json:"singleton"`
	Config       json.RawMessage `json:"config"`
	Filters      json.RawMessage `json:"filters"`
	SecretsSet   []string        `json:"secrets_set"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	LastDelivery *DeliveryView   `json:"last_delivery,omitempty"`
}

// DeliveryView is the API shape of one delivery-log row.
type DeliveryView struct {
	ID            int64           `json:"id"`
	InstanceID    int64           `json:"instance_id"`
	EventKey      string          `json:"event_key"`
	EventType     string          `json:"event_type"`
	Status        string          `json:"status"`
	Attempts      int             `json:"attempts"`
	LastError     string          `json:"last_error,omitempty"`
	NextAttemptAt *time.Time      `json:"next_attempt_at,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// Kinds lists the registered connector kinds for the admin UI's kind picker.
func (s *ConnectorService) Kinds() []string {
	return s.registry.Kinds()
}

// Create validates and stores a new instance.
func (s *ConnectorService) Create(ctx context.Context, in ConnectorInput) (*ConnectorView, error) {
	impl, ok := s.registry.Get(in.Kind)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrConnectorUnknownKind, in.Kind)
	}

	name, err := validateConnectorName(in.Name)
	if err != nil {
		return nil, err
	}

	if impl.Singleton() {
		count, err := s.repo.CountByKind(ctx, in.Kind)
		if err != nil {
			return nil, fmt.Errorf("count connectors by kind: %w", err)
		}
		if count > 0 {
			return nil, fmt.Errorf("%w: %s", ErrConnectorSingleton, in.Kind)
		}
	}

	// Merging against an empty stored config normalises the input the same way
	// an update does: a secret submitted as "" is treated as unset rather than
	// stored as a real empty credential.
	cfg, filters, err := s.prepare(json.RawMessage("{}"), in, impl)
	if err != nil {
		return nil, err
	}

	c := &model.NotificationConnector{
		Kind:    in.Kind,
		Name:    name,
		Enabled: in.Enabled,
		Config:  cfg,
		Filters: filters,
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, fmt.Errorf("create connector: %w", err)
	}

	return s.view(c, nil), nil
}

// Update applies an admin edit. The kind is immutable and secrets follow the
// write-only merge rules (see connector.MergeSecrets).
func (s *ConnectorService) Update(ctx context.Context, id int64, in ConnectorInput) (*ConnectorView, error) {
	existing, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Kind != "" && in.Kind != existing.Kind {
		return nil, fmt.Errorf("%w: %s", ErrConnectorKindImmutable, existing.Kind)
	}
	impl, ok := s.registry.Get(existing.Kind)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrConnectorUnknownKind, existing.Kind)
	}

	name, err := validateConnectorName(in.Name)
	if err != nil {
		return nil, err
	}

	cfg, filters, err := s.prepare(existing.Config, in, impl)
	if err != nil {
		return nil, err
	}

	existing.Name = name
	existing.Enabled = in.Enabled
	existing.Config = cfg
	existing.Filters = filters

	if err := s.repo.Update(ctx, existing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConnectorNotFound
		}
		return nil, fmt.Errorf("update connector: %w", err)
	}

	return s.view(existing, nil), nil
}

// prepare merges secrets onto the stored config and validates the result. Every
// validation runs on the merged config, never on the raw submission — otherwise
// a blank "leave the secret alone" field would look like a missing credential.
func (s *ConnectorService) prepare(storedConfig json.RawMessage, in ConnectorInput, impl connector.Connector) (json.RawMessage, json.RawMessage, error) {
	merged, err := connector.MergeSecrets(storedConfig, in.Config, impl.SecretFields())
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrConnectorInvalid, err)
	}
	if err := impl.ValidateConfig(merged); err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrConnectorInvalid, err)
	}

	filters := in.Filters
	if len(filters) == 0 {
		filters = json.RawMessage("{}")
	}
	if _, err := connector.ParseFilters(filters); err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrConnectorInvalid, err)
	}

	return merged, filters, nil
}

// Get returns one instance.
func (s *ConnectorService) Get(ctx context.Context, id int64) (*ConnectorView, error) {
	c, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.view(c, nil), nil
}

// Delete removes an instance. Its delivery rows go with it (ON DELETE CASCADE).
func (s *ConnectorService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConnectorNotFound
		}
		return fmt.Errorf("delete connector: %w", err)
	}
	return nil
}

// List returns every instance, each carrying its most recent delivery so the
// admin table can show a status chip without an extra request per row.
func (s *ConnectorService) List(ctx context.Context) ([]ConnectorView, error) {
	connectors, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list connectors: %w", err)
	}

	latest, err := s.deliveries.LatestStatusByInstance(ctx)
	if err != nil {
		// A broken delivery log should not hide the instances themselves — the
		// admin still needs to be able to fix or disable them.
		latest = nil
	}

	views := make([]ConnectorView, 0, len(connectors))
	for i := range connectors {
		c := connectors[i]
		var last *model.ConnectorDelivery
		if d, ok := latest[c.ID]; ok {
			last = &d
		}
		views = append(views, *s.view(&c, last))
	}
	return views, nil
}

// Deliveries returns a page of an instance's delivery log.
func (s *ConnectorService) Deliveries(ctx context.Context, id int64, page, perPage int) ([]DeliveryView, int64, error) {
	if _, err := s.get(ctx, id); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.deliveries.ListByInstance(ctx, id, page, perPage)
	if err != nil {
		return nil, 0, fmt.Errorf("list connector deliveries: %w", err)
	}
	views := make([]DeliveryView, 0, len(rows))
	for i := range rows {
		views = append(views, deliveryView(&rows[i]))
	}
	return views, total, nil
}

// TestSend delivers a canned announcement synchronously so the admin gets an
// immediate answer. It deliberately bypasses the instance's filters, the global
// kill-switch and the queue — the question being asked is "can this destination
// be reached with these credentials", not "would a real event go here". Disabled
// instances can be tested too, which is what makes it useful for setting one up
// before turning it on.
//
// It still goes through the SSRF guard, and it still records a delivery row so
// the log stays a complete picture of what the connector has done.
func (s *ConnectorService) TestSend(ctx context.Context, id int64) (*DeliveryView, error) {
	c, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	impl, ok := s.registry.Get(c.Kind)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrConnectorUnknownKind, c.Kind)
	}

	announcement := connector.Sample()
	if s.baseURL != "" {
		announcement.URL = s.baseURL + "/torrent/0"
		announcement.Body, _ = connector.RenderTemplate(connector.DefaultTemplate, announcement)
	}
	// A fresh key per test means repeated tests never collide with each other or
	// with a real event's dedupe row.
	announcement.DeliveryKey = "test:" + uuid.NewString()

	payload, err := json.Marshal(announcement)
	if err != nil {
		return nil, fmt.Errorf("marshal test announcement: %w", err)
	}

	row := &model.ConnectorDelivery{
		InstanceID: c.ID,
		EventKey:   announcement.DeliveryKey,
		EventType:  connector.EventTest,
		Payload:    payload,
		Status:     model.DeliveryPending,
	}
	if _, err := s.deliveries.InsertPending(ctx, row); err != nil {
		return nil, fmt.Errorf("record test delivery: %w", err)
	}

	deliverCtx, cancel := context.WithTimeout(ctx, testSendTimeout)
	defer cancel()

	inst := connector.Instance{ID: c.ID, Name: c.Name, Config: c.Config}
	deliverErr := connector.SafeDeliver(deliverCtx, impl, inst, announcement)

	if deliverErr == nil {
		if err := s.deliveries.MarkSent(ctx, row.ID, model.DeliverySent); err != nil {
			return nil, fmt.Errorf("record test delivery result: %w", err)
		}
		row.Status = model.DeliverySent
		view := deliveryView(row)
		return &view, nil
	}

	message := connector.RedactError(deliverErr, c.Config, impl.SecretFields())
	if err := s.deliveries.MarkFailedAttempt(ctx, row.ID, 1, message, nil); err != nil {
		return nil, fmt.Errorf("record test delivery result: %w", err)
	}
	row.Status = model.DeliveryFailed
	row.Attempts = 1
	row.LastError = &message
	view := deliveryView(row)
	return &view, nil
}

func (s *ConnectorService) get(ctx context.Context, id int64) (*model.NotificationConnector, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConnectorNotFound
		}
		return nil, fmt.Errorf("get connector: %w", err)
	}
	if c == nil {
		return nil, ErrConnectorNotFound
	}
	return c, nil
}

// view builds the API shape, redacting the config against whatever the
// registered connector declares as secret. A kind that is no longer registered
// (an instance left behind by a downgrade) is redacted as if everything in it
// were secret, which is the only safe assumption.
func (s *ConnectorService) view(c *model.NotificationConnector, last *model.ConnectorDelivery) *ConnectorView {
	var (
		secretFields []string
		singleton    bool
	)
	impl, registered := s.registry.Get(c.Kind)
	if registered {
		secretFields = impl.SecretFields()
		singleton = impl.Singleton()
	}

	var (
		redacted   json.RawMessage
		secretsSet []string
	)
	if registered {
		redacted, secretsSet = connector.RedactConfig(c.Config, secretFields)
	} else {
		redacted = json.RawMessage("{}")
	}

	filters := c.Filters
	if len(filters) == 0 {
		filters = json.RawMessage("{}")
	}

	view := &ConnectorView{
		ID:         c.ID,
		Kind:       c.Kind,
		Name:       c.Name,
		Enabled:    c.Enabled,
		Singleton:  singleton,
		Config:     redacted,
		Filters:    filters,
		SecretsSet: secretsSet,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	}
	if view.SecretsSet == nil {
		view.SecretsSet = []string{}
	}
	if last != nil {
		d := deliveryView(last)
		view.LastDelivery = &d
	}
	return view
}

func deliveryView(d *model.ConnectorDelivery) DeliveryView {
	view := DeliveryView{
		ID:            d.ID,
		InstanceID:    d.InstanceID,
		EventKey:      d.EventKey,
		EventType:     d.EventType,
		Status:        d.Status,
		Attempts:      d.Attempts,
		NextAttemptAt: d.NextAttemptAt,
		Payload:       d.Payload,
		CreatedAt:     d.CreatedAt,
		UpdatedAt:     d.UpdatedAt,
	}
	if d.LastError != nil {
		view.LastError = *d.LastError
	}
	return view
}

func validateConnectorName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("%w: name is required", ErrConnectorInvalid)
	}
	if len(trimmed) > maxConnectorNameLength {
		return "", fmt.Errorf("%w: name cannot exceed %d characters", ErrConnectorInvalid, maxConnectorNameLength)
	}
	return trimmed, nil
}
