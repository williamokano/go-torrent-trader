package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/zeebo/bencode"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- mock storage ---

type memStorage struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newMemStorage() *memStorage {
	return &memStorage{files: make(map[string][]byte)}
}

func (m *memStorage) Put(_ context.Context, key string, reader io.Reader) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.files[key] = data
	m.mu.Unlock()
	return nil
}

func (m *memStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	data, ok := m.files[key]
	m.mu.Unlock()
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *memStorage) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.files, key)
	m.mu.Unlock()
	return nil
}

func (m *memStorage) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	_, ok := m.files[key]
	m.mu.Unlock()
	return ok, nil
}

func (m *memStorage) URL(_ context.Context, key string) (string, error) {
	return "/files/" + key, nil
}

// --- mock torrent repo ---

type memTorrentRepo struct {
	mu       sync.Mutex
	torrents []*model.Torrent
	nextID   int64
}

func newMemTorrentRepo() *memTorrentRepo {
	return &memTorrentRepo{nextID: 1}
}

func (m *memTorrentRepo) GetByID(_ context.Context, id int64) (*model.Torrent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.torrents {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *memTorrentRepo) GetByInfoHash(_ context.Context, infoHash []byte) (*model.Torrent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.torrents {
		if bytes.Equal(t.InfoHash, infoHash) {
			return t, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *memTorrentRepo) List(_ context.Context, opts repository.ListTorrentsOptions) ([]model.Torrent, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []model.Torrent
	for _, t := range m.torrents {
		if opts.CategoryID != nil && t.CategoryID != *opts.CategoryID {
			continue
		}
		result = append(result, *t)
	}
	total := int64(len(result))

	page := opts.Page
	if page <= 0 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 25
	}
	start := (page - 1) * perPage
	if start >= len(result) {
		return nil, total, nil
	}
	end := start + perPage
	if end > len(result) {
		end = len(result)
	}
	return result[start:end], total, nil
}

func (m *memTorrentRepo) Create(_ context.Context, torrent *model.Torrent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	torrent.ID = m.nextID
	m.nextID++
	m.torrents = append(m.torrents, torrent)
	return nil
}

func (m *memTorrentRepo) Update(_ context.Context, torrent *model.Torrent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range m.torrents {
		if t.ID == torrent.ID {
			m.torrents[i] = torrent
			return nil
		}
	}
	return errors.New("not found")
}

func (m *memTorrentRepo) IncrementSeeders(_ context.Context, _ int64, _ int) error { return nil }
func (m *memTorrentRepo) IncrementLeechers(_ context.Context, _ int64, _ int) error { return nil }

// --- mock user repo ---

type memUserRepo struct {
	mu    sync.Mutex
	users []*model.User
}

func newMemUserRepo() *memUserRepo {
	return &memUserRepo{}
}

func (m *memUserRepo) addUser(u *model.User) {
	m.mu.Lock()
	m.users = append(m.users, u)
	m.mu.Unlock()
}

func (m *memUserRepo) GetByID(_ context.Context, id int64) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *memUserRepo) GetByUsername(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("not found")
}

func (m *memUserRepo) GetByEmail(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("not found")
}

func (m *memUserRepo) GetByPasskey(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("not found")
}

func (m *memUserRepo) Count(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.users)), nil
}

func (m *memUserRepo) Create(_ context.Context, _ *model.User) error { return nil }
func (m *memUserRepo) Update(_ context.Context, _ *model.User) error { return nil }

// --- helpers ---

// buildTorrentFile creates a minimal valid .torrent file for testing.
func buildTorrentFile(name string) []byte {
	info := map[string]interface{}{
		"name":         name,
		"piece length": 262144,
		"pieces":       "xxxxxxxxxxxxxxxxxxxx", // 20 bytes (1 fake piece hash)
		"length":       1024,
	}
	infoBytes, _ := bencode.EncodeBytes(info)

	meta := map[string]interface{}{
		"announce": "http://example.com/announce",
		"info":     bencode.RawMessage(infoBytes),
	}
	data, _ := bencode.EncodeBytes(meta)
	return data
}

// buildMultiFileTorrent creates a multi-file .torrent for testing.
func buildMultiFileTorrent(name string) []byte {
	info := map[string]interface{}{
		"name":         name,
		"piece length": 262144,
		"pieces":       "xxxxxxxxxxxxxxxxxxxx",
		"files": []map[string]interface{}{
			{"length": 512, "path": []string{"file1.txt"}},
			{"length": 256, "path": []string{"subdir", "file2.txt"}},
		},
	}
	infoBytes, _ := bencode.EncodeBytes(info)

	meta := map[string]interface{}{
		"announce": "http://example.com/announce",
		"info":     bencode.RawMessage(infoBytes),
	}
	data, _ := bencode.EncodeBytes(meta)
	return data
}

// --- tests ---

func TestParseTorrentFile_SingleFile(t *testing.T) {
	data := buildTorrentFile("test-single")
	parsed, err := ParseTorrentFile(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Name != "test-single" {
		t.Errorf("expected name test-single, got %s", parsed.Name)
	}
	if parsed.Size != 1024 {
		t.Errorf("expected size 1024, got %d", parsed.Size)
	}
	if parsed.FileCount != 1 {
		t.Errorf("expected file_count 1, got %d", parsed.FileCount)
	}
	if len(parsed.InfoHash) != 20 {
		t.Errorf("expected 20-byte info_hash, got %d bytes", len(parsed.InfoHash))
	}
}

func TestParseTorrentFile_MultiFile(t *testing.T) {
	data := buildMultiFileTorrent("test-multi")
	parsed, err := ParseTorrentFile(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Name != "test-multi" {
		t.Errorf("expected name test-multi, got %s", parsed.Name)
	}
	if parsed.Size != 768 {
		t.Errorf("expected size 768, got %d", parsed.Size)
	}
	if parsed.FileCount != 2 {
		t.Errorf("expected file_count 2, got %d", parsed.FileCount)
	}
}

func TestParseTorrentFile_Invalid(t *testing.T) {
	_, err := ParseTorrentFile([]byte("not a torrent"))
	if !errors.Is(err, ErrInvalidTorrent) {
		t.Errorf("expected ErrInvalidTorrent, got %v", err)
	}
}

func TestTorrentService_Upload(t *testing.T) {
	repo := newMemTorrentRepo()
	userRepo := newMemUserRepo()
	store := newMemStorage()
	svc := NewTorrentService(repo, userRepo, store, "http://localhost/announce")

	data := buildTorrentFile("upload-test")
	req := UploadTorrentRequest{
		CategoryID: 1,
	}

	torrent, err := svc.Upload(context.Background(), data, req, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if torrent.Name != "upload-test" {
		t.Errorf("expected name upload-test, got %s", torrent.Name)
	}
	if torrent.UploaderID != 42 {
		t.Errorf("expected uploader_id 42, got %d", torrent.UploaderID)
	}
	if torrent.CategoryID != 1 {
		t.Errorf("expected category_id 1, got %d", torrent.CategoryID)
	}

	// Verify file was stored
	exists, _ := store.Exists(context.Background(), "torrents/1.torrent")
	if !exists {
		t.Error("expected torrent file to be stored")
	}
}

func TestTorrentService_Upload_CustomName(t *testing.T) {
	repo := newMemTorrentRepo()
	userRepo := newMemUserRepo()
	store := newMemStorage()
	svc := NewTorrentService(repo, userRepo, store, "http://localhost/announce")

	data := buildTorrentFile("original-name")
	req := UploadTorrentRequest{
		Name:        "custom-name",
		Description: "a test torrent",
		CategoryID:  2,
		Anonymous:   true,
	}

	torrent, err := svc.Upload(context.Background(), data, req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if torrent.Name != "custom-name" {
		t.Errorf("expected name custom-name, got %s", torrent.Name)
	}
	if torrent.Description == nil || *torrent.Description != "a test torrent" {
		t.Error("expected description to be set")
	}
	if !torrent.Anonymous {
		t.Error("expected anonymous to be true")
	}
}

func TestTorrentService_Upload_Duplicate(t *testing.T) {
	repo := newMemTorrentRepo()
	userRepo := newMemUserRepo()
	store := newMemStorage()
	svc := NewTorrentService(repo, userRepo, store, "http://localhost/announce")

	data := buildTorrentFile("dup-test")
	req := UploadTorrentRequest{CategoryID: 1}

	_, err := svc.Upload(context.Background(), data, req, 1)
	if err != nil {
		t.Fatalf("first upload failed: %v", err)
	}

	_, err = svc.Upload(context.Background(), data, req, 2)
	if !errors.Is(err, ErrDuplicateTorrent) {
		t.Errorf("expected ErrDuplicateTorrent, got %v", err)
	}
}

func TestTorrentService_GetByID(t *testing.T) {
	repo := newMemTorrentRepo()
	userRepo := newMemUserRepo()
	store := newMemStorage()
	svc := NewTorrentService(repo, userRepo, store, "http://localhost/announce")

	data := buildTorrentFile("get-test")
	uploaded, _ := svc.Upload(context.Background(), data, UploadTorrentRequest{CategoryID: 1}, 1)

	torrent, err := svc.GetByID(context.Background(), uploaded.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if torrent.Name != "get-test" {
		t.Errorf("expected name get-test, got %s", torrent.Name)
	}
}

func TestTorrentService_GetByID_NotFound(t *testing.T) {
	repo := newMemTorrentRepo()
	userRepo := newMemUserRepo()
	store := newMemStorage()
	svc := NewTorrentService(repo, userRepo, store, "http://localhost/announce")

	_, err := svc.GetByID(context.Background(), 999)
	if !errors.Is(err, ErrTorrentNotFound) {
		t.Errorf("expected ErrTorrentNotFound, got %v", err)
	}
}

func TestTorrentService_List(t *testing.T) {
	repo := newMemTorrentRepo()
	userRepo := newMemUserRepo()
	store := newMemStorage()
	svc := NewTorrentService(repo, userRepo, store, "http://localhost/announce")

	// Upload 3 torrents
	for i := 0; i < 3; i++ {
		data := buildTorrentFile("list-test-" + string(rune('a'+i)))
		_, _ = svc.Upload(context.Background(), data, UploadTorrentRequest{CategoryID: 1}, 1)
	}

	torrents, total, err := svc.List(context.Background(), repository.ListTorrentsOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(torrents) != 3 {
		t.Errorf("expected 3 torrents, got %d", len(torrents))
	}
}

func TestTorrentService_List_Pagination(t *testing.T) {
	repo := newMemTorrentRepo()
	userRepo := newMemUserRepo()
	store := newMemStorage()
	svc := NewTorrentService(repo, userRepo, store, "http://localhost/announce")

	for i := 0; i < 5; i++ {
		data := buildTorrentFile("page-test-" + string(rune('a'+i)))
		_, _ = svc.Upload(context.Background(), data, UploadTorrentRequest{CategoryID: 1}, 1)
	}

	torrents, total, err := svc.List(context.Background(), repository.ListTorrentsOptions{
		Page:    1,
		PerPage: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(torrents) != 2 {
		t.Errorf("expected 2 torrents on page, got %d", len(torrents))
	}
}

func TestTorrentService_DownloadTorrent(t *testing.T) {
	repo := newMemTorrentRepo()
	userRepo := newMemUserRepo()
	store := newMemStorage()
	svc := NewTorrentService(repo, userRepo, store, "http://tracker.example.com/announce")

	passkey := "abc123passkey"
	userRepo.addUser(&model.User{ID: 1, Passkey: &passkey})

	data := buildTorrentFile("download-test")
	uploaded, err := svc.Upload(context.Background(), data, UploadTorrentRequest{CategoryID: 1}, 1)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	result, filename, err := svc.DownloadTorrent(context.Background(), uploaded.ID, 1)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	if filename != "download-test.torrent" {
		t.Errorf("expected filename download-test.torrent, got %s", filename)
	}

	// Verify announce URL was rewritten
	var meta map[string]interface{}
	if err := bencode.DecodeBytes(result, &meta); err != nil {
		t.Fatalf("failed to decode downloaded torrent: %v", err)
	}
	announce, ok := meta["announce"].(string)
	if !ok {
		t.Fatal("announce not found in downloaded torrent")
	}
	expected := "http://tracker.example.com/announce?passkey=abc123passkey"
	if announce != expected {
		t.Errorf("expected announce %q, got %q", expected, announce)
	}
}

func TestTorrentService_DownloadTorrent_NotFound(t *testing.T) {
	repo := newMemTorrentRepo()
	userRepo := newMemUserRepo()
	store := newMemStorage()
	svc := NewTorrentService(repo, userRepo, store, "http://localhost/announce")

	_, _, err := svc.DownloadTorrent(context.Background(), 999, 1)
	if !errors.Is(err, ErrTorrentNotFound) {
		t.Errorf("expected ErrTorrentNotFound, got %v", err)
	}
}
