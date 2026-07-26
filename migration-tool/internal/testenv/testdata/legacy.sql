-- The legacy corpus: a TorrentTrader 3.0 database as an operator actually has
-- it, after a few years of mods and a decade of accumulated damage.
--
-- Every test in this module runs against this one file. It is deliberately
-- small and deliberately nasty: a million clean rows would prove less than the
-- fifty awkward ones below, because a migration does not fail on the ordinary
-- data. It fails on the row somebody inserted in 2009 with the strict mode off.
--
-- The core tables carry their full stock definitions, so the baseline in
-- internal/baseline is checked against what a real MySQL 8 reports rather than
-- against what the reference document says — the two differ for several of
-- these types.
--
-- Structural deviations, each exercising one branch of the comparison:
--   * users.seedbonus and users.karma     mod-added columns
--   * bonus_log                           a mod-added table
--   * polls, pollanswers, faq, ...        stock tables this install dropped
--   * shoutbox                            a stock table whose columns the
--                                         reference does not document
--   * DEFAULT CHARSET=latin1              what a 2008-era install actually is,
--                                         and the reason the tool reports
--                                         encodings at all
--
-- Data deviations, each a way a real migration breaks. The row that carries
-- each one is commented where it is inserted:
--   * zero dates                          MyISAM accepts '0000-00-00', and
--                                         PostgreSQL rejects it outright
--   * invited_by = 0                      the legacy "nobody" sentinel meeting
--                                         a real foreign key
--   * an over-long username               legacy varchar(40) into varchar(20)
--   * latin1 high bytes                   the only proof the text is converted
--                                         rather than copied
--   * a malformed info_hash               not 40 hex characters
--   * orphans                             a peer whose member was deleted, a
--                                         completion whose torrent was deleted
--   * a duplicate passkey                 UNIQUE in the target, not in MyISAM
--   * a dangling class                    pointing at a group that is gone
--   * unclosed and nested BBCode          the converter's hard cases
--
-- Strict mode is off for the load, which is how these rows came to exist in the
-- first place. MySQL 8 would otherwise refuse to insert half of them.
SET SESSION sql_mode = '';

CREATE TABLE users (
    id           INT UNSIGNED NOT NULL AUTO_INCREMENT,
    username     VARCHAR(40) NOT NULL DEFAULT '',
    password     VARCHAR(40) NOT NULL DEFAULT '',
    secret       VARCHAR(20) BINARY NOT NULL DEFAULT '',
    editsecret   VARCHAR(20) BINARY NOT NULL DEFAULT '',
    email        VARCHAR(80) NOT NULL DEFAULT '',
    status       ENUM('pending','confirmed') NOT NULL DEFAULT 'pending',
    enabled      VARCHAR(10) NOT NULL DEFAULT 'yes',
    added        DATETIME DEFAULT NULL,
    last_login   DATETIME DEFAULT NULL,
    last_access  DATETIME DEFAULT NULL,
    last_browse  INT DEFAULT 0,
    ip           VARCHAR(39) NOT NULL DEFAULT '',
    class        TINYINT UNSIGNED NOT NULL DEFAULT 1,
    privacy      ENUM('strong','normal','low') NOT NULL DEFAULT 'normal',
    stylesheet   INT NOT NULL DEFAULT 1,
    language     VARCHAR(20) NOT NULL DEFAULT '',
    uploaded     BIGINT UNSIGNED NOT NULL DEFAULT 0,
    downloaded   BIGINT UNSIGNED NOT NULL DEFAULT 0,
    passkey      VARCHAR(32) NOT NULL DEFAULT '',
    avatar       VARCHAR(100) NOT NULL DEFAULT '',
    title        VARCHAR(30) NOT NULL DEFAULT '',
    signature    VARCHAR(200) NOT NULL DEFAULT '',
    info         TEXT,
    country      INT UNSIGNED NOT NULL DEFAULT 0,
    gender       VARCHAR(6) NOT NULL DEFAULT '',
    age          INT NOT NULL DEFAULT 0,
    client       VARCHAR(25) NOT NULL DEFAULT '',
    donated      INT UNSIGNED NOT NULL DEFAULT 0,
    warned       CHAR(3) NOT NULL DEFAULT 'no',
    forumbanned  CHAR(3) NOT NULL DEFAULT 'no',
    modcomment   TEXT,
    acceptpms    ENUM('yes','no') NOT NULL DEFAULT 'yes',
    commentpm    ENUM('yes','no') NOT NULL DEFAULT 'yes',
    notifs       VARCHAR(100) NOT NULL DEFAULT '',
    invited_by   INT NOT NULL DEFAULT 0,
    invitees     VARCHAR(100) NOT NULL DEFAULT '',
    invites      SMALLINT NOT NULL DEFAULT 0,
    invitedate   DATETIME DEFAULT NULL,
    team         INT UNSIGNED NOT NULL DEFAULT 0,
    tzoffset     INT NOT NULL DEFAULT 0,
    hideshoutbox ENUM('yes','no') NOT NULL DEFAULT 'no',
    page         TEXT,
    -- Mods.
    seedbonus    FLOAT NOT NULL DEFAULT 0,
    karma        INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    UNIQUE KEY username (username)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

CREATE TABLE `groups` (
    group_id        INT NOT NULL AUTO_INCREMENT,
    level           VARCHAR(50) NOT NULL DEFAULT '',
    view_torrents   ENUM('yes','no') NOT NULL DEFAULT 'yes',
    edit_torrents   ENUM('yes','no') NOT NULL DEFAULT 'no',
    delete_torrents ENUM('yes','no') NOT NULL DEFAULT 'no',
    view_users      ENUM('yes','no') NOT NULL DEFAULT 'yes',
    edit_users      ENUM('yes','no') NOT NULL DEFAULT 'no',
    delete_users    ENUM('yes','no') NOT NULL DEFAULT 'no',
    view_news       ENUM('yes','no') NOT NULL DEFAULT 'yes',
    edit_news       ENUM('yes','no') NOT NULL DEFAULT 'no',
    delete_news     ENUM('yes','no') NOT NULL DEFAULT 'no',
    can_upload      ENUM('yes','no') NOT NULL DEFAULT 'no',
    can_download    ENUM('yes','no') NOT NULL DEFAULT 'yes',
    view_forum      ENUM('yes','no') NOT NULL DEFAULT 'yes',
    edit_forum      ENUM('yes','no') NOT NULL DEFAULT 'yes',
    delete_forum    ENUM('yes','no') NOT NULL DEFAULT 'no',
    control_panel   ENUM('yes','no') NOT NULL DEFAULT 'no',
    staff_page      ENUM('yes','no') NOT NULL DEFAULT 'no',
    staff_public    ENUM('yes','no') NOT NULL DEFAULT 'no',
    staff_sort      TINYINT UNSIGNED NOT NULL DEFAULT 0,
    PRIMARY KEY (group_id)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

CREATE TABLE torrents (
    id              INT UNSIGNED NOT NULL AUTO_INCREMENT,
    info_hash       VARCHAR(40) NOT NULL DEFAULT '',
    name            VARCHAR(255) NOT NULL DEFAULT '',
    filename        VARCHAR(255) NOT NULL DEFAULT '',
    save_as         VARCHAR(255) NOT NULL DEFAULT '',
    descr           TEXT,
    image1          TEXT,
    image2          TEXT,
    category        INT UNSIGNED NOT NULL DEFAULT 0,
    torrentlang     INT UNSIGNED NOT NULL DEFAULT 0,
    size            BIGINT UNSIGNED NOT NULL DEFAULT 0,
    added           DATETIME DEFAULT NULL,
    type            ENUM('single','multi') NOT NULL DEFAULT 'single',
    numfiles        INT UNSIGNED NOT NULL DEFAULT 0,
    owner           INT UNSIGNED NOT NULL DEFAULT 0,
    anon            ENUM('yes','no') NOT NULL DEFAULT 'no',
    comments        INT UNSIGNED NOT NULL DEFAULT 0,
    views           INT UNSIGNED NOT NULL DEFAULT 0,
    hits            INT UNSIGNED NOT NULL DEFAULT 0,
    times_completed INT UNSIGNED NOT NULL DEFAULT 0,
    seeders         INT UNSIGNED NOT NULL DEFAULT 0,
    leechers        INT UNSIGNED NOT NULL DEFAULT 0,
    numratings      INT UNSIGNED NOT NULL DEFAULT 0,
    ratingsum       INT UNSIGNED NOT NULL DEFAULT 0,
    visible         ENUM('yes','no') NOT NULL DEFAULT 'yes',
    banned          ENUM('yes','no') NOT NULL DEFAULT 'no',
    nfo             ENUM('yes','no') NOT NULL DEFAULT 'no',
    announce        VARCHAR(255) NOT NULL DEFAULT '',
    external        ENUM('yes','no') NOT NULL DEFAULT 'no',
    freeleech       ENUM('0','1') NOT NULL DEFAULT '0',
    last_action     DATETIME DEFAULT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY info_hash (info_hash)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

CREATE TABLE peers (
    id          INT UNSIGNED NOT NULL AUTO_INCREMENT,
    torrent     INT UNSIGNED NOT NULL DEFAULT 0,
    peer_id     VARCHAR(40) NOT NULL DEFAULT '',
    ip          VARCHAR(64) NOT NULL DEFAULT '',
    port        SMALLINT UNSIGNED NOT NULL DEFAULT 0,
    uploaded    BIGINT UNSIGNED NOT NULL DEFAULT 0,
    downloaded  BIGINT UNSIGNED NOT NULL DEFAULT 0,
    to_go       BIGINT UNSIGNED NOT NULL DEFAULT 0,
    seeder      ENUM('yes','no') NOT NULL DEFAULT 'no',
    started     DATETIME DEFAULT NULL,
    last_action DATETIME DEFAULT NULL,
    connectable ENUM('yes','no') NOT NULL DEFAULT 'yes',
    client      VARCHAR(60) NOT NULL DEFAULT '',
    userid      VARCHAR(32) NOT NULL DEFAULT '',
    passkey     VARCHAR(32) NOT NULL DEFAULT '',
    PRIMARY KEY (id)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

CREATE TABLE completed (
    id        INT UNSIGNED NOT NULL AUTO_INCREMENT,
    userid    INT NOT NULL DEFAULT 0,
    torrentid INT NOT NULL DEFAULT 0,
    date      DATETIME DEFAULT NULL,
    PRIMARY KEY (id)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

CREATE TABLE messages (
    id       INT UNSIGNED NOT NULL AUTO_INCREMENT,
    sender   INT UNSIGNED NOT NULL DEFAULT 0,
    receiver INT UNSIGNED NOT NULL DEFAULT 0,
    added    DATETIME DEFAULT NULL,
    subject  TEXT,
    msg      TEXT,
    unread   ENUM('yes','no') NOT NULL DEFAULT 'yes',
    poster   BIGINT UNSIGNED NOT NULL DEFAULT 0,
    location ENUM('in','out','both','draft','template') NOT NULL DEFAULT 'in',
    PRIMARY KEY (id)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

CREATE TABLE forum_forums (
    id            INT UNSIGNED NOT NULL AUTO_INCREMENT,
    name          VARCHAR(60) NOT NULL DEFAULT '',
    description   VARCHAR(200) NOT NULL DEFAULT '',
    category      TINYINT NOT NULL DEFAULT 0,
    sort          TINYINT UNSIGNED NOT NULL DEFAULT 0,
    minclassread  TINYINT UNSIGNED NOT NULL DEFAULT 0,
    minclasswrite TINYINT UNSIGNED NOT NULL DEFAULT 0,
    guest_read    ENUM('yes','no') NOT NULL DEFAULT 'no',
    PRIMARY KEY (id)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

CREATE TABLE forum_topics (
    id       INT UNSIGNED NOT NULL AUTO_INCREMENT,
    forumid  INT UNSIGNED NOT NULL DEFAULT 0,
    userid   INT UNSIGNED NOT NULL DEFAULT 0,
    subject  VARCHAR(40) NOT NULL DEFAULT '',
    locked   ENUM('yes','no') NOT NULL DEFAULT 'no',
    sticky   ENUM('yes','no') NOT NULL DEFAULT 'no',
    moved    ENUM('yes','no') NOT NULL DEFAULT 'no',
    views    INT NOT NULL DEFAULT 0,
    lastpost INT UNSIGNED NOT NULL DEFAULT 0,
    PRIMARY KEY (id)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

CREATE TABLE forum_posts (
    id       INT UNSIGNED NOT NULL AUTO_INCREMENT,
    topicid  INT UNSIGNED NOT NULL DEFAULT 0,
    userid   INT UNSIGNED NOT NULL DEFAULT 0,
    added    DATETIME DEFAULT NULL,
    body     LONGTEXT,
    editedby INT UNSIGNED NOT NULL DEFAULT 0,
    editedat DATETIME DEFAULT NULL,
    PRIMARY KEY (id)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

CREATE TABLE categories (
    id         INT UNSIGNED NOT NULL AUTO_INCREMENT,
    name       VARCHAR(50) NOT NULL DEFAULT '',
    parent_cat VARCHAR(50) NOT NULL DEFAULT '',
    image      VARCHAR(100) NOT NULL DEFAULT '',
    sort       INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

CREATE TABLE files (
    id       INT UNSIGNED NOT NULL AUTO_INCREMENT,
    torrent  INT UNSIGNED NOT NULL DEFAULT 0,
    filename VARCHAR(255) NOT NULL DEFAULT '',
    size     BIGINT UNSIGNED NOT NULL DEFAULT 0,
    PRIMARY KEY (id)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

CREATE TABLE comments (
    id      INT UNSIGNED NOT NULL AUTO_INCREMENT,
    torrent INT UNSIGNED NOT NULL DEFAULT 0,
    news    INT UNSIGNED NOT NULL DEFAULT 0,
    user    INT UNSIGNED NOT NULL DEFAULT 0,
    added   DATETIME DEFAULT NULL,
    text    TEXT,
    PRIMARY KEY (id)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

CREATE TABLE forumcats (
    id   INT UNSIGNED NOT NULL AUTO_INCREMENT,
    name VARCHAR(60) NOT NULL DEFAULT '',
    sort TINYINT UNSIGNED NOT NULL DEFAULT 0,
    PRIMARY KEY (id)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

-- A stock table the reference document names but does not break down.
CREATE TABLE shoutbox (
    id     INT UNSIGNED NOT NULL AUTO_INCREMENT,
    userid INT UNSIGNED NOT NULL DEFAULT 0,
    date   INT NOT NULL DEFAULT 0,
    text   TEXT,
    PRIMARY KEY (id)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

-- A table no stock TorrentTrader ever had.
CREATE TABLE bonus_log (
    id     INT UNSIGNED NOT NULL AUTO_INCREMENT,
    userid INT UNSIGNED NOT NULL DEFAULT 0,
    amount FLOAT NOT NULL DEFAULT 0,
    PRIMARY KEY (id)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;

-- ---------------------------------------------------------------------------
-- Members
-- ---------------------------------------------------------------------------
INSERT INTO users (id, username, password, email, status, enabled, added, ip, class, uploaded, downloaded, passkey, info, invited_by, warned, forumbanned, donated) VALUES
    -- Ordinary rows, so "everything is broken" is not the only case covered.
    (1, 'alice',   '5f4dcc3b5aa765d61d8327deb882cf99', 'alice@example.com', 'confirmed', 'yes', '2008-04-01 12:00:00', '10.0.0.1',  7, 1099511627776, 549755813888, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1', 'Site founder.',      0, 'no',  'no',  25),
    (2, 'bob',     '5f4dcc3b5aa765d61d8327deb882cf99', 'bob@example.com',   'confirmed', 'yes', '2009-06-15 08:30:00', '10.0.0.2',  1, 10737418240,   21474836480,  'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa2', 'Just here for the films.', 1, 'no', 'no', 0),

    -- Latin1 high bytes. If the migration copies these as bytes instead of
    -- converting them, this is where it shows.
    (3, 'renée',   '5f4dcc3b5aa765d61d8327deb882cf99', 'renee@example.com', 'confirmed', 'yes', '2010-02-20 22:10:00', '10.0.0.3',  1, 5368709120,    5368709120,   'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa3', 'Café, naïve, Größe, façade.', 1, 'no', 'no', 0),

    -- 27 characters, against a target username of varchar(20). Nothing can
    -- migrate this row without a decision being made about it first.
    (4, 'a_very_long_legacy_username', '5f4dcc3b5aa765d61d8327deb882cf99', 'long@example.com', 'confirmed', 'yes', '2011-01-01 00:00:00', '10.0.0.4', 1, 0, 0, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa4', '', 1, 'no', 'no', 0),

    -- A zero date. MyISAM took it; PostgreSQL will not.
    (5, 'zerodate', '5f4dcc3b5aa765d61d8327deb882cf99', 'zero@example.com', 'confirmed', 'yes', '0000-00-00 00:00:00', '10.0.0.5', 1, 0, 0, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa5', '', 0, 'no', 'no', 0),

    -- class 99: no such group. The target has a real foreign key here.
    (6, 'orphanclass', '5f4dcc3b5aa765d61d8327deb882cf99', 'orphan@example.com', 'confirmed', 'yes', '2012-03-03 03:03:03', '10.0.0.6', 99, 0, 0, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa6', '', 1, 'no', 'no', 0),

    -- The same passkey as alice. UNIQUE in the target, unconstrained in MyISAM,
    -- and a passkey collision means two members announcing as one.
    (7, 'dupekey', '5f4dcc3b5aa765d61d8327deb882cf99', 'dupe@example.com', 'confirmed', 'yes', '2013-05-05 05:05:05', '10.0.0.7', 1, 0, 0, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1', '', 1, 'no', 'no', 0),

    -- Never confirmed, disabled, forum-banned and warned: the flag combination
    -- that has to survive four separate transforms intact.
    (8, 'pending', '5f4dcc3b5aa765d61d8327deb882cf99', 'pending@example.com', 'pending', 'no', '2014-07-07 07:07:07', '', 1, 0, 0, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa8', '', 0, 'yes', 'yes', 0);

-- ---------------------------------------------------------------------------
-- Groups. Deliberately not the full stock seven: class 99 above points at
-- nothing, and an install that renamed its groups is the normal case.
-- ---------------------------------------------------------------------------
INSERT INTO `groups` (group_id, level, can_upload, can_download, control_panel, edit_torrents) VALUES
    (1, 'Member',        'no',  'yes', 'no',  'no'),
    (2, 'Power User',    'yes', 'yes', 'no',  'no'),
    (7, 'Administrator', 'yes', 'yes', 'yes', 'yes');

-- ---------------------------------------------------------------------------
-- Torrents
-- ---------------------------------------------------------------------------
INSERT INTO torrents (id, info_hash, name, descr, category, size, added, type, numfiles, owner, visible, banned, nfo, freeleech, times_completed, seeders, leechers, last_action) VALUES
    (1, '0123456789abcdef0123456789abcdef01234567', 'Ordinary Release 2008', '[b]Bold[/b] and [i]italic[/i].', 1, 734003200, '2008-05-01 10:00:00', 'single', 1, 1, 'yes', 'no', 'yes', '0', 120, 4, 1, '2024-01-01 00:00:00'),

    -- Unclosed and nested BBCode, which is what forum and description text
    -- actually looks like after fifteen years of hand-typing.
    (2, 'fedcba9876543210fedcba9876543210fedcba98', 'Nested Markup', '[quote][b]unclosed bold [i]and italic[/b] still going[/quote] [url=http://example.com]link', 1, 1073741824, '2010-08-12 14:20:00', 'multi', 12, 2, 'yes', 'no', 'no', '1', 8, 2, 3, '2024-01-02 00:00:00'),

    -- Not 40 hex characters. The tracker compares 20 raw bytes, so this row
    -- cannot be converted and must not be silently skipped either.
    (3, 'NOTAVALIDINFOHASH', 'Corrupt Hash', 'Uploaded by a broken script in 2009.', 1, 100, '2009-09-09 09:09:09', 'single', 1, 1, 'no', 'yes', 'no', '0', 0, 0, 0, '2009-09-09 09:09:09'),

    -- Zero date, and an owner who no longer exists.
    (4, 'aaaabbbbccccddddeeeeffff00001111222233334', 'Orphaned Upload', 'Owner was deleted years ago.', 1, 2048, '0000-00-00 00:00:00', 'single', 1, 404, 'yes', 'no', 'no', '0', 3, 0, 0, '0000-00-00 00:00:00'),

    -- Latin1 in a torrent name and description.
    (5, '99998888777766665555444433332222111100ab', 'Le Fabuleux Destin d''Amélie', 'Réalisé par Jean-Pierre Jeunet. Größe: 1,4 Go.', 1, 1503238553, '2011-11-11 11:11:11', 'multi', 24, 3, 'yes', 'no', 'no', '0', 42, 7, 2, '2024-01-03 00:00:00');

INSERT INTO categories (id, name, parent_cat) VALUES
    (1, 'Movies', ''),
    (2, 'Movies/DVD-R', 'Movies');

-- A torrent with many file rows, and torrent 1 deliberately has none — the
-- target holds the file list as a JSONB document, so both ends matter.
INSERT INTO files (torrent, filename, size) VALUES
    (2, 'nested/part01.rar', 89478485),
    (2, 'nested/part02.rar', 89478485),
    (2, 'nested/part03.rar', 89478485),
    (5, 'amélie/VIDEO_TS/VTS_01_1.VOB', 1073741824),
    (5, 'amélie/VIDEO_TS/VTS_01_2.VOB', 429496729);

-- ---------------------------------------------------------------------------
-- Swarm and history
-- ---------------------------------------------------------------------------
INSERT INTO peers (torrent, peer_id, ip, port, uploaded, downloaded, to_go, seeder, started, last_action, client, userid, passkey) VALUES
    (1, '-qB4450-abcdefghijkl', '10.0.0.1', 6881, 1048576, 0, 0, 'yes', '2024-01-01 00:00:00', '2024-01-01 01:00:00', 'qBittorrent 4.4.5', '1', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1'),
    -- IPv6, which the legacy varchar(64) holds and the target INET must parse.
    (1, '-TR2940-mnopqrstuvwx', '2001:db8::1', 51413, 0, 524288, 524288, 'no', '2024-01-01 00:30:00', '2024-01-01 01:00:00', 'Transmission 2.94', '2', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa2'),
    -- A peer whose member was deleted. peers.user_id is NOT NULL in the target.
    (1, '-DE13F0-yzabcdefghij', '10.0.0.99', 6881, 0, 0, 100, 'no', '2024-01-01 00:45:00', '2024-01-01 01:00:00', 'Deluge 1.3.15', '404', 'ffffffffffffffffffffffffffffffff');

INSERT INTO completed (userid, torrentid, date) VALUES
    (1, 1, '2008-05-02 11:00:00'),
    (2, 1, '2010-01-01 00:00:00'),
    (3, 5, '2011-11-12 00:00:00'),
    -- A completion for a torrent that no longer exists.
    (1, 999, '2012-01-01 00:00:00'),
    -- ...and one for a member who does not.
    (404, 1, '2012-01-02 00:00:00');

-- ---------------------------------------------------------------------------
-- Social
-- ---------------------------------------------------------------------------
INSERT INTO messages (sender, receiver, added, subject, msg, unread, location) VALUES
    (1, 2, '2010-01-01 10:00:00', 'Welcome', 'Welcome to the tracker. [b]Read the rules.[/b]', 'no', 'both'),
    (2, 1, '2010-01-02 10:00:00', 'Ré: Welcome', 'Merci ! Très bien.', 'yes', 'both'),
    -- A draft and a template: not messages, and the target keeps them in
    -- saved_messages instead.
    (1, 0, '2010-01-03 10:00:00', 'Unsent draft', 'Half-written and never sent.', 'no', 'draft'),
    (1, 0, '2010-01-04 10:00:00', 'Warning template', 'You have been warned for [reason].', 'no', 'template');

INSERT INTO forumcats (id, name) VALUES (1, 'General');

INSERT INTO forum_forums (id, name, description, category, sort, minclassread, minclasswrite) VALUES
    (1, 'Announcements', 'Staff posts here.', 1, 1, 1, 7),
    (2, 'Chat',          'Everyone posts here.', 1, 2, 1, 1);

INSERT INTO forum_topics (id, forumid, userid, subject, locked, sticky, views, lastpost) VALUES
    (1, 1, 1, 'Welcome to the tracker', 'no', 'yes', 1500, 2),
    (2, 2, 3, 'Qu''est-ce que vous regardez ?', 'no', 'no', 42, 3);

INSERT INTO forum_posts (id, topicid, userid, added, body, editedby, editedat) VALUES
    (1, 1, 1, '2008-04-01 12:30:00', 'Welcome. Please read the rules.', 0, '0000-00-00 00:00:00'),
    (2, 1, 2, '2009-06-15 09:00:00', '[quote=alice]Please read the rules.[/quote] Done.', 1, '2009-06-15 09:05:00'),
    (3, 2, 3, '2011-11-11 12:00:00', 'Je regarde [i]Amélie[/i] — très bien !', 0, '0000-00-00 00:00:00');

INSERT INTO comments (torrent, user, added, text) VALUES
    (1, 2, '2008-05-03 12:00:00', 'Good quality, thanks.'),
    (5, 3, '2011-11-13 12:00:00', 'Excellente qualité.');

INSERT INTO shoutbox (userid, date, text) VALUES
    (1, 1209600000, 'Tracker is up.'),
    (3, 1320000000, 'Bonjour à tous !');

-- A mod table nobody but the operator understands.
INSERT INTO bonus_log (userid, amount) VALUES
    (1, 1500.5),
    (2, 20.0);
