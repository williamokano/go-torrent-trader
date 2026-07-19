import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useToast } from "@/components/toast";
import { useChat } from "@/lib/useChat";
import { Pagination } from "@/components/Pagination";
import {
  type Notification,
  type NotificationGroup,
  notificationLink,
  notificationMessage,
  groupLink,
  groupMessage,
} from "./notificationDisplay";
import "./notifications.css";

interface Preference {
  notification_type: string;
  enabled: boolean;
}

type DigestFrequency = "off" | "daily" | "weekly";

const DIGEST_FREQUENCIES: DigestFrequency[] = ["off", "daily", "weekly"];

const DIGEST_LABELS: Record<DigestFrequency, string> = {
  off: "Off",
  daily: "Daily",
  weekly: "Weekly",
};

const PER_PAGE = 25;

const TYPE_LABELS: Record<string, string> = {
  forum_reply: "Forum Reply",
  mention: "Mention",
  forum_mention: "Mention", // legacy rows, superseded by "mention"
  topic_reply: "Topic Reply",
  torrent_comment: "Torrent Comment",
  pm_received: "Private Message",
  system: "System",
};

function authHeaders(): Record<string, string> {
  const token = getAccessToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function formatTime(iso: string): string {
  const d = new Date(iso);
  const now = new Date();
  const diffMs = now.getTime() - d.getTime();
  const diffMin = Math.floor(diffMs / 60000);
  if (diffMin < 1) return "just now";
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHours = Math.floor(diffMin / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  if (diffDays < 7) return `${diffDays}d ago`;
  return d.toLocaleDateString();
}

export function NotificationsPage() {
  const toast = useToast();
  const { setNotifUnreadCount } = useChat();
  const [tab, setTab] = useState<"all" | "unread" | "preferences">("all");
  const [groups, setGroups] = useState<NotificationGroup[]>([]);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);

  // Preferences state
  const [preferences, setPreferences] = useState<Preference[]>([]);
  const [prefsLoading, setPrefsLoading] = useState(false);
  const [digestFrequency, setDigestFrequency] =
    useState<DigestFrequency>("off");

  const totalPages = Math.ceil(total / PER_PAGE);

  const fetchNotifications = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams({
        page: String(page),
        per_page: String(PER_PAGE),
      });
      if (tab === "unread") params.set("unread_only", "true");

      const res = await fetch(
        `${getConfig().API_URL}/api/v1/notifications/grouped?${params}`,
        { headers: authHeaders() },
      );
      if (!res.ok) throw new Error("Failed to fetch notifications");
      const body = await res.json();
      const fetchedTotal = body.total ?? 0;

      // Marking a whole group read can wipe out an entire page in one
      // click (unlike the old per-notification flow, which only ever
      // removed one row at a time) -- if that leaves `page` past the end,
      // snap back instead of rendering an empty page while unread items
      // still exist earlier in the list.
      const lastPage = Math.max(1, Math.ceil(fetchedTotal / PER_PAGE));
      if (page > lastPage) {
        setPage(lastPage);
        return;
      }

      setGroups(body.groups ?? []);
      setTotal(fetchedTotal);
    } catch {
      toast.error("Failed to load notifications");
    } finally {
      setLoading(false);
    }
  }, [page, tab, toast]);

  useEffect(() => {
    if (tab !== "preferences") {
      fetchNotifications();
    }
  }, [tab, page, fetchNotifications]);

  const fetchPreferences = useCallback(async () => {
    setPrefsLoading(true);
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/notifications/preferences`,
        { headers: authHeaders() },
      );
      if (!res.ok) throw new Error("Failed to fetch preferences");
      const body = await res.json();
      setPreferences(body.preferences ?? []);
    } catch {
      toast.error("Failed to load notification preferences");
    } finally {
      setPrefsLoading(false);
    }
  }, [toast]);

  const fetchDigestFrequency = useCallback(async () => {
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/notifications/digest-preference`,
        { headers: authHeaders() },
      );
      if (!res.ok) throw new Error("Failed to fetch digest preference");
      const body = await res.json();
      setDigestFrequency((body.frequency as DigestFrequency) ?? "off");
    } catch {
      toast.error("Failed to load digest preference");
    }
  }, [toast]);

  useEffect(() => {
    if (tab === "preferences") {
      fetchPreferences();
      fetchDigestFrequency();
    }
  }, [tab, fetchPreferences, fetchDigestFrequency]);

  // PUTs a single notification read and applies the local optimistic
  // update. Does not touch the unread-count badge -- callers refetch it
  // once, after all their PUTs have settled (see handleMarkRead /
  // handleMarkGroupRead below). Returns whether the PUT succeeded so a
  // caller marking several notifications at once can report partial
  // failure instead of silently swallowing it.
  async function markNotificationRead(id: number): Promise<boolean> {
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/notifications/${id}/read`,
        { method: "PUT", headers: authHeaders() },
      );
      if (!res.ok) throw new Error();
      setGroups((prev) =>
        prev.map((g) => {
          if (!g.notifications.some((n) => n.id === id)) return g;
          const notifications = g.notifications.map((n) =>
            n.id === id ? { ...n, read: true } : n,
          );
          return {
            ...g,
            notifications,
            unread: notifications.some((n) => !n.read),
          };
        }),
      );
      return true;
    } catch {
      return false;
    }
  }

  // Refetches the authoritative unread count from the server. Called once
  // per user action (never once per PUT) so marking several notifications
  // read at once can't race itself into a stale badge.
  function refreshUnreadCount() {
    fetch(`${getConfig().API_URL}/api/v1/notifications/unread-count`, {
      headers: authHeaders(),
    })
      .then((r) => r.json())
      .then((d) => setNotifUnreadCount(d?.count ?? 0))
      .catch(() => {});
  }

  async function handleMarkRead(id: number) {
    const ok = await markNotificationRead(id);
    if (!ok) {
      toast.error("Failed to mark notification as read");
      return;
    }
    refreshUnreadCount();
  }

  // Marks every still-unread notification underlying a collapsed group as
  // read (e.g. clicking through a "5 new replies" group, or its explicit
  // "Mark all read" control). Reuses the existing per-notification endpoint
  // -- grouping never needed a batch endpoint of its own. The PUTs run in
  // parallel (each targets a distinct notification, so there's no write
  // conflict), but the unread-count badge is refreshed exactly once after
  // they all settle, rather than once per PUT racing to write the badge.
  async function handleMarkGroupRead(g: NotificationGroup) {
    const unreadIds = g.notifications.filter((n) => !n.read).map((n) => n.id);
    if (unreadIds.length === 0) return;
    const results = await Promise.all(
      unreadIds.map((id) => markNotificationRead(id)),
    );
    if (results.some((ok) => !ok)) {
      toast.error("Failed to mark some notifications as read");
    }
    refreshUnreadCount();
  }

  function toggleExpanded(key: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }

  async function handleMarkAllRead() {
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/notifications/read-all`,
        { method: "PUT", headers: authHeaders() },
      );
      if (!res.ok) throw new Error();
      setGroups((prev) =>
        prev.map((g) => ({
          ...g,
          unread: false,
          notifications: g.notifications.map((n) => ({ ...n, read: true })),
        })),
      );
      setNotifUnreadCount(0);
      toast.success("All notifications marked as read");
    } catch {
      toast.error("Failed to mark all as read");
    }
  }

  async function handleTogglePreference(notifType: string, enabled: boolean) {
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/notifications/preferences`,
        {
          method: "PUT",
          headers: {
            ...authHeaders(),
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            notification_type: notifType,
            enabled,
          }),
        },
      );
      if (!res.ok) throw new Error();
      setPreferences((prev) =>
        prev.map((p) =>
          p.notification_type === notifType ? { ...p, enabled } : p,
        ),
      );
    } catch {
      toast.error("Failed to update preference");
    }
  }

  async function handleDigestFrequencyChange(frequency: DigestFrequency) {
    const previous = digestFrequency;
    setDigestFrequency(frequency);
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/notifications/digest-preference`,
        {
          method: "PUT",
          headers: {
            ...authHeaders(),
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ frequency }),
        },
      );
      if (!res.ok) throw new Error();
    } catch {
      setDigestFrequency(previous);
      toast.error("Failed to update digest preference");
    }
  }

  // renderNotification renders a single notification exactly as the
  // pre-grouping UI did: type, message, time, and a "Mark read" control
  // when unread. Used both for singleton groups (count === 1, the common
  // case) and for each entry inside an expanded group.
  function renderNotification(n: Notification) {
    const link = notificationLink(n);
    const content = (
      <div
        className={`notifs-item${!n.read ? " notifs-item--unread" : ""}`}
        key={n.id}
      >
        <div className="notifs-item__type">{TYPE_LABELS[n.type] ?? n.type}</div>
        <div className="notifs-item__message">{notificationMessage(n)}</div>
        <div className="notifs-item__meta">
          <span className="notifs-item__time">{formatTime(n.created_at)}</span>
          {!n.read && (
            <button
              className="notifs-item__mark-read"
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                handleMarkRead(n.id);
              }}
            >
              Mark read
            </button>
          )}
        </div>
      </div>
    );

    return link ? (
      <Link
        key={n.id}
        to={link}
        className="notifs-item__link"
        onClick={() => {
          if (!n.read) handleMarkRead(n.id);
        }}
      >
        {content}
      </Link>
    ) : (
      content
    );
  }

  // renderGroup renders a collapsed entry: a singleton (count === 1) looks
  // identical to a plain notification, while a real group (multiple
  // topic_reply notifications for the same topic) shows the combined
  // message, count, and an expand toggle that reveals the individual
  // notifications underneath via renderNotification.
  function renderGroup(g: NotificationGroup) {
    if (g.count <= 1) {
      return g.notifications[0] ? renderNotification(g.notifications[0]) : null;
    }

    const isOpen = expanded.has(g.key);
    const link = groupLink(g);
    // Only the type + message (the navigable part) sits inside the Link.
    // The count badge, "Mark all read", and expand toggle are rendered as
    // its siblings, not its children -- nesting <button>s inside an <a>
    // is invalid HTML and confuses screen readers, and this group summary
    // has two of them (the singleton notification view below still nests
    // its one "Mark read" button inside the Link, matching this page's
    // pre-existing pattern; not touched here to keep this fix scoped).
    const summaryText = (
      <>
        <div className="notifs-item__type">{TYPE_LABELS[g.type] ?? g.type}</div>
        <div className="notifs-item__message">{groupMessage(g)}</div>
      </>
    );

    return (
      <div className="notifs-group" key={g.key}>
        <div
          className={`notifs-item notifs-item--group${g.unread ? " notifs-item--unread" : ""}`}
        >
          {link ? (
            <Link
              to={link}
              className="notifs-item__link notifs-group__summary-link"
              onClick={() => {
                if (g.unread) handleMarkGroupRead(g);
              }}
            >
              {summaryText}
            </Link>
          ) : (
            summaryText
          )}
          <div className="notifs-item__meta">
            <span className="notifs-item__time">
              {formatTime(g.latest_created_at)}
            </span>
            <span
              className="notifs-group__count"
              title={`${g.count} notifications`}
              aria-hidden="true"
            >
              {g.count}
            </span>
            {g.unread && (
              <button
                className="notifs-item__mark-read"
                onClick={() => handleMarkGroupRead(g)}
              >
                Mark all read
              </button>
            )}
            <button
              className="notifs-group__toggle"
              aria-expanded={isOpen}
              onClick={() => toggleExpanded(g.key)}
            >
              {isOpen ? "Hide" : "Show"} individual replies
            </button>
          </div>
        </div>
        {isOpen && (
          <div className="notifs-group__children">
            {g.notifications.map((n) => renderNotification(n))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="notifs-page">
      <div className="notifs-page__header">
        <h1 className="notifs-page__title">Notifications</h1>
        {tab !== "preferences" && groups.some((g) => g.unread) && (
          <button className="notifs-page__mark-all" onClick={handleMarkAllRead}>
            Mark all read
          </button>
        )}
      </div>

      <div className="notifs-page__tabs">
        <button
          className={`notifs-page__tab${tab === "all" ? " notifs-page__tab--active" : ""}`}
          onClick={() => {
            setTab("all");
            setPage(1);
          }}
        >
          All
        </button>
        <button
          className={`notifs-page__tab${tab === "unread" ? " notifs-page__tab--active" : ""}`}
          onClick={() => {
            setTab("unread");
            setPage(1);
          }}
        >
          Unread
        </button>
        <button
          className={`notifs-page__tab${tab === "preferences" ? " notifs-page__tab--active" : ""}`}
          onClick={() => setTab("preferences")}
        >
          Preferences
        </button>
      </div>

      {tab === "preferences" ? (
        <div className="notifs-prefs">
          {prefsLoading ? (
            <p className="notifs-page__empty">Loading preferences...</p>
          ) : (
            <>
              <div className="notifs-prefs__digest">
                <label
                  className="notifs-prefs__digest-label"
                  htmlFor="digest-frequency"
                >
                  Email digest
                </label>
                <select
                  id="digest-frequency"
                  className="notifs-prefs__digest-select"
                  value={digestFrequency}
                  onChange={(e) =>
                    handleDigestFrequencyChange(
                      e.target.value as DigestFrequency,
                    )
                  }
                >
                  {DIGEST_FREQUENCIES.map((freq) => (
                    <option key={freq} value={freq}>
                      {DIGEST_LABELS[freq]}
                    </option>
                  ))}
                </select>
                <p className="notifs-prefs__digest-hint">
                  Get an email summarizing your unread notifications since your
                  last digest — sent at most once a day (daily) or once a week
                  (weekly), and only when there's something new.
                </p>
              </div>
              <div className="notifs-prefs__divider" />
              <div className="notifs-prefs__list">
                {preferences.map((p) => (
                  <label
                    key={p.notification_type}
                    className="notifs-prefs__item"
                  >
                    <input
                      type="checkbox"
                      checked={p.enabled}
                      onChange={(e) =>
                        handleTogglePreference(
                          p.notification_type,
                          e.target.checked,
                        )
                      }
                    />
                    <span className="notifs-prefs__label">
                      {TYPE_LABELS[p.notification_type] ?? p.notification_type}
                    </span>
                  </label>
                ))}
              </div>
            </>
          )}
        </div>
      ) : loading ? (
        <p className="notifs-page__empty">Loading...</p>
      ) : groups.length === 0 ? (
        <p className="notifs-page__empty">
          {tab === "unread"
            ? "No unread notifications"
            : "No notifications yet"}
        </p>
      ) : (
        <>
          <div className="notifs-list">{groups.map((g) => renderGroup(g))}</div>
          <Pagination
            currentPage={page}
            totalPages={totalPages}
            onPageChange={setPage}
          />
        </>
      )}
    </div>
  );
}
