package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// Every repository method that talks to the database has a branch for "the
// database said no". Those branches are the ones that never run in a happy-path
// test, and they are exactly where a swallowed error hides: a method that
// returns (nil, nil) or (0, nil) on a broken connection reads to its caller as
// "no such row" or "zero", and the failure surfaces later as corrupt state
// rather than an error.
//
// A closed connection pool makes every query fail deterministically, which lets
// us assert the whole package propagates rather than swallows. This matters most
// for the handful of methods that legitimately return (nil, nil) for "not found"
// — GetActiveMute, for one — where "no error" is a valid answer and the code
// must not give it for the wrong reason.

// closedDB returns a connection pool that has been closed, so every query on it
// fails immediately with sql.ErrConnDone-style errors. It is a separate pool
// from testDB, which stays open for the rest of the suite.
func closedDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("skipping: needs the Postgres container (not available under -short)")
	}
	db := openTestDB(t, dsn)
	if err := db.Close(); err != nil {
		t.Fatalf("closing pool: %v", err)
	}
	return db
}

// TestRepositoriesPropagateDBErrors calls every repository method against a dead
// pool and requires each to return an error. A method that reports success here
// is swallowing a database failure.
func TestRepositoriesPropagateDBErrors(t *testing.T) {
	if testDB == nil {
		t.Skip("skipping: needs the Postgres container (not available under -short)")
	}
	db := closedDB(t)
	ctx := context.Background()
	now := time.Now()

	// Each case runs one method and returns its error. Cases whose method also
	// returns a value assert that the value is the zero value: returning an error
	// alongside a half-populated result invites a caller to use it.
	cases := map[string]func() error{
		// --- users ---------------------------------------------------------
		"UserRepo.Create": func() error { return NewUserRepo(db).Create(ctx, &model.User{}) },
		"UserRepo.GetByID": func() error {
			u, err := NewUserRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "UserRepo.GetByID", u == nil))
		},
		"UserRepo.GetByUsername": func() error {
			u, err := NewUserRepo(db).GetByUsername(ctx, "x")
			return firstErr(err, nilCheck(t, "UserRepo.GetByUsername", u == nil))
		},
		"UserRepo.GetByEmail": func() error {
			u, err := NewUserRepo(db).GetByEmail(ctx, "x@y.z")
			return firstErr(err, nilCheck(t, "UserRepo.GetByEmail", u == nil))
		},
		"UserRepo.GetByPasskey": func() error {
			u, err := NewUserRepo(db).GetByPasskey(ctx, "k")
			return firstErr(err, nilCheck(t, "UserRepo.GetByPasskey", u == nil))
		},
		"UserRepo.Count": func() error {
			_, err := NewUserRepo(db).Count(ctx)
			return err
		},
		"UserRepo.Update":         func() error { return NewUserRepo(db).Update(ctx, &model.User{ID: 1}) },
		"UserRepo.IncrementStats": func() error { return NewUserRepo(db).IncrementStats(ctx, 1, 1, 1) },
		"UserRepo.List": func() error {
			users, _, err := NewUserRepo(db).List(ctx, repository.ListUsersOptions{})
			return firstErr(err, nilCheck(t, "UserRepo.List", users == nil))
		},
		"UserRepo.ListStaff": func() error {
			_, err := NewUserRepo(db).ListStaff(ctx)
			return err
		},
		"UserRepo.UpdateLastAccess": func() error { return NewUserRepo(db).UpdateLastAccess(ctx, 1) },

		// --- torrents ------------------------------------------------------
		"TorrentRepo.Create": func() error { return NewTorrentRepo(db).Create(ctx, &model.Torrent{}) },
		"TorrentRepo.GetByID": func() error {
			tor, err := NewTorrentRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "TorrentRepo.GetByID", tor == nil))
		},
		"TorrentRepo.GetByInfoHash": func() error {
			tor, err := NewTorrentRepo(db).GetByInfoHash(ctx, infoHash("x"))
			return firstErr(err, nilCheck(t, "TorrentRepo.GetByInfoHash", tor == nil))
		},
		"TorrentRepo.List": func() error {
			_, _, err := NewTorrentRepo(db).List(ctx, repository.ListTorrentsOptions{})
			return err
		},
		"TorrentRepo.ListSearch": func() error {
			// The search path builds a different query than the plain list.
			_, _, err := NewTorrentRepo(db).List(ctx, repository.ListTorrentsOptions{Search: "something"})
			return err
		},
		"TorrentRepo.ListByUploader": func() error {
			_, err := NewTorrentRepo(db).ListByUploader(ctx, 1, 10)
			return err
		},
		"TorrentRepo.Update":                  func() error { return NewTorrentRepo(db).Update(ctx, &model.Torrent{ID: 1}) },
		"TorrentRepo.Delete":                  func() error { return NewTorrentRepo(db).Delete(ctx, 1) },
		"TorrentRepo.IncrementSeeders":        func() error { return NewTorrentRepo(db).IncrementSeeders(ctx, 1, 1) },
		"TorrentRepo.IncrementLeechers":       func() error { return NewTorrentRepo(db).IncrementLeechers(ctx, 1, 1) },
		"TorrentRepo.IncrementTimesCompleted": func() error { return NewTorrentRepo(db).IncrementTimesCompleted(ctx, 1) },

		// --- peers ---------------------------------------------------------
		"PeerRepo.Upsert": func() error { return NewPeerRepo(db).Upsert(ctx, &model.Peer{}) },
		"PeerRepo.GetByTorrentAndUser": func() error {
			p, err := NewPeerRepo(db).GetByTorrentAndUser(ctx, 1, 1)
			return firstErr(err, nilCheck(t, "PeerRepo.GetByTorrentAndUser", p == nil))
		},
		"PeerRepo.GetByTorrentUserAndPeerID": func() error {
			p, err := NewPeerRepo(db).GetByTorrentUserAndPeerID(ctx, 1, 1, []byte("p"))
			return firstErr(err, nilCheck(t, "PeerRepo.GetByTorrentUserAndPeerID", p == nil))
		},
		"PeerRepo.ListByTorrent": func() error {
			_, err := NewPeerRepo(db).ListByTorrent(ctx, 1, 10)
			return err
		},
		"PeerRepo.ListByUserSeeding": func() error {
			_, _, err := NewPeerRepo(db).ListByUserSeeding(ctx, 1, 1, 10)
			return err
		},
		"PeerRepo.ListByUserLeeching": func() error {
			_, _, err := NewPeerRepo(db).ListByUserLeeching(ctx, 1, 1, 10)
			return err
		},
		"PeerRepo.CountByUser": func() error {
			_, _, err := NewPeerRepo(db).CountByUser(ctx, 1)
			return err
		},
		"PeerRepo.CountByTorrent": func() error {
			_, err := NewPeerRepo(db).CountByTorrent(ctx, 1)
			return err
		},
		"PeerRepo.CountTotalByUser": func() error {
			_, err := NewPeerRepo(db).CountTotalByUser(ctx, 1)
			return err
		},
		"PeerRepo.Delete": func() error { return NewPeerRepo(db).Delete(ctx, 1, 1, []byte("p")) },
		"PeerRepo.DeleteStale": func() error {
			n, err := NewPeerRepo(db).DeleteStale(ctx, now)
			return firstErr(err, nilCheck(t, "PeerRepo.DeleteStale", n == 0))
		},

		// --- transfer history ----------------------------------------------
		"TransferHistoryRepo.Upsert": func() error {
			return NewTransferHistoryRepo(db).Upsert(ctx, &model.TransferHistory{})
		},
		"TransferHistoryRepo.ListByUser": func() error {
			_, _, err := NewTransferHistoryRepo(db).ListByUser(ctx, 1, 1, 10)
			return err
		},

		// --- forum ----------------------------------------------------------
		"ForumPostRepo.Create": func() error { return NewForumPostRepo(db).Create(ctx, &model.ForumPost{}) },
		"ForumPostRepo.GetByID": func() error {
			p, err := NewForumPostRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "ForumPostRepo.GetByID", p == nil))
		},
		"ForumPostRepo.ListByTopic": func() error {
			_, _, err := NewForumPostRepo(db).ListByTopic(ctx, 1, 1, 10)
			return err
		},
		"ForumPostRepo.Update": func() error { return NewForumPostRepo(db).Update(ctx, &model.ForumPost{ID: 1}) },
		"ForumPostRepo.Delete": func() error { return NewForumPostRepo(db).Delete(ctx, 1) },
		"ForumPostRepo.CountByUser": func() error {
			_, err := NewForumPostRepo(db).CountByUser(ctx, 1)
			return err
		},
		"ForumPostRepo.Search": func() error {
			// A non-empty query is required or Search short-circuits before touching the DB.
			_, _, err := NewForumPostRepo(db).Search(ctx, "term", nil, 100, 1, 10)
			return err
		},
		"ForumPostRepo.GetFirstPostIDByTopic": func() error {
			_, err := NewForumPostRepo(db).GetFirstPostIDByTopic(ctx, 1)
			return err
		},
		"ForumPostRepo.SoftDelete": func() error { return NewForumPostRepo(db).SoftDelete(ctx, 1, 1) },
		"ForumPostRepo.Restore":    func() error { return NewForumPostRepo(db).Restore(ctx, 1) },
		"ForumPostRepo.CreateEdit": func() error {
			return NewForumPostRepo(db).CreateEdit(ctx, &model.ForumPostEdit{PostID: 1})
		},
		"ForumPostRepo.ListEdits": func() error {
			_, err := NewForumPostRepo(db).ListEdits(ctx, 1)
			return err
		},
		"ForumRepo.GetByID": func() error {
			f, err := NewForumRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "ForumRepo.GetByID", f == nil))
		},
		"ForumRepo.List": func() error {
			_, err := NewForumRepo(db).List(ctx)
			return err
		},
		"ForumRepo.ListByCategory": func() error {
			_, err := NewForumRepo(db).ListByCategory(ctx, 1)
			return err
		},
		"ForumRepo.Create": func() error { return NewForumRepo(db).Create(ctx, &model.Forum{}) },
		"ForumRepo.Update": func() error { return NewForumRepo(db).Update(ctx, &model.Forum{ID: 1}) },
		"ForumRepo.Delete": func() error { return NewForumRepo(db).Delete(ctx, 1) },
		"ForumRepo.CountTopicsByForum": func() error {
			_, err := NewForumRepo(db).CountTopicsByForum(ctx, 1)
			return err
		},
		"ForumRepo.IncrementTopicCount": func() error { return NewForumRepo(db).IncrementTopicCount(ctx, 1, 1) },
		"ForumRepo.IncrementPostCount":  func() error { return NewForumRepo(db).IncrementPostCount(ctx, 1, 1) },
		"ForumRepo.UpdateLastPost":      func() error { return NewForumRepo(db).UpdateLastPost(ctx, 1, 1) },
		"ForumRepo.RecalculateLastPost": func() error { return NewForumRepo(db).RecalculateLastPost(ctx, 1) },
		"ForumRepo.RecalculateCounts":   func() error { return NewForumRepo(db).RecalculateCounts(ctx, 1) },
		"ForumCategoryRepo.GetByID": func() error {
			c, err := NewForumCategoryRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "ForumCategoryRepo.GetByID", c == nil))
		},
		"ForumCategoryRepo.List": func() error {
			_, err := NewForumCategoryRepo(db).List(ctx)
			return err
		},
		"ForumCategoryRepo.Create": func() error {
			return NewForumCategoryRepo(db).Create(ctx, &model.ForumCategory{})
		},
		"ForumCategoryRepo.Update": func() error {
			return NewForumCategoryRepo(db).Update(ctx, &model.ForumCategory{ID: 1})
		},
		"ForumCategoryRepo.Delete": func() error { return NewForumCategoryRepo(db).Delete(ctx, 1) },
		"ForumCategoryRepo.CountForumsByCategory": func() error {
			_, err := NewForumCategoryRepo(db).CountForumsByCategory(ctx, 1)
			return err
		},
		"ForumTopicRepo.GetByID": func() error {
			tp, err := NewForumTopicRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "ForumTopicRepo.GetByID", tp == nil))
		},
		"ForumTopicRepo.ListByForum": func() error {
			_, _, err := NewForumTopicRepo(db).ListByForum(ctx, 1, 1, 10)
			return err
		},
		"ForumTopicRepo.Create": func() error { return NewForumTopicRepo(db).Create(ctx, &model.ForumTopic{}) },
		"ForumTopicRepo.IncrementViewCount": func() error {
			return NewForumTopicRepo(db).IncrementViewCount(ctx, 1)
		},
		"ForumTopicRepo.IncrementPostCount": func() error {
			return NewForumTopicRepo(db).IncrementPostCount(ctx, 1, 1)
		},
		"ForumTopicRepo.UpdateLastPost": func() error {
			return NewForumTopicRepo(db).UpdateLastPost(ctx, 1, 1, now)
		},
		"ForumTopicRepo.RecalculateLastPost": func() error {
			return NewForumTopicRepo(db).RecalculateLastPost(ctx, 1)
		},
		"ForumTopicRepo.SetLocked":     func() error { return NewForumTopicRepo(db).SetLocked(ctx, 1, true) },
		"ForumTopicRepo.SetPinned":     func() error { return NewForumTopicRepo(db).SetPinned(ctx, 1, true) },
		"ForumTopicRepo.UpdateTitle":   func() error { return NewForumTopicRepo(db).UpdateTitle(ctx, 1, "t") },
		"ForumTopicRepo.UpdateForumID": func() error { return NewForumTopicRepo(db).UpdateForumID(ctx, 1, 1) },
		"ForumTopicRepo.Delete":        func() error { return NewForumTopicRepo(db).Delete(ctx, 1) },

		// --- moderation -----------------------------------------------------
		"WarningRepo.Create": func() error { return NewWarningRepo(db).Create(ctx, &model.Warning{}) },
		"WarningRepo.GetByID": func() error {
			w, err := NewWarningRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "WarningRepo.GetByID", w == nil))
		},
		"WarningRepo.ListByUser": func() error {
			_, err := NewWarningRepo(db).ListByUser(ctx, 1, true)
			return err
		},
		"WarningRepo.ListAll": func() error {
			_, _, err := NewWarningRepo(db).ListAll(ctx, repository.ListWarningsOptions{})
			return err
		},
		"WarningRepo.Update": func() error { return NewWarningRepo(db).Update(ctx, &model.Warning{ID: 1}) },
		"WarningRepo.CountActiveByUser": func() error {
			_, err := NewWarningRepo(db).CountActiveByUser(ctx, 1)
			return err
		},
		"WarningRepo.CountActiveManualByUser": func() error {
			_, err := NewWarningRepo(db).CountActiveManualByUser(ctx, 1)
			return err
		},
		"WarningRepo.GetActiveRatioWarning": func() error {
			w, err := NewWarningRepo(db).GetActiveRatioWarning(ctx, 1)
			return firstErr(err, nilCheck(t, "WarningRepo.GetActiveRatioWarning", w == nil))
		},
		"WarningRepo.GetUsersWithLowRatio": func() error {
			_, err := NewWarningRepo(db).GetUsersWithLowRatio(ctx, 0.5, 1)
			return err
		},
		"WarningRepo.ResolveExpiredManualWarnings": func() error {
			_, err := NewWarningRepo(db).ResolveExpiredManualWarnings(ctx)
			return err
		},
		"RestrictionRepo.Create": func() error { return NewRestrictionRepo(db).Create(ctx, &model.Restriction{}) },
		"RestrictionRepo.GetByID": func() error {
			r, err := NewRestrictionRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "RestrictionRepo.GetByID", r == nil))
		},
		"RestrictionRepo.ListByUser": func() error {
			_, err := NewRestrictionRepo(db).ListByUser(ctx, 1)
			return err
		},
		"RestrictionRepo.ListActive": func() error {
			_, err := NewRestrictionRepo(db).ListActive(ctx)
			return err
		},
		"RestrictionRepo.Lift": func() error { return NewRestrictionRepo(db).Lift(ctx, 1, nil) },
		"RestrictionRepo.LiftExpired": func() error {
			_, err := NewRestrictionRepo(db).LiftExpired(ctx)
			return err
		},
		"RestrictionRepo.HasActiveByType": func() error {
			// A swallowed error here would report "not restricted" and let a
			// banned user through, so the false must come with an error.
			ok, err := NewRestrictionRepo(db).HasActiveByType(ctx, 1, "upload")
			return firstErr(err, nilCheck(t, "RestrictionRepo.HasActiveByType", !ok))
		},
		"CheatFlagRepo.Create": func() error { return NewCheatFlagRepo(db).Create(ctx, &model.CheatFlag{}) },
		"CheatFlagRepo.GetByID": func() error {
			f, err := NewCheatFlagRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "CheatFlagRepo.GetByID", f == nil))
		},
		"CheatFlagRepo.List": func() error {
			_, _, err := NewCheatFlagRepo(db).List(ctx, repository.ListCheatFlagsOptions{})
			return err
		},
		"CheatFlagRepo.Dismiss": func() error { return NewCheatFlagRepo(db).Dismiss(ctx, 1, 1) },
		"CheatFlagRepo.HasRecentUndismissed": func() error {
			ok, err := NewCheatFlagRepo(db).HasRecentUndismissed(ctx, 1, 1, "t", 24)
			return firstErr(err, nilCheck(t, "CheatFlagRepo.HasRecentUndismissed", !ok))
		},
		"ModNoteRepo.Create": func() error { return NewModNoteRepo(db).Create(ctx, &model.ModNote{}) },
		"ModNoteRepo.GetByID": func() error {
			n, err := NewModNoteRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "ModNoteRepo.GetByID", n == nil))
		},
		"ModNoteRepo.ListByUser": func() error {
			_, err := NewModNoteRepo(db).ListByUser(ctx, 1)
			return err
		},
		"ModNoteRepo.Delete": func() error { return NewModNoteRepo(db).Delete(ctx, 1) },
		"BanRepo.CreateEmailBan": func() error {
			return NewBanRepo(db).CreateEmailBan(ctx, &model.BannedEmail{Pattern: "x"})
		},
		"BanRepo.DeleteEmailBan": func() error { return NewBanRepo(db).DeleteEmailBan(ctx, 1) },
		"BanRepo.ListEmailBans": func() error {
			_, err := NewBanRepo(db).ListEmailBans(ctx)
			return err
		},
		"BanRepo.IsEmailBanned": func() error {
			// A swallowed error would silently un-ban everyone.
			banned, err := NewBanRepo(db).IsEmailBanned(ctx, "a@b.c")
			return firstErr(err, nilCheck(t, "BanRepo.IsEmailBanned", !banned))
		},
		"BanRepo.CreateIPBan": func() error {
			return NewBanRepo(db).CreateIPBan(ctx, &model.BannedIP{IPRange: "10.0.0.0/8"})
		},
		"BanRepo.DeleteIPBan": func() error { return NewBanRepo(db).DeleteIPBan(ctx, 1) },
		"BanRepo.ListIPBans": func() error {
			_, err := NewBanRepo(db).ListIPBans(ctx)
			return err
		},
		"BanRepo.IsIPBanned": func() error {
			banned, err := NewBanRepo(db).IsIPBanned(ctx, "10.0.0.1")
			return firstErr(err, nilCheck(t, "BanRepo.IsIPBanned", !banned))
		},
		"ReportRepo.Create": func() error { return NewReportRepo(db).Create(ctx, &model.Report{}) },
		"ReportRepo.GetByID": func() error {
			r, err := NewReportRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "ReportRepo.GetByID", r == nil))
		},
		"ReportRepo.ExistsByReporterAndTorrent": func() error {
			_, err := NewReportRepo(db).ExistsByReporterAndTorrent(ctx, 1, nil)
			return err
		},
		"ReportRepo.List": func() error {
			_, _, err := NewReportRepo(db).List(ctx, repository.ListReportsOptions{})
			return err
		},
		"ReportRepo.Resolve": func() error { return NewReportRepo(db).Resolve(ctx, 1, 1) },

		// --- content --------------------------------------------------------
		"NewsRepo.Create": func() error { return NewNewsRepo(db).Create(ctx, &model.NewsArticle{}) },
		"NewsRepo.GetByID": func() error {
			a, err := NewNewsRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "NewsRepo.GetByID", a == nil))
		},
		"NewsRepo.GetPublishedByID": func() error {
			a, err := NewNewsRepo(db).GetPublishedByID(ctx, 1)
			return firstErr(err, nilCheck(t, "NewsRepo.GetPublishedByID", a == nil))
		},
		"NewsRepo.Update": func() error { return NewNewsRepo(db).Update(ctx, &model.NewsArticle{ID: 1}) },
		"NewsRepo.Delete": func() error { return NewNewsRepo(db).Delete(ctx, 1) },
		"NewsRepo.List": func() error {
			_, _, err := NewNewsRepo(db).List(ctx, repository.ListNewsOptions{})
			return err
		},
		"NewsRepo.ListPublished": func() error {
			_, _, err := NewNewsRepo(db).ListPublished(ctx, 1, 10)
			return err
		},
		"CommentRepo.Create": func() error { return NewCommentRepo(db).Create(ctx, &model.Comment{}) },
		"CommentRepo.GetByID": func() error {
			c, err := NewCommentRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "CommentRepo.GetByID", c == nil))
		},
		"CommentRepo.ListByTorrent": func() error {
			_, _, err := NewCommentRepo(db).ListByTorrent(ctx, 1, 1, 10)
			return err
		},
		"CommentRepo.Update": func() error { return NewCommentRepo(db).Update(ctx, &model.Comment{ID: 1}) },
		"CommentRepo.Delete": func() error { return NewCommentRepo(db).Delete(ctx, 1) },
		"RatingRepo.Upsert":  func() error { return NewRatingRepo(db).Upsert(ctx, &model.Rating{}) },
		"RatingRepo.GetByTorrentAndUser": func() error {
			r, err := NewRatingRepo(db).GetByTorrentAndUser(ctx, 1, 1)
			return firstErr(err, nilCheck(t, "RatingRepo.GetByTorrentAndUser", r == nil))
		},
		"RatingRepo.GetStatsByTorrent": func() error {
			_, _, err := NewRatingRepo(db).GetStatsByTorrent(ctx, 1)
			return err
		},
		"CategoryRepo.GetByID": func() error {
			c, err := NewCategoryRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "CategoryRepo.GetByID", c == nil))
		},
		"CategoryRepo.List": func() error {
			_, err := NewCategoryRepo(db).List(ctx)
			return err
		},
		"CategoryRepo.Create": func() error { return NewCategoryRepo(db).Create(ctx, &model.Category{}) },
		"CategoryRepo.Update": func() error { return NewCategoryRepo(db).Update(ctx, &model.Category{ID: 1}) },
		"CategoryRepo.Delete": func() error { return NewCategoryRepo(db).Delete(ctx, 1) },
		"CategoryRepo.CountTorrentsByCategory": func() error {
			_, err := NewCategoryRepo(db).CountTorrentsByCategory(ctx, 1)
			return err
		},
		"GroupRepo.List": func() error {
			_, err := NewGroupRepo(db).List(ctx)
			return err
		},
		"GroupRepo.GetByID": func() error {
			g, err := NewGroupRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "GroupRepo.GetByID", g == nil))
		},

		// --- messaging ------------------------------------------------------
		"MessageRepo.Create": func() error { return NewMessageRepo(db).Create(ctx, &model.Message{}) },
		"MessageRepo.GetByID": func() error {
			m, err := NewMessageRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "MessageRepo.GetByID", m == nil))
		},
		"MessageRepo.ListInbox": func() error {
			_, _, err := NewMessageRepo(db).ListInbox(ctx, 1, 1, 10)
			return err
		},
		"MessageRepo.ListOutbox": func() error {
			_, _, err := NewMessageRepo(db).ListOutbox(ctx, 1, 1, 10)
			return err
		},
		"MessageRepo.MarkAsRead":    func() error { return NewMessageRepo(db).MarkAsRead(ctx, 1, 1) },
		"MessageRepo.DeleteForUser": func() error { return NewMessageRepo(db).DeleteForUser(ctx, 1, 1) },
		"MessageRepo.CountUnread": func() error {
			_, err := NewMessageRepo(db).CountUnread(ctx, 1)
			return err
		},
		"ChatMessageRepo.Create": func() error { return NewChatMessageRepo(db).Create(ctx, &model.ChatMessage{}) },
		"ChatMessageRepo.ListRecent": func() error {
			_, err := NewChatMessageRepo(db).ListRecent(ctx, 10)
			return err
		},
		"ChatMessageRepo.ListBefore": func() error {
			_, err := NewChatMessageRepo(db).ListBefore(ctx, 5, 10)
			return err
		},
		"ChatMessageRepo.Delete": func() error { return NewChatMessageRepo(db).Delete(ctx, 1) },
		"ChatMessageRepo.DeleteByUserID": func() error {
			n, err := NewChatMessageRepo(db).DeleteByUserID(ctx, 1)
			return firstErr(err, nilCheck(t, "ChatMessageRepo.DeleteByUserID", n == 0))
		},
		"ChatMuteRepo.Create": func() error { return NewChatMuteRepo(db).Create(ctx, &model.ChatMute{}) },
		"ChatMuteRepo.GetActiveMute": func() error {
			// This method returns (nil, nil) for "no active mute". On a dead
			// database it must NOT do that: a swallowed error would un-mute
			// everyone the moment the DB hiccups.
			m, err := NewChatMuteRepo(db).GetActiveMute(ctx, 1)
			return firstErr(err, nilCheck(t, "ChatMuteRepo.GetActiveMute", m == nil))
		},
		"ChatMuteRepo.ListActive": func() error {
			_, _, err := NewChatMuteRepo(db).ListActive(ctx, 1, 10)
			return err
		},
		"ChatMuteRepo.Delete": func() error { return NewChatMuteRepo(db).Delete(ctx, 1) },
		"ChatMuteRepo.DeleteExpired": func() error {
			_, err := NewChatMuteRepo(db).DeleteExpired(ctx)
			return err
		},

		// --- notifications ---------------------------------------------------
		"NotificationRepo.Create": func() error {
			return NewNotificationRepo(db).Create(ctx, &model.Notification{Data: []byte(`{}`)})
		},
		"NotificationRepo.GetByID": func() error {
			n, err := NewNotificationRepo(db).GetByID(ctx, 1)
			return firstErr(err, nilCheck(t, "NotificationRepo.GetByID", n == nil))
		},
		"NotificationRepo.List": func() error {
			_, _, err := NewNotificationRepo(db).List(ctx, 1, repository.ListNotificationsOptions{})
			return err
		},
		"NotificationRepo.MarkRead":    func() error { return NewNotificationRepo(db).MarkRead(ctx, 1, 1) },
		"NotificationRepo.MarkAllRead": func() error { return NewNotificationRepo(db).MarkAllRead(ctx, 1) },
		"NotificationRepo.CountUnread": func() error {
			_, err := NewNotificationRepo(db).CountUnread(ctx, 1)
			return err
		},
		"NotificationRepo.DeleteOld": func() error {
			n, err := NewNotificationRepo(db).DeleteOld(ctx, now)
			return firstErr(err, nilCheck(t, "NotificationRepo.DeleteOld", n == 0))
		},
		"NotificationPreferenceRepo.Get": func() error {
			p, err := NewNotificationPreferenceRepo(db).Get(ctx, 1, "pm")
			return firstErr(err, nilCheck(t, "NotificationPreferenceRepo.Get", p == nil))
		},
		"NotificationPreferenceRepo.GetAll": func() error {
			_, err := NewNotificationPreferenceRepo(db).GetAll(ctx, 1)
			return err
		},
		"NotificationPreferenceRepo.Set": func() error {
			return NewNotificationPreferenceRepo(db).Set(ctx, 1, "pm", true)
		},
		"NotificationPreferenceRepo.IsEnabled": func() error {
			_, err := NewNotificationPreferenceRepo(db).IsEnabled(ctx, 1, "pm")
			return err
		},
		"TopicSubscriptionRepo.Subscribe":   func() error { return NewTopicSubscriptionRepo(db).Subscribe(ctx, 1, 1) },
		"TopicSubscriptionRepo.Unsubscribe": func() error { return NewTopicSubscriptionRepo(db).Unsubscribe(ctx, 1, 1) },
		"TopicSubscriptionRepo.IsSubscribed": func() error {
			_, err := NewTopicSubscriptionRepo(db).IsSubscribed(ctx, 1, 1)
			return err
		},
		"TopicSubscriptionRepo.ListSubscribers": func() error {
			_, err := NewTopicSubscriptionRepo(db).ListSubscribers(ctx, 1)
			return err
		},

		// --- misc -------------------------------------------------------------
		"InviteRepo.Create": func() error { return NewInviteRepo(db).Create(ctx, &model.Invite{}) },
		"InviteRepo.GetByToken": func() error {
			inv, err := NewInviteRepo(db).GetByToken(ctx, "t")
			return firstErr(err, nilCheck(t, "InviteRepo.GetByToken", inv == nil))
		},
		"InviteRepo.ListByInviter": func() error {
			_, _, err := NewInviteRepo(db).ListByInviter(ctx, 1, 1, 10)
			return err
		},
		"InviteRepo.Redeem": func() error { return NewInviteRepo(db).Redeem(ctx, "t", 1) },
		"InviteRepo.CountPendingByInviter": func() error {
			_, err := NewInviteRepo(db).CountPendingByInviter(ctx, 1)
			return err
		},
		"ActivityLogRepo.Create": func() error { return NewActivityLogRepo(db).Create(ctx, &model.ActivityLog{}) },
		"ActivityLogRepo.List": func() error {
			_, _, err := NewActivityLogRepo(db).List(ctx, repository.ListActivityLogsOptions{})
			return err
		},
		"ReseedRequestRepo.Create": func() error {
			return NewReseedRequestRepo(db).Create(ctx, &model.ReseedRequest{})
		},
		"ReseedRequestRepo.ExistsByTorrentAndUser": func() error {
			_, err := NewReseedRequestRepo(db).ExistsByTorrentAndUser(ctx, 1, 1)
			return err
		},
		"ReseedRequestRepo.CountByTorrent": func() error {
			_, err := NewReseedRequestRepo(db).CountByTorrent(ctx, 1)
			return err
		},
		"SiteSettingsRepo.Get": func() error {
			s, err := NewSiteSettingsRepo(db).Get(ctx, "k")
			return firstErr(err, nilCheck(t, "SiteSettingsRepo.Get", s == nil))
		},
		"SiteSettingsRepo.Set": func() error { return NewSiteSettingsRepo(db).Set(ctx, "k", "v") },
		"SiteSettingsRepo.GetAll": func() error {
			_, err := NewSiteSettingsRepo(db).GetAll(ctx)
			return err
		},
		"DashboardRepo.GetStats": func() error {
			s, err := NewDashboardRepo(db).GetStats(ctx)
			return firstErr(err, nilCheck(t, "DashboardRepo.GetStats", s == nil))
		},
		"EmailConfirmationRepo.Create": func() error {
			return NewEmailConfirmationRepo(db).Create(ctx, 1, []byte("h"), now)
		},
		"EmailConfirmationRepo.ClaimByTokenHash": func() error {
			c, err := NewEmailConfirmationRepo(db).ClaimByTokenHash(ctx, []byte("h"))
			return firstErr(err, nilCheck(t, "EmailConfirmationRepo.ClaimByTokenHash", c == nil))
		},
		"EmailConfirmationRepo.GetLatestByUserID": func() error {
			c, err := NewEmailConfirmationRepo(db).GetLatestByUserID(ctx, 1)
			return firstErr(err, nilCheck(t, "EmailConfirmationRepo.GetLatestByUserID", c == nil))
		},
		"EmailConfirmationRepo.DeleteByUserID": func() error {
			return NewEmailConfirmationRepo(db).DeleteByUserID(ctx, 1)
		},
		"PasswordResetRepo.Create": func() error {
			return NewPasswordResetRepo(db).Create(&service.PasswordReset{UserID: 1, CreatedAt: now, ExpiresAt: now})
		},
		"PasswordResetRepo.ClaimByTokenHash": func() error {
			pr, err := NewPasswordResetRepo(db).ClaimByTokenHash("h")
			return firstErr(err, nilCheck(t, "PasswordResetRepo.ClaimByTokenHash", pr == nil))
		},
		"PasswordResetRepo.CountRecentByUserID": func() error {
			_, err := NewPasswordResetRepo(db).CountRecentByUserID(1, time.Hour)
			return err
		},
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Errorf("%s on a closed pool returned nil error — the database failure was swallowed", name)
			}
		})
	}
}

// firstErr returns the first non-nil error, so a case can report either the
// method's error or a zero-value violation.
func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// nilCheck fails the test when a method returned a non-zero value alongside an
// error. It returns nil so it composes into firstErr without masking the real error.
func nilCheck(t *testing.T, name string, isZero bool) error {
	t.Helper()
	if !isZero {
		t.Errorf("%s returned a non-zero value alongside an error; callers may use it", name)
	}
	return nil
}
