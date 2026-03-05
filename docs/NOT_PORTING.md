# Features Not Being Ported

The following TorrentTrader 3.0 features will not be ported to the new Go implementation. Each has a rationale explaining why it was dropped.

## Dropped Features

### 1. Blocks/Widget System

**Original**: Server-side widget system for rendering page blocks (e.g., online users, stats, latest torrents in sidebars). Blocks were configurable via admin panel and rendered into page layouts using Smarty templates.

**Why dropped**: Replaced by React component architecture. The frontend handles all UI composition, making a server-side block system redundant.

### 2. Server-Side Theme System

**Original**: PHP-based Smarty templates with server-rendered themes. Admins could switch between themes, and themes controlled the entire HTML output.

**Why dropped**: Replaced by React frontend with CSS-variable-based theming. Themes are now client-side concerns, not server-side.

### 3. UI Internationalization (i18n)

**Original**: Server-side language files for multi-language support. The system loaded translation strings from PHP arrays and injected them into templates.

**Why dropped**: Out of scope for initial release. Can be added later with react-i18next if needed. The tracker community is primarily English-speaking.

### 4. Teams/Groups System

**Original**: User-created teams/groups for organizing around content (e.g., release groups, fan communities). Users could join teams, teams had profiles and member lists.

**Why dropped**: Rarely used in modern private trackers. Adds complexity without proportional value. Note: staff roles and permission groups (admin, moderator, etc.) are still fully supported — this only drops user-created social groups.

### 5. Polls

**Original**: Forum poll creation and voting. Admins or users could attach polls to forum topics with multiple-choice options.

**Why dropped**: Low usage feature. Can be added as a future enhancement if the community requests it.

### 6. FAQ Management (Admin)

**Original**: Admin CRUD interface for FAQ entries stored in the database. FAQs could be created, edited, reordered, and deleted through the admin panel.

**Why dropped**: FAQ content will be a static page. Content changes go through code/deploy, which is simpler and version-controlled.

### 7. Rules Management (Admin)

**Original**: Admin CRUD interface for site rules stored in the database. Similar to FAQ management — editable through the admin panel.

**Why dropped**: Same as FAQ — static page, version-controlled content. Changing rules is infrequent enough that a deploy is acceptable.

### 8. Word Censor/Filter

**Original**: Database-driven word replacement system. Admins could define words and their replacements, which were applied across forum posts, comments, and shoutbox messages.

**Why dropped**: Overly complex for minimal benefit. Basic moderation is better handled by human moderators and the report system.

### 9. Guest Tracking

**Original**: Tracking anonymous/non-logged-in visitors — recording page views, IP addresses, and browsing patterns for guests.

**Why dropped**: Not useful for a private tracker where registration is required to access content. Adds unnecessary database writes with no practical benefit.

### 10. Torrent Batch Import

**Original**: Bulk import of .torrent files via an admin interface, allowing mass-addition of torrents from a directory.

**Why dropped**: Niche admin feature. Individual uploads and the migration tool cover the use cases adequately.

### 11. Connectability Check

**Original**: Server-side check to verify if a user's BitTorrent client port is reachable from the internet. Displayed a "connectable" or "not connectable" status on user profiles.

**Why dropped**: Unreliable with modern NAT/firewall setups, VPNs, and IPv6 transitions. Most BitTorrent clients handle connectivity detection themselves.

### 12. Server Load Monitor

**Original**: PHP page showing server CPU usage, memory consumption, disk space, and other system metrics accessible from the admin panel.

**Why dropped**: Replaced by a proper monitoring stack (Prometheus/Grafana or similar). The application shouldn't expose system metrics directly through its own UI.

### 13. System Check Page

**Original**: Admin page verifying PHP extensions, file permissions, directory writability, and other runtime prerequisites.

**Why dropped**: Irrelevant — we're not using PHP. Docker containers handle runtime dependencies, and Go binaries are self-contained.

### 14. Karma/Reputation System

**Original**: User karma points that could be given by other users as a social reputation indicator, separate from upload/download ratio.

**Why dropped**: Not widely used in practice. The ratio system (upload/download) already serves as the primary reputation metric in private tracker communities.

### 15. Torrent Bookmarks

**Original**: Users could bookmark torrents for later reference, creating a personal saved list accessible from their profile.

**Why dropped**: Not core functionality for MVP. Can be reconsidered as a future enhancement.

## May Be Added Later

Some of these features could be revisited in future phases if there is community demand:

- **Internationalization (i18n)** — Most likely candidate for future addition via react-i18next.
- **Polls** — Straightforward to add to the forum system later.
- **Torrent Bookmarks** — Simple feature that could enhance user experience post-MVP.

These would be evaluated based on user feedback after the initial release.
