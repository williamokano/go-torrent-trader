# TorrentTrader 3.0 - Complete Feature Documentation

> **Purpose**: This document dissects every feature, endpoint, flow, and business rule in TorrentTrader 3.0.
> It is implementation-agnostic and designed to serve as a specification for porting to Go or modern PHP.

---

## Table of Contents

1. [Database Schema](#1-database-schema)
2. [Tracker / Announce System](#2-tracker--announce-system)
3. [Torrent Management](#3-torrent-management)
4. [User & Authentication System](#4-user--authentication-system)
5. [Invitation System](#5-invitation-system)
6. [Forum System](#6-forum-system)
7. [Shoutbox (Chat)](#7-shoutbox-chat)
8. [Private Messaging](#8-private-messaging)
9. [Admin Control Panel](#9-admin-control-panel)
10. [Cleanup / Cron Tasks](#10-cleanup--cron-tasks)
11. [Blocks / Widget System](#11-blocks--widget-system)
12. [Miscellaneous Systems](#12-miscellaneous-systems)
13. [Configuration Reference](#13-configuration-reference)

---

## 1. Database Schema

### 1.1 Table Overview

| Table | Purpose | Engine |
|---|---|---|
| `users` | User accounts, stats, preferences | MyISAM |
| `groups` | Role definitions with granular permissions | MyISAM |
| `torrents` | Core torrent metadata | MyISAM |
| `files` | Individual files within a torrent | MyISAM |
| `peers` | Active peers (ephemeral tracker state) | MyISAM |
| `completed` | Download completion log per user/torrent | MyISAM |
| `announce` | Tracker announce URLs per torrent | MyISAM |
| `categories` | Torrent categories (hierarchical) | MyISAM |
| `torrentlang` | Torrent language options | MyISAM |
| `comments` | Comments on torrents and news | MyISAM |
| `ratings` | Torrent ratings (1-5 stars) | MyISAM |
| `reports` | Abuse reports (torrent/user/comment/forum) | MyISAM |
| `messages` | Private messages between users | MyISAM |
| `forum_forums` | Forum categories | MyISAM |
| `forum_topics` | Forum threads | MyISAM |
| `forum_posts` | Forum post content | MyISAM |
| `forum_readposts` | Per-user read tracking for forum | MyISAM |
| `forumcats` | Forum category groupings | MyISAM |
| `shoutbox` | Chat/shoutbox messages | MyISAM |
| `news` | Site news articles | MyISAM |
| `polls` | Site polls (up to 20 options) | MyISAM |
| `pollanswers` | Poll vote records | MyISAM |
| `teams` | User teams/groups | MyISAM |
| `bans` | IP address bans (range support) | MyISAM |
| `email_bans` | Email/domain bans | MyISAM |
| `warnings` | User warnings with expiry | MyISAM |
| `blocks` | Sidebar widget configuration | MyISAM |
| `faq` | FAQ categories and items | MyISAM |
| `rules` | Site rules (role-gated visibility) | MyISAM |
| `countries` | Country list with flags | MyISAM |
| `languages` | UI language files | MyISAM |
| `stylesheets` | Theme definitions | MyISAM |
| `censor` | Word censor replacements | MyISAM |
| `guests` | Guest visitor tracking | MyISAM |
| `tasks` | Cron task scheduling | MyISAM |
| `log` | Admin/system activity log | MyISAM |
| `sqlerr` | SQL error log | MyISAM |

> **Note**: All tables use MyISAM. No foreign key constraints are enforced at the DB level.
> Relationships are implicit via naming conventions. A port should use InnoDB (or equivalent)
> with proper foreign keys and transactions.

### 1.2 Core Tables - Detailed Schema

#### users

| Column | Type | Notes |
|---|---|---|
| id | INT UNSIGNED AUTO_INCREMENT | PK |
| username | VARCHAR(40) | UNIQUE |
| password | VARCHAR(40) | Hashed (SHA1/MD5/HMAC) |
| secret | VARCHAR(20) BINARY | Session/recovery token |
| editsecret | VARCHAR(20) BINARY | Email change token |
| email | VARCHAR(80) | |
| status | ENUM('pending','confirmed') | |
| enabled | VARCHAR(10) | 'yes'/'no' |
| added | DATETIME | Registration date |
| last_login | DATETIME | |
| last_access | DATETIME | |
| last_browse | INT | Timestamp of last torrent browse |
| ip | VARCHAR(39) | Last known IP (IPv4/IPv6) |
| class | TINYINT UNSIGNED | FK to groups.group_id |
| privacy | ENUM('strong','normal','low') | Profile visibility |
| stylesheet | INT | FK to stylesheets.id |
| language | VARCHAR(20) | FK to languages.id |
| uploaded | BIGINT UNSIGNED | Total bytes uploaded |
| downloaded | BIGINT UNSIGNED | Total bytes downloaded |
| passkey | VARCHAR(32) | Tracker authentication key |
| avatar | VARCHAR(100) | Avatar URL |
| title | VARCHAR(30) | Custom title |
| signature | VARCHAR(200) | |
| info | TEXT | Bio/profile text |
| country | INT UNSIGNED | FK to countries.id |
| gender | VARCHAR(6) | |
| age | INT | |
| client | VARCHAR(25) | Preferred torrent client |
| donated | INT UNSIGNED | Donation amount |
| warned | CHAR(3) | 'yes'/'no' |
| forumbanned | CHAR(3) | 'yes'/'no' |
| modcomment | TEXT | Staff-only notes |
| acceptpms | ENUM('yes','no') | Accept PMs from non-staff |
| commentpm | ENUM('yes','no') | Notify on torrent comments |
| notifs | VARCHAR(100) | Notification preferences (e.g. '[pm]') |
| invited_by | INT | FK to users.id (self-referencing) |
| invitees | VARCHAR(100) | Space-separated invited user IDs |
| invites | SMALLINT | Remaining invite count |
| invitedate | DATETIME | |
| team | INT UNSIGNED | FK to teams.id |
| tzoffset | INT | Timezone offset in hours |
| hideshoutbox | ENUM('yes','no') | |
| page | TEXT | Current page (for "who's online") |

**Indexes**: username (UNIQUE), (status, added), ip, uploaded, downloaded, country

#### groups

| Column | Type | Default |
|---|---|---|
| group_id | INT AUTO_INCREMENT | PK |
| level | VARCHAR(50) | Group display name |
| view_torrents | ENUM('yes','no') | 'yes' |
| edit_torrents | ENUM('yes','no') | 'no' |
| delete_torrents | ENUM('yes','no') | 'no' |
| view_users | ENUM('yes','no') | 'yes' |
| edit_users | ENUM('yes','no') | 'no' |
| delete_users | ENUM('yes','no') | 'no' |
| view_news | ENUM('yes','no') | 'yes' |
| edit_news | ENUM('yes','no') | 'no' |
| delete_news | ENUM('yes','no') | 'no' |
| can_upload | ENUM('yes','no') | 'no' |
| can_download | ENUM('yes','no') | 'yes' |
| view_forum | ENUM('yes','no') | 'yes' |
| edit_forum | ENUM('yes','no') | 'yes' |
| delete_forum | ENUM('yes','no') | 'no' |
| control_panel | ENUM('yes','no') | 'no' |
| staff_page | ENUM('yes','no') | 'no' |
| staff_public | ENUM('yes','no') | 'no' |
| staff_sort | TINYINT UNSIGNED | 0 |

**Default Groups (seed data)**:

| group_id | Level | Key Permissions |
|---|---|---|
| 1 | Member | upload, download, forum read/write |
| 2 | Power User | download only |
| 3 | VIP | upload, download |
| 4 | Uploader | upload, staff page |
| 5 | Moderator | edit torrents/users, manage forum |
| 6 | Super Moderator | full permissions, staff public |
| 7 | Administrator | full permissions, control panel |

#### torrents

| Column | Type | Notes |
|---|---|---|
| id | INT UNSIGNED AUTO_INCREMENT | PK |
| info_hash | VARCHAR(40) | UNIQUE (hex-encoded, 40 chars) |
| name | VARCHAR(255) | FULLTEXT indexed |
| filename | VARCHAR(255) | Original .torrent filename |
| save_as | VARCHAR(255) | |
| descr | TEXT | Description (BBCode) |
| image1 | TEXT | Custom image URL/path |
| image2 | TEXT | Custom image URL/path |
| category | INT UNSIGNED | FK to categories.id |
| torrentlang | INT UNSIGNED | FK to torrentlang.id |
| size | BIGINT UNSIGNED | Total bytes |
| added | DATETIME | |
| type | ENUM('single','multi') | |
| numfiles | INT UNSIGNED | |
| owner | INT UNSIGNED | FK to users.id |
| anon | ENUM('yes','no') | Anonymous upload |
| comments | INT UNSIGNED | Denormalized count |
| views | INT UNSIGNED | Detail page views |
| hits | INT UNSIGNED | Download link clicks |
| times_completed | INT UNSIGNED | Denormalized completion count |
| seeders | INT UNSIGNED | Denormalized seeder count |
| leechers | INT UNSIGNED | Denormalized leecher count |
| numratings | INT UNSIGNED | |
| ratingsum | INT UNSIGNED | Sum for avg calculation |
| visible | ENUM('yes','no') | |
| banned | ENUM('yes','no') | |
| nfo | ENUM('yes','no') | NFO file exists |
| announce | VARCHAR(255) | Primary announce URL |
| external | ENUM('yes','no') | External tracker |
| freeleech | ENUM('0','1') | |
| last_action | DATETIME | |

**Indexes**: info_hash (UNIQUE, first 20 chars), owner, visible, (category, visible), FULLTEXT on name

#### peers

| Column | Type | Notes |
|---|---|---|
| id | INT UNSIGNED AUTO_INCREMENT | PK |
| torrent | INT UNSIGNED | FK to torrents.id |
| peer_id | VARCHAR(40) | BitTorrent peer ID |
| ip | VARCHAR(64) | |
| port | SMALLINT UNSIGNED | |
| uploaded | BIGINT UNSIGNED | Per-session bytes up |
| downloaded | BIGINT UNSIGNED | Per-session bytes down |
| to_go | BIGINT UNSIGNED | Bytes remaining |
| seeder | ENUM('yes','no') | |
| started | DATETIME | |
| last_action | DATETIME | |
| connectable | ENUM('yes','no') | Port reachable |
| client | VARCHAR(60) | Client identifier |
| userid | VARCHAR(32) | FK to users.id |
| passkey | VARCHAR(32) | |

**Indexes**: (torrent, peer_id) UNIQUE, torrent, (torrent, seeder), last_action

#### completed

| Column | Type | Notes |
|---|---|---|
| id | INT UNSIGNED AUTO_INCREMENT | PK |
| userid | INT | FK to users.id |
| torrentid | INT | FK to torrents.id |
| date | DATETIME | |

**Indexes**: (userid, torrentid) UNIQUE

#### messages

| Column | Type | Notes |
|---|---|---|
| id | INT UNSIGNED AUTO_INCREMENT | PK |
| sender | INT UNSIGNED | FK to users.id |
| receiver | INT UNSIGNED | FK to users.id |
| added | DATETIME | |
| subject | TEXT | |
| msg | TEXT | Body |
| unread | ENUM('yes','no') | |
| poster | BIGINT UNSIGNED | Thread ID |
| location | ENUM('in','out','both','draft','template') | |

#### forum_forums

| Column | Type | Notes |
|---|---|---|
| id | INT UNSIGNED AUTO_INCREMENT | PK |
| name | VARCHAR(60) | |
| description | VARCHAR(200) | |
| category | TINYINT | FK to forumcats.id |
| sort | TINYINT UNSIGNED | Display order |
| minclassread | TINYINT UNSIGNED | Min group to read |
| minclasswrite | TINYINT UNSIGNED | Min group to post |
| guest_read | ENUM('yes','no') | |

#### forum_topics

| Column | Type | Notes |
|---|---|---|
| id | INT UNSIGNED AUTO_INCREMENT | PK |
| forumid | INT UNSIGNED | FK to forum_forums.id |
| userid | INT UNSIGNED | Creator |
| subject | VARCHAR(40) | |
| locked | ENUM('yes','no') | |
| sticky | ENUM('yes','no') | |
| moved | ENUM('yes','no') | |
| views | INT | |
| lastpost | INT UNSIGNED | FK to forum_posts.id |

#### forum_posts

| Column | Type | Notes |
|---|---|---|
| id | INT UNSIGNED AUTO_INCREMENT | PK |
| topicid | INT UNSIGNED | FK to forum_topics.id |
| userid | INT UNSIGNED | Author |
| added | DATETIME | |
| body | LONGTEXT | FULLTEXT indexed |
| editedby | INT UNSIGNED | FK to users.id |
| editedat | DATETIME | |

### 1.3 Entity Relationship Summary

```
users 1--N torrents (owner)
users 1--N peers (userid)
users 1--N completed (userid)
users 1--N comments (user)
users 1--N ratings (user)
users 1--N reports (addedby)
users 1--N messages (sender/receiver)
users 1--N forum_topics (userid)
users 1--N forum_posts (userid)
users 1--N shoutbox (userid)
users 1--N warnings (userid/warnedby)
users 1--N pollanswers (userid)
users N--1 groups (class -> group_id)
users N--1 countries (country)
users N--1 stylesheets (stylesheet)
users N--1 teams (team)
users 1--1 users (invited_by, self-referencing)

torrents 1--N files (torrent)
torrents 1--N peers (torrent)
torrents 1--N completed (torrentid)
torrents 1--N comments (torrent)
torrents 1--N ratings (torrent)
torrents 1--N announce (torrent)
torrents N--1 categories (category)
torrents N--1 torrentlang (torrentlang)

categories: parent_cat (string) groups subcategories under parents

forum_forums N--1 forumcats (category)
forum_topics N--1 forum_forums (forumid)
forum_posts N--1 forum_topics (topicid)
forum_readposts: (userid, topicid) -> lastpostread

polls 1--N pollanswers (pollid)
news 1--N comments (news)
```

### 1.4 Seed Data Summary

| Table | Records | Content |
|---|---|---|
| groups | 7 | Member through Administrator |
| categories | 46 | Movies, TV, Games, Apps, Music, Anime, Other + subcategories |
| countries | 101 | Countries with flag images |
| torrentlang | 7 | EN, FR, DE, IT, JA, ES, RU |
| faq | 65 | 9 categories + 56 items |
| rules | 3 | General, Forum, Moderating |
| blocks | 20 | Sidebar widgets |
| polls | 1 | Sample poll |
| news | 1 | Welcome message |
| stylesheets | 1 | Default theme |
| languages | 1 | English |
| censor | 1 | Sample word filter |

---

## 2. Tracker / Announce System

### 2.1 Announce Endpoint

**Endpoint**: `GET /announce.php?passkey={passkey}&info_hash={hash}&peer_id={id}&port={port}&uploaded={bytes}&downloaded={bytes}&left={bytes}&event={event}`

#### Complete Flow

```
1. INITIALIZATION
   |-> Connect to database
   |-> Load site config
   |-> Block web browsers (check User-Agent for Mozilla/Opera/Links/Lynx)

2. INPUT PARSING & VALIDATION
   |-> Extract GET parameters: passkey, info_hash, peer_id, ip, event, port,
   |   downloaded, uploaded, left, no_peer_id, num_want
   |-> Validate required params present (passkey, info_hash, peer_id, port,
   |   downloaded, uploaded, left)
   |-> Validate peer_id is exactly 20 bytes
   |-> Validate info_hash: if 20 bytes -> convert to 40-char hex; if 40 -> accept; else error
   |-> If MEMBERSONLY: validate passkey is exactly 32 chars
   |-> Override IP with REMOTE_ADDR (ignore client-supplied IP)
   |-> Validate port: > 0 and <= 65535
   |-> Check port blacklist (411-413, 1214, 4662, 6346-6347, 6699)

3. CLIENT CLASSIFICATION
   |-> Determine seeder/leecher: left == 0 -> seeder, else leecher
   |-> Extract client agent from first 8 chars of peer_id
   |-> Check against BANNED_AGENTS list (default: "-AZ21, -BC, LIME")

4. USER AUTHENTICATION (if MEMBERSONLY)
   |-> Query: SELECT u.id, u.class, u.uploaded, u.downloaded, u.ip, u.passkey,
   |          g.can_download FROM users u JOIN groups g ON u.class = g.group_id
   |          WHERE u.passkey = ? AND u.enabled = 'yes' AND u.status = 'confirmed'
   |-> If not found: error "Cannot locate a user with that passkey!"
   |-> If can_download = 'no': error "You do not have permission to download"

5. TORRENT LOOKUP
   |-> Query: SELECT id, info_hash, banned, freeleech, seeders + leechers AS numpeers,
   |          UNIX_TIMESTAMP(added) AS ts, seeders, leechers, times_completed
   |          FROM torrents WHERE info_hash = ?
   |-> If not found: error "Torrent not found on this tracker"
   |-> If banned = 'yes': error "Torrent has been banned"

6. FETCH PEER LIST (for response)
   |-> If numpeers > 50: SELECT ... ORDER BY RAND() LIMIT 50
   |-> Else: SELECT all peers for this torrent
   |-> Build response peer list (exclude self)
   |-> Store self's record if found (for delta calculation)

7. BUILD RESPONSE
   |-> Bencode dict with:
   |   - complete: seeder count
   |   - downloaded: times_completed
   |   - incomplete: leecher count
   |   - interval: 900 seconds (15 min)
   |   - min interval: 300 seconds (5 min)
   |   - peers: list of {ip, peer id, port} dicts

8. CONNECTION LIMIT CHECK (new peers only, MEMBERSONLY)
   |-> Count peers with same passkey where seeder='no'
   |   If >= 1: error "Connection limit exceeded! You may only leech from one location"
   |-> Count peers with same passkey where seeder='yes'
   |   If >= 3: error "Connection limit exceeded!"

9. WAIT TIME CHECK (new leechers only, MEMBERSONLY + MEMBERSONLY_WAIT)
   |-> Only applies to user classes in WAIT_CLASS list (default: '1,2')
   |-> Calculate user ratio and upload GB
   |-> Tier-based wait from torrent added time:
   |   - ratio == 0 AND gigs == 0        -> 24h wait
   |   - ratio < 0.50 OR gigs < 1 GB     -> 24h wait
   |   - ratio < 0.65 OR gigs < 3 GB     -> 12h wait
   |   - ratio < 0.80 OR gigs < 5 GB     -> 6h wait
   |   - ratio < 0.95 OR gigs < 7 GB     -> 2h wait
   |   - else                             -> 0h wait

10. CONNECTABILITY CHECK (new peers only)
    |-> Attempt TCP connection to peer's ip:port (5s timeout)
    |-> Record connectable = 'yes' or 'no' (informational, doesn't block)

11. STATS UPDATE (existing peers only, if upload or download delta > 0)
    |-> Calculate deltas: upthis = max(0, uploaded - old_uploaded)
    |                     downthis = max(0, downloaded - old_downloaded)
    |-> If freeleech: UPDATE users SET uploaded += upthis (download NOT counted)
    |-> Else: UPDATE users SET uploaded += upthis, downloaded += downthis

12. EVENT HANDLING
    |
    |-> EVENT: "stopped"
    |   |-> DELETE FROM peers WHERE torrent = ? AND peer_id = ?
    |   |-> Decrement seeders or leechers count (with floor at 0)
    |
    |-> EVENT: "completed"
    |   |-> INCREMENT torrents.times_completed
    |   |-> INSERT INTO completed (userid, torrentid, date)
    |
    |-> EVENT: "started" or absent
    |   |-> If existing peer: UPDATE peers SET ip, port, uploaded, downloaded,
    |   |   to_go, last_action, client, seeder
    |   |   -> Detect seeder/leecher status change, adjust counters
    |   |-> If new peer: INSERT INTO peers
    |   |   -> Increment seeders or leechers count

13. TORRENT VISIBILITY UPDATE
    |-> If seeder and not banned: SET visible = 'yes', last_action = NOW()

14. FLUSH ALL TORRENT TABLE UPDATES
    |-> Single UPDATE torrents SET ... WHERE id = ?

15. SEND RESPONSE
    |-> Output bencoded response dict
    |-> Close DB connection
```

#### Key Business Rules - Announce

- **Freeleech**: Downloads NOT counted against user ratio; uploads always count
- **Connection limits**: 1 concurrent leech location, 3 concurrent seed locations (per passkey)
- **Peer response limit**: 50 peers max per announce (random selection if more)
- **Negative prevention**: All counter decrements use `CASE WHEN val < 1 THEN 0 ELSE val - 1 END`
- **IP enforcement**: Client-supplied IP always ignored; REMOTE_ADDR used
- **Seeder status transitions**: Tracked and counters adjusted bidirectionally

### 2.2 Scrape Endpoint

**Endpoint**: `GET /scrape.php?info_hash={hash}` (supports multiple `info_hash` params)

#### Flow

```
1. Parse raw query string for info_hash parameters
2. For each hash: if 20 bytes -> hex encode; if 40 -> accept; else skip
3. Query: SELECT info_hash, seeders, leechers, times_completed, filename
          FROM torrents WHERE info_hash IN (...)
4. Return bencoded response:
   d5:filesd
     <20-byte-binary-hash>d
       8:completei<seeders>e
       10:downloadedi<times_completed>e
       10:incompletei<leechers>e
       4:name<len>:<filename>
     e
   ee
```

### 2.3 Torrent Download (Passkey Injection)

**Endpoint**: `GET /download.php?id={torrent_id}&passkey={passkey}`

#### Flow

```
1. Authenticate user via passkey or session cookies
2. Check permissions (MEMBERSONLY, can_download)
3. Look up torrent: SELECT filename, banned, external, announce FROM torrents WHERE id = ?
4. Validate: torrent exists, not banned, file exists and readable
5. Increment hit counter: UPDATE torrents SET hits = hits + 1
6. Generate passkey if user doesn't have one:
   passkey = md5(username + microtime + secret + random)
7. If LOCAL torrent + MEMBERSONLY:
   a. BDecode the .torrent file
   b. Replace announce URL with: PASSKEYURL formatted with user's passkey
   c. Remove announce-list (multi-tracker)
   d. BEncode and send modified torrent
8. If EXTERNAL torrent:
   a. Stream original file unmodified
9. Response headers:
   Content-Disposition: attachment; filename="<name>[<domain>].torrent"
   Content-Type: application/x-bittorrent
```

### 2.4 BEncode/BDecode

**Supported types**:
- Integers: `i<num>e`
- Strings: `<length>:<data>`
- Lists: `l<items>e`
- Dicts: `d<key><value>...e` (keys sorted alphabetically per spec)

---

## 3. Torrent Management

### 3.1 Upload

**Endpoint**: `POST /torrents-upload.php`

#### Flow

```
1. PERMISSION CHECK
   |-> Must be logged in
   |-> can_upload = 'yes'
   |-> If UPLOADERSONLY: class >= 4

2. TORRENT FILE VALIDATION
   |-> Must have .torrent extension
   |-> Parse with BDecode: extract announce, info_hash, name, size, file list, etc.
   |-> Check announce URL against configured tracker URLs
   |-> If external tracker and ALLOWEXTERNAL = false: reject

3. FORM DATA
   |-> Name (optional, defaults to internal torrent name with underscores->spaces)
   |-> Description (BBCode)
   |-> Category (required)
   |-> Language (optional, defaults to 0)
   |-> Anonymous flag (optional, if ANONYMOUSUPLOAD enabled)

4. NFO UPLOAD (optional)
   |-> Max 65,535 bytes
   |-> Must match *.nfo pattern
   |-> Stored as {torrent_id}.nfo

5. IMAGE UPLOAD (optional, up to 2)
   |-> Max size from config
   |-> Must be valid image (verified via getimagesize)
   |-> Allowed types: GIF, JPEG, PNG, WEBP

6. DUPLICATE CHECK
   |-> info_hash must be unique (DB constraint)

7. DATABASE INSERT
   |-> torrents: all metadata fields
   |-> files: one row per file in torrent (path, size)
   |-> announce: one row per announce URL (excluding UDP)

8. EXTERNAL TORRENT SCRAPE (if external + UPLOADSCRAPE)
   |-> Convert /announce to /scrape URL
   |-> Fetch live stats (seeders, leechers, completed)
   |-> Update torrent record

9. FILE STORAGE
   |-> .torrent file: {torrent_dir}/{id}.torrent
   |-> NFO file: {nfo_dir}/{id}.nfo
   |-> Images: {torrent_dir}/images/{id}{n}{ext}
```

### 3.2 Browse / Listing

**Endpoint**: `GET /torrents.php?cat={id}&parent_cat={name}&sort={field}&type={asc|desc}`

- Displays torrents with visible='yes' only
- Filters: category, parent category, multiple categories via checkboxes
- Sorting: name, times_completed, seeders, leechers, comments, size, id (default DESC)
- 20 torrents per page with pagination
- Shows: name, category, size, seeders, leechers, completed, comments, rating, uploader, freeleech/external flags

### 3.3 Detail Page

**Endpoint**: `GET /torrents-details.php?id={id}&hit=1`

- Core info: name, description, category, language, size, info_hash, uploader, dates, stats
- Download button with health indicator
- Peer list table (local torrents): port, uploaded, downloaded, ratio, completion %, client, username
- File list: all files with sizes
- Rating system: 1-5 stars, one vote per user, average shown if >= 2 ratings
- Comments section: paginated at 10/page, BBCode support
- NFO viewer (if present)
- External torrents: "Update Stats" scrapes external trackers live
- Reseed request link (if seeders <= 1)
- "Who's completed" link

### 3.4 Search

**Endpoint**: `GET /torrents-search.php`

- Full-text search on torrent name (MySQL MATCH...AGAINST BOOLEAN MODE)
- Fallback to LIKE search if full-text returns nothing
- Max 5 search terms, min 2 chars each
- Filters: categories (multi-select), language, visibility (active/dead/all), freeleech, external
- 20 results per page

### 3.5 Edit Torrent

**Endpoint**: `POST /torrents-edit.php?id={id}`

- Requires: owner OR edit_torrents='yes'
- Editable by owner: name, description, category, language, visible, anonymous
- Editable by staff only: banned, freeleech
- Image management: keep/delete/replace per image
- NFO management: keep/update

### 3.6 Delete Torrent

- Requires: edit_torrents='yes' (staff) or owner
- Reason required
- Deletes: DB record, .torrent file, images
- If deleter != owner: sends PM to owner with reason
- Logged to site log

### 3.7 Torrent Comments

**Endpoint**: `GET/POST /comments.php`

- Types: torrent comments, news comments
- Add: logged in, non-empty body
- Edit: author OR edit_torrents/edit_news permission
- Delete: delete_torrents/delete_news permission
- Increments/decrements torrents.comments counter
- All actions logged

### 3.8 Ratings

- Scale: 1-5 (Sucks, Pretty Bad, Decent, Pretty Good, Cool)
- One vote per user per torrent (UNIQUE constraint on torrent+user)
- Average displayed only if numratings >= 2

### 3.9 Reporting

**Endpoint**: `POST /report.php`

- Types: user, torrent, comment, forum
- Requires reason
- One report per user per item (duplicate prevention)
- Stored in reports table for mod review

### 3.10 NFO View/Edit

- View: displays in textarea with CP437 -> HTML entity translation
- Edit: staff only (edit_torrents='yes'), can update or delete, reason required for delete

### 3.11 Reseed Request

**Endpoint**: `POST /torrents-reseed.php?id={id}`

- Local torrents only, not banned
- Rate limited: 1 request per torrent per 24h (cookie-based)
- Sends PM to all users in completed table for this torrent
- Also PMs torrent owner

### 3.12 Torrent Import

**Endpoint**: `POST /torrents-import.php`

- Staff only (edit_torrents='yes')
- Batch imports .torrent files from 'import' directory
- Same validation as single upload
- Deletes successfully imported files

### 3.13 RSS Feed

**Endpoint**: `GET /rss.php?cat={ids}&passkey={key}&dllink={0|1}&incldead={0|1}`

- RSS 2.0 XML format
- Last 50 torrents
- Filters: category, uploader, include dead
- Link type: details page or direct download
- Passkey authentication for download links

### 3.14 Today's Torrents

**Endpoint**: `GET /torrents-today.php`

- Torrents added in last 24 hours
- Grouped by category, max 10 per category

### 3.15 Completed List

**Endpoint**: `GET /torrents-completed.php?id={id}`

- Local torrents only
- Shows users who completed the torrent
- Indicates if user is currently seeding
- Respects privacy settings

### 3.16 Need Seed List

**Endpoint**: `GET /torrents-needseed.php`

- Criteria: not banned, leechers > 0, seeders <= 1, not external
- Ordered by seeders ASC

---

## 4. User & Authentication System

### 4.1 Registration

**Endpoint**: `POST /account-signup.php`

#### Flow

```
1. INPUT VALIDATION
   |-> Username: max 15 chars, alphanumeric only (ctype_alnum)
   |-> Password: 6-40 chars, cannot match username
   |-> Email: valid format, not banned domain, not already in use
   |-> Optional: age, country, gender, client

2. INVITE CHECK (if invite params present)
   |-> Validate invite ID + MD5(secret) match
   |-> Skip email requirement for invited users

3. FIRST USER PRIVILEGE
   |-> If user count == 0: assign class 7 (Administrator)
   |-> Otherwise: assign class 1 (Member)

4. ACCOUNT CREATION
   |-> Generate random 20-char secret
   |-> Hash password via passhash() (configurable: SHA1/MD5/HMAC)
   |-> Insert into users table
   |-> Default theme and language assigned

5. EMAIL CONFIRMATION (conditional)
   |-> If CONFIRMEMAIL: status='pending', send confirmation email
   |-> If ACONFIRM: admin approval required after confirmation
   |-> If neither: status='confirmed' immediately
   |-> Invited users: always confirmed immediately

6. WELCOME PM (if WELCOMEPMON)
   |-> System sends configured welcome message
```

### 4.2 Login

**Endpoint**: `POST /account-login.php`

#### Flow

```
1. Query user by username
2. Hash submitted password with passhash()
3. Compare with stored hash
4. Check: password matches AND status='confirmed' AND enabled='yes'
5. Set cookies:
   - uid: numeric user ID
   - pass: SHA1(id + secret + password_hash + IP + secret)
   - Expiry: 30 days, HttpOnly, Secure
6. Optional "returnto" redirect
```

### 4.3 Per-Request Authentication

```
1. Extract cookies: uid, pass
2. Validate: pass length == 40, uid is numeric
3. Query: SELECT * FROM users JOIN groups WHERE id = uid AND enabled = 'yes'
4. Recalculate: SHA1(id + secret + password + current_IP + secret)
5. Compare with cookie pass value
6. If match: set $CURUSER global, update last_access + current page
7. If no match: clear cookies, user is unauthenticated
```

**Key**: IP is part of the cookie hash. Changing IP invalidates the session.

### 4.4 Password Recovery

**Endpoint**: `GET/POST /account-recover.php`

```
Stage 1: User submits email
  -> Look up user by email
  -> Generate new secret, store MD5(secret) in DB
  -> Send email with link: account-recover.php?id=USER_ID&secret=MD5(secret)

Stage 2: User clicks link
  -> Validate id + secret match
  -> User enters new password (min 6 chars)
  -> Update password hash and generate new secret
```

### 4.5 Email Confirmation

**Endpoint**: `GET /account-confirm.php?id={id}&secret={hash}`

- Validates user exists with status='pending'
- Validates MD5(secret) matches
- Sets status='confirmed', generates new secret

### 4.6 Email Change

**Endpoint**: `GET /account-ce.php?id={id}&secret={hash}&email={email}`

- Triggered from profile settings
- Uses separate editsecret field
- Hash = MD5(editsecret + email + editsecret)
- Updates email, clears editsecret

### 4.7 Profile & Settings

**Endpoint**: `GET/POST /account.php`

**Viewable profile info**: stats, ratio, activity, torrents
**Editable settings**: email (triggers confirmation), theme, language, client, age, gender, country, team, avatar URL, custom title, signature (max 150 chars), privacy level, accept PMs, email notifications, passkey reset, timezone, shoutbox visibility

### 4.8 User Roles & Permissions

See groups table in Section 1.2. Permission checks are done by reading the user's group record and checking the specific ENUM field (e.g., `$CURUSER["edit_torrents"] == "yes"`).

### 4.9 Passkey System

- 32-character hex string
- Used in announce URL for private tracker authentication
- Generated: `md5(username + microtime + secret + random)`
- User can reset via settings; staff can reset via admin
- Distinct from session cookies (used only by BitTorrent client)

### 4.10 Ratio Tracking

- `users.uploaded` / `users.downloaded` (both BIGINT, bytes)
- Updated by announce.php on each peer announce (delta calculation)
- Display: if downloaded > 0 -> uploaded/downloaded; else "---" (infinite)
- Staff can manually adjust both values

---

## 5. Invitation System

### 5.1 Invite Flow

**Endpoint**: `POST /invite.php`

```
1. PREREQUISITES
   |-> INVITEONLY or ENABLEINVITES must be enabled
   |-> User must have invites > 0
   |-> Active user count < maxusers_invites

2. SEND INVITE
   |-> Validate email: valid format, not banned, not in use
   |-> Create dummy user record:
   |   - username: "invite_" + random 20-char secret
   |   - status: 'pending'
   |   - invited_by: current user ID
   |-> Decrement inviter's invite count
   |-> Add invited user ID to inviter's invitees field
   |-> Send email with link: account-signup.php?invite=ID&secret=MD5(secret)

3. REDEEM INVITE (at signup)
   |-> Email pre-filled and hidden
   |-> User enters username, password, profile info
   |-> Updates dummy account with real credentials
   |-> Account immediately confirmed (no email confirmation)
```

### 5.2 Auto-Invite Distribution (Cleanup Task)

- Runs periodically during cleanup
- Criteria: user downloaded GB within range AND ratio above threshold
- Max invites per class:
  - Member: 5, Power User: 10, VIP: 20, Uploader: 25
  - Moderator: 100, Super Mod: 100, Admin: 400

---

## 6. Forum System

### 6.1 Structure

```
Forum Categories (forumcats)
  -> Forums (forum_forums)
    -> Topics (forum_topics)
      -> Posts (forum_posts)
```

### 6.2 Endpoints & Actions

**Endpoint**: `GET/POST /forums.php?action={action}`

| Action | Method | Permission | Description |
|---|---|---|---|
| (default) | GET | view_forum | Forum index with all forums, stats, top posters |
| viewforum | GET | minclassread | Topic listing, 20/page, sticky first |
| viewtopic | GET | minclassread | Post listing, 20/page, increments views |
| newtopic | POST | minclasswrite | Create topic (subject max 50 chars + body) |
| post | POST | minclasswrite | Reply to topic (checks locked status) |
| editpost | POST | author OR edit_forum | Edit post body, tracks editedby/editedat |
| deletepost | POST | delete_forum | Delete single post (not if only post in topic) |
| deletetopic | POST | delete_forum | Delete topic + all posts + readposts |
| locktopic | GET | delete_forum + edit_forum | Set locked='yes' |
| unlocktopic | GET | delete_forum + edit_forum | Set locked='no' |
| setsticky | GET | edit_forum | Set sticky='yes' |
| unsetsticky | GET | edit_forum | Set sticky='no' |
| movetopic | POST | delete_forum + edit_forum | Move to different forum |
| renametopic | POST | delete_forum + edit_forum | Change subject |
| viewunread | GET | logged in | Up to 25 topics with unread posts |
| search | GET | logged in | Full-text search on post body, up to 50 results |
| catchup | GET | logged in | Mark all forums read |

### 6.3 Read Tracking

- `forum_readposts` table: (userid, topicid, lastpostread)
- Updated when user views a topic
- Used to show new/unread indicators (folder_new vs folder icons)
- "Catch up" marks all topics as read

### 6.4 Forum Permissions

| Permission | Controls |
|---|---|
| minclassread | Minimum user class to view forum |
| minclasswrite | Minimum user class to post |
| guest_read | Allow unauthenticated viewing |
| view_forum (group) | Global forum access |
| edit_forum (group) | Rename, sticky, lock, move |
| delete_forum (group) | Delete posts/topics |
| forumbanned (user) | Per-user forum ban |

### 6.5 Forum Features

- Sticky topics (appear first)
- Locked topics (no new replies)
- Moved topics (redirects)
- Post editing with edit history (editedby, editedat)
- Top 10 posters sidebar
- Quick jump dropdown to any accessible forum
- BBCode + smilies in posts
- User info per post: avatar, stats, ratio, title, signature

---

## 7. Shoutbox (Chat)

### 7.1 Endpoint

**Endpoint**: `GET/POST /shoutbox.php`

### 7.2 Features

| Feature | Detail |
|---|---|
| Post message | Requires login, stores username + userid + message + date |
| Display | Last 20 messages, newest first |
| Delete | Author or edit_users permission; logged |
| Duplicate prevention | Same message from same user within 30 seconds rejected |
| Auto-refresh | Page refreshes every 300 seconds |
| History | Full paginated history (100/page) at `?history=1` |
| Formatting | BBCode + smilies via format_comment() |
| User preference | Can hide shoutbox (hideshoutbox='yes') |

### 7.3 Integration

- Embedded as iframe on homepage (index.php)
- Refresh interval hardcoded at 300 seconds
- Styling via theme CSS

---

## 8. Private Messaging

### 8.1 Endpoints

**Endpoint**: `GET/POST /mailbox.php?action={action}`

### 8.2 Message Locations

| Location | Description |
|---|---|
| `in` | Inbox (received) |
| `out` | Outbox (sent copy) |
| `both` | In both inbox and outbox |
| `draft` | Unsent draft |
| `template` | Reusable template |

### 8.3 Actions

| Action | Description |
|---|---|
| (default) | Overview with counts per folder |
| inbox | Received messages, 20/page, sortable |
| outbox | Sent messages, 20/page |
| drafts | Unsent drafts |
| templates | Saved templates |
| compose | New message form |
| send | Send message to user |
| read | Expand message inline |
| delete | Bulk delete selected messages |
| markread | Bulk mark as read |

### 8.4 Compose & Send Flow

```
1. Select recipient (dropdown of enabled, confirmed users)
   |-> Respects acceptpms: can only PM users with acceptpms='yes' unless sender is staff
   |-> Optionally load template
2. Enter subject + body (BBCode editor)
3. Options: Send, Save Draft, Save Template, Save Copy to Outbox
4. On send:
   |-> Validate recipient exists, enabled, confirmed
   |-> Insert message with location='in' (and 'out' or 'both' if save copy)
   |-> Delete draft if sending from draft
   |-> Email notification if recipient has '[pm]' in notifs field
```

### 8.5 Deletion Logic

- Inbox delete: hard delete 'in'; change 'both' -> 'out'
- Outbox delete: hard delete 'out'; change 'both' -> 'in'
- Draft/template delete: hard delete

### 8.6 Reply

- Pre-fills subject with "Re: {original}"
- Pre-fills body with quoted original
- Auto-selects original sender as recipient
- Marks original as read

---

## 9. Admin Control Panel

### 9.1 Access

Requires `control_panel = 'yes'` in user's group permissions.

**Endpoint**: `GET/POST /admincp.php`

### 9.2 Sections & Features

#### User Management
- Advanced user search (multiple criteria)
- Simple user search (username, email, IP) with delete
- View pending/confirmed invited users
- View warned users
- Manual registration confirmation
- Privacy level filtering
- Edit user details (via admin-modtasks.php):
  - Class/rank, title, uploaded/downloaded, avatar, signature, IP, donated, invites, passkey, modcomment, enabled, warned, forumbanned
  - Promotion/demotion notifications sent to user
  - Cannot demote same-rank or higher users

#### Warning System
- Add warnings with reason, expiry, type ("Poor Ratio" or custom)
- Track warning history
- Auto-warnings for low ratio (configurable)
- Auto-removal when ratio improves
- Auto-ban if ratio not improved within warning period

#### User Groups / Roles
- Create/edit/delete groups
- Configure all 18 permission fields per group

#### Torrent Management
- Search/view/edit/delete torrents
- Ban/unban torrents
- View all freeleech torrents
- View all banned torrents

#### News Management
- Add/edit/delete news articles
- BBCode support
- Deleting news also deletes associated comments

#### FAQ Management
- Create/edit/delete FAQ categories and items
- Item flags: Hidden(0), Normal(1), Updated(2), New(3)
- Reorder categories and items

#### Polls
- Create polls with up to 20 options
- Edit/delete polls
- View results with voter details

#### Site Content
- Rules management (role-gated visibility)
- Word censor (word -> replacement pairs)
- Block/widget system management

#### Security & Moderation
- IP bans (single or range, IPv4/IPv6)
- Email bans (address or domain)
- Reports management (filter by type, mark complete)

#### Communication
- Mass PM to user groups
- Message spy (view all PMs)

#### Monitoring
- Peers list (all active peers)
- Cheater detection (suspicious upload patterns)
- Latest comments
- Users online / who's where
- Avatar log
- Site activity log (searchable)
- SQL error log

#### System
- Database backup (.sql and .sql.gz)
- Force cleanup trigger
- Theme management (add/delete)
- Category management (create/edit/delete with parent hierarchy)
- Language management (add/edit/delete)

#### Site Settings
- All configuration values editable via admin panel

---

## 10. Cleanup / Cron Tasks

**File**: `backend/cleanup.php` (triggered periodically, configurable interval)

### 10.1 Tasks Performed

| Task | Description |
|---|---|
| Peer cleanup | Delete inactive peers (older than announce_interval) |
| Stats recalculation | Recount seeders/leechers from peers table |
| Comment count | Recount comments per torrent |
| Dead torrent hiding | Set visible='no' for torrents with no activity > max_dead_torrent_time |
| Pending account cleanup | Delete unconfirmed registrations > signup_timeout |
| Log pruning | Delete log entries > LOGCLEAN |
| Ratio warnings | Auto-warn users with low ratio + high download |
| Warning removal | Remove warnings when ratio improves |
| Auto-ban | Ban users who don't improve within warning period |
| Auto-invites | Distribute invites based on download/ratio criteria |
| DB optimization | REPAIR, ANALYZE, OPTIMIZE on all tables |

### 10.2 Ratio Warning Thresholds

| Setting | Default |
|---|---|
| ratiowarn_enable | true |
| ratiowarn_minratio | 0.4 |
| ratiowarn_mingigs | 4 GB (minimum downloaded to trigger) |
| ratiowarn_daystowarn | 14 days to improve before ban |

---

## 11. Blocks / Widget System

### 11.1 Architecture

- Blocks stored in `blocks` table with position (left/middle/right), sort order, enabled flag
- Block PHP files in `/blocks/` directory
- Each block is a self-contained PHP include

### 11.2 Available Blocks (20 total)

| Block | Position | Description |
|---|---|---|
| login_block | left | Login form |
| navigate_block | left | Navigation links |
| simplesearch_block | left | Quick search |
| advancesearch_block | left | Advanced search |
| invite_block | left | Invite form |
| donate_block | left | Donation info |
| maincats_block | middle | Category navigation |
| latestuploads_block | middle | Recent torrents |
| latestimages_block | middle | Recent torrent images |
| mostactivetorrents_block | middle | Most active torrents |
| scrollingnews_block | middle | Scrolling news ticker |
| polls_block | middle | Active poll |
| rss_block | right | RSS feed links |
| usersonline_block | right | Online users |
| newestmember_block | right | Newest member |
| advancestats_block | right | Advanced statistics |
| serverload_block | right | Server load info |
| seedwanted_block | right | Torrents needing seed |
| themelang_block | right | Theme/language switcher |
| poweredby_block | right | Attribution |

### 11.3 Admin Block Management

- Add new blocks from files
- Upload block files
- Enable/disable blocks
- Set position (left/middle/right)
- Set sort order
- Delete blocks
- Preview before enabling

---

## 12. Miscellaneous Systems

### 12.1 Teams

**Endpoints**: `/teams-create.php` (admin), `/teams-view.php` (public)

- Admin creates teams with name, owner, description, logo URL
- Users join teams via profile settings (class > 1 required)
- Team view shows all members with upload/download stats
- Teams are social/organizational, no permission implications

### 12.2 Theme System

- Database-driven theme registry (stylesheets table)
- Each theme is a directory under `/themes/`
- Contains: theme.css, header.php, footer.php, block.php
- Users select theme via settings or theme switcher block
- Admin can add/delete themes

### 12.3 Multi-Language Support

- Translation functions: `T_($string)` for single, `P_($string, $count)` for plural
- Language files in `/languages/` directory
- GNU gettext plural forms standard
- 200+ translation keys
- User selects language via settings

### 12.4 Caching System (TTCache class)

| Backend | Description |
|---|---|
| Memcache | Distributed memory cache with host/port config |
| APC | In-process PHP opcode cache |
| XCache | Alternative PHP opcode cache |
| Disk | File-based fallback |

- Methods: Set(key, value, ttl), Get(key, ttl), Delete(key)
- Query caching: `get_row_count_cached()`, `SQL_Query_exec_cached()` (300s default TTL)
- Automatic SHA1 hashing of queries for cache keys

### 12.5 Email System (TTMail class)

- Backends: PEAR Mail (SMTP with auth) or PHP mail()
- SMTP config: host, port, SSL, auth, user, password
- Used for: signup confirmation, password recovery, invite emails, PM notifications

### 12.6 Input Parsing

**ParseTorrent() function**:
- Reads .torrent file with BDecode
- Extracts: announce URL, info_hash (SHA1), creation date, name, total size, file count, announce list, comment, file list
- Returns structured array

### 12.7 BBCode

**Supported tags**: `[b]`, `[i]`, `[u]`, `[color=]`, `[size=]`, `[font=]`, `[url]`, `[url=]`, `[img]`, `[quote]`, `[quote=]`, `[*]`, `[spoiler]`, `[spoiler=]`

**Editor toolbar**: Bold, Italic, Underline, List, Quote, URL, Image buttons + smiley picker (26 smilies)

### 12.8 Smilies

26 emoticons mapped from text codes to PNG images:
`:)`, `:(`, `;)`, `:P`, `:D`, `:|`, `:O`, `:?`, `8)`, `8o`, `B)`, `:-)`, `:-(`, `:-*`, `O:-D`, `:-@`, `:o)`, `:help`, `:love`, `:warn`, `:bomb`, `:idea`, `:bad`, `:!`, `brb`

### 12.9 Word Censor

- Database-driven word replacement (censor table)
- Alternative: file-based censor (censor.txt)
- Applied to user-generated content

### 12.10 Polls

- Up to 20 options per poll
- One vote per user per poll
- Results visible with voter breakdown
- Admin creates/edits/deletes polls

### 12.11 FAQ

- Two-level hierarchy: categories and items
- Flag-based visibility: Hidden(0), Normal(1), Updated(2), New(3)
- Table of contents with anchor links
- Admin manages via faq-manage.php

### 12.12 Rules

- Database-driven rules with BBCode formatting
- Visibility control: public or class-gated
- Admin manages sections via admincp

### 12.13 Staff Page

- Shows users with staff_page='yes' in their group
- Public sees only staff_public='yes' entries
- Online/offline indicator (180-second threshold)
- PM button per staff member

### 12.14 Member List

- Requires view_users='yes'
- Shows confirmed users only
- Filters: search by username (LIKE), filter by letter (A-Z), filter by class
- 25 users per page
- Columns: username, registered, last access, class, country, donation indicator

### 12.15 Server Load Monitoring

- Windows-only (WMI COM interface)
- Displays: OS info, CPU count, per-CPU usage, total CPU, RAM usage, uptime

### 12.16 System Check (/check.php)

Validates:
- PHP version >= 7.2.0
- Required extensions: zlib, XML, MySQLi, curl, openSSL, gmp, bcmath
- File/directory permissions
- Database connectivity
- Table existence
- Default theme/language validity
- SQL strict mode status

---

## 13. Configuration Reference

### 13.1 Constants

```
KB = 1024, MB = 1048576, GB = 1073741824
MINUTE = 60, HOUR = 3600, DAY = 86400, WEEK = 604800
```

### 13.2 Core Settings

| Setting | Default | Description |
|---|---|---|
| SITENAME | 'Testing' | Display name |
| SITEEMAIL | (configure) | Sender email |
| SITEURL | (configure) | Base URL |
| SITE_ONLINE | true | Site enabled |
| OFFLINEMSG | 'Site is down...' | Offline message |
| CHARSET | 'utf-8' | HTML charset |

### 13.3 Access Control

| Setting | Default | Description |
|---|---|---|
| MEMBERSONLY | true | Require login to browse |
| MEMBERSONLY_WAIT | true | Wait times for bad ratio |
| INVITEONLY | false | Invite-only signup |
| ENABLEINVITES | true | Allow invites |
| CONFIRMEMAIL | false | Email confirmation required |
| ACONFIRM | false | Admin confirmation required |
| UPLOADERSONLY | false | Restrict uploads to class >= 4 |
| ALLOWEXTERNAL | true | Allow external torrents |
| ANONYMOUSUPLOAD | false | Allow anonymous uploads |
| maxusers | 20000 | Max accounts |
| maxusers_invites | 25000 | Max accounts (invite mode) |

### 13.4 Tracker Settings

| Setting | Default | Description |
|---|---|---|
| announce_interval | 900s (15 min) | Peer announce interval |
| min_interval | 300s (5 min) | Minimum announce interval |
| PEERLIMIT | 10000 | Config value (actual limit is hardcoded 50) |
| BANNED_AGENTS | "-AZ21, -BC, LIME" | Blocked clients |
| PASSKEYURL | {SITEURL}/announce.php?passkey=%s | Passkey URL template |

### 13.5 Wait Time Tiers

| Tier | Min Ratio | Min GB | Wait |
|---|---|---|---|
| A | 0.50 | 1 | 24 hours |
| B | 0.65 | 3 | 12 hours |
| C | 0.80 | 5 | 6 hours |
| D | 0.95 | 7 | 2 hours |
| E | (above) | (above) | 0 hours |

Applied to user classes in WAIT_CLASS (default: '1,2').

### 13.6 Cleanup & Maintenance

| Setting | Default | Description |
|---|---|---|
| autoclean_interval | 600s (10 min) | Cleanup frequency |
| max_dead_torrent_time | 21600s (6 hr) | Hide inactive torrents |
| signup_timeout | 259200s (3 days) | Delete pending signups |
| LOGCLEAN | 2419200s (28 days) | Log retention |

### 13.7 Ratio Warning

| Setting | Default | Description |
|---|---|---|
| ratiowarn_enable | true | Enable auto-warnings |
| ratiowarn_minratio | 0.4 | Minimum acceptable ratio |
| ratiowarn_mingigs | 4 GB | Minimum downloaded to trigger |
| ratiowarn_daystowarn | 14 | Days before auto-ban |

### 13.8 Password Hashing

| Setting | Default | Description |
|---|---|---|
| passhash_method | 'sha1' | Hash method (sha1/md5/hmac) |
| passhash_algorithm | 'sha1' | HMAC algorithm |
| passhash_salt | '' | HMAC salt (should be 20+ random chars) |

> **Warning**: Changing these after deployment requires all users to reset passwords.

### 13.9 Cache

| Setting | Default | Description |
|---|---|---|
| cache_type | 'disk' | Backend: disk/memcache/apc/xcache |
| cache_dir | {cwd}/cache | Disk cache path |
| cache_memcache_host | 'localhost' | Memcache host |
| cache_memcache_port | 11211 | Memcache port |

### 13.10 Mail

| Setting | Default | Description |
|---|---|---|
| mail_type | 'php' | Backend: php or pear |
| mail_smtp_host | 'localhost' | SMTP server |
| mail_smtp_port | '25' | SMTP port |
| mail_smtp_ssl | false | Enable SSL/TLS |
| mail_smtp_auth | false | Enable SMTP auth |
| mail_smtp_user | '' | SMTP username |
| mail_smtp_pass | '' | SMTP password |

### 13.11 UI/Layout

| Setting | Default | Description |
|---|---|---|
| LEFTNAV | true | Left sidebar |
| RIGHTNAV | true | Right sidebar |
| MIDDLENAV | true | Middle column |
| SHOUTBOX | true | Shoutbox enabled |
| NEWSON | true | News block |
| DONATEON | true | Donation block |
| DISCLAIMERON | true | Disclaimer block |
| SITENOTICEON | true | Site notice banner |
| SITENOTICE | 'Welcome...' | Notice text |
| WELCOMEPMON | true | Auto-PM new users |
| WELCOMEPMMSG | 'Thank you...' | Welcome PM text |

### 13.12 File Paths

| Setting | Description |
|---|---|
| torrent_dir | Torrent file storage (CHMOD 777) |
| nfo_dir | NFO file storage (CHMOD 777) |
| blocks_dir | Block template directory |
| cache_dir | Disk cache directory (CHMOD 777) |

### 13.13 Image Upload

| Setting | Default |
|---|---|
| image_max_filesize | 512 KB |
| allowed_image_types | GIF, JPEG, PNG, WEBP |
