-- A TorrentTrader 3.0 database as an operator is likely to have it: the stock
-- schema, plus a few years of mods.
--
-- The core tables are the full stock definitions, so the tests prove the
-- baseline in internal/baseline survives a round trip through a real MySQL
-- server — which reports several of these types differently from the way the
-- reference document writes them.
--
-- The deliberate deviations, each exercising one branch of the comparison:
--   * users.seedbonus and users.karma     mod-added columns
--   * bonus_log                           a mod-added table
--   * polls, pollanswers, faq, ...        stock tables this install dropped
--   * shoutbox                            a stock table whose columns the
--                                         reference does not document

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
) ENGINE=MyISAM;

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
) ENGINE=MyISAM;

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
) ENGINE=MyISAM;

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
) ENGINE=MyISAM;

CREATE TABLE completed (
    id        INT UNSIGNED NOT NULL AUTO_INCREMENT,
    userid    INT NOT NULL DEFAULT 0,
    torrentid INT NOT NULL DEFAULT 0,
    date      DATETIME DEFAULT NULL,
    PRIMARY KEY (id)
) ENGINE=MyISAM;

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
) ENGINE=MyISAM;

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
) ENGINE=MyISAM;

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
) ENGINE=MyISAM;

CREATE TABLE forum_posts (
    id       INT UNSIGNED NOT NULL AUTO_INCREMENT,
    topicid  INT UNSIGNED NOT NULL DEFAULT 0,
    userid   INT UNSIGNED NOT NULL DEFAULT 0,
    added    DATETIME DEFAULT NULL,
    body     LONGTEXT,
    editedby INT UNSIGNED NOT NULL DEFAULT 0,
    editedat DATETIME DEFAULT NULL,
    PRIMARY KEY (id)
) ENGINE=MyISAM;

CREATE TABLE categories (
    id         INT UNSIGNED NOT NULL AUTO_INCREMENT,
    name       VARCHAR(50) NOT NULL DEFAULT '',
    parent_cat VARCHAR(50) NOT NULL DEFAULT '',
    image      VARCHAR(100) NOT NULL DEFAULT '',
    sort       INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id)
) ENGINE=MyISAM;

CREATE TABLE files (
    id       INT UNSIGNED NOT NULL AUTO_INCREMENT,
    torrent  INT UNSIGNED NOT NULL DEFAULT 0,
    filename VARCHAR(255) NOT NULL DEFAULT '',
    size     BIGINT UNSIGNED NOT NULL DEFAULT 0,
    PRIMARY KEY (id)
) ENGINE=MyISAM;

CREATE TABLE comments (
    id      INT UNSIGNED NOT NULL AUTO_INCREMENT,
    torrent INT UNSIGNED NOT NULL DEFAULT 0,
    news    INT UNSIGNED NOT NULL DEFAULT 0,
    user    INT UNSIGNED NOT NULL DEFAULT 0,
    added   DATETIME DEFAULT NULL,
    text    TEXT,
    PRIMARY KEY (id)
) ENGINE=MyISAM;

-- A stock table the reference document names but does not break down.
CREATE TABLE shoutbox (
    id     INT UNSIGNED NOT NULL AUTO_INCREMENT,
    userid INT UNSIGNED NOT NULL DEFAULT 0,
    date   INT NOT NULL DEFAULT 0,
    text   TEXT,
    PRIMARY KEY (id)
) ENGINE=MyISAM;

-- A table no stock TorrentTrader ever had.
CREATE TABLE bonus_log (
    id     INT UNSIGNED NOT NULL AUTO_INCREMENT,
    userid INT UNSIGNED NOT NULL DEFAULT 0,
    amount FLOAT NOT NULL DEFAULT 0,
    PRIMARY KEY (id)
) ENGINE=MyISAM;

INSERT INTO users (username, passkey, class) VALUES
    ('alice', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 7),
    ('bob',   'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 1),
    ('carol', 'cccccccccccccccccccccccccccccccc', 1);

INSERT INTO torrents (info_hash, name, size, owner) VALUES
    ('0123456789abcdef0123456789abcdef01234567', 'Example release', 1024, 1),
    ('fedcba9876543210fedcba9876543210fedcba98', 'Another release', 2048, 2);

INSERT INTO peers (torrent, peer_id, ip, port, userid) VALUES
    (1, 'peer-one', '10.0.0.1', 6881, '1');
