import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useToast } from "@/components/toast";
import { useChat } from "@/lib/useChat";
import { Pagination } from "@/components/Pagination";
import "./notifications.css";

interface Notification {
  id: number;
  type: string;
  data: Record<string, unknown>;
  read: boolean;
  created_at: string;
}

interface Preference {
  notification_type: string;
  enabled: boolean;
}

const PER_PAGE = 25;

const TYPE_LABELS: Record<string, string> = {
  forum_reply: "Forum Reply",
  forum_mention: "Forum Mention",
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

function notificationLink(n: Notification): string | null {
  const d = n.data;
  switch (n.type) {
    case "forum_reply":
    case "forum_mention":
    case "topic_reply":
      if (d.topic_id) return `/forums/topics/${d.topic_id}`;
      break;
    case "torrent_comment":
      if (d.torrent_id) return `/torrent/${d.torrent_id}`;
      break;
    case "pm_received":
      return "/messages";
  }
  return null;
}

function notificationMessage(n: Notification): string {
  const d = n.data;
  const actor = (d.actor_username as string) || "Someone";
  switch (n.type) {
    case "forum_reply":
      return `${actor} replied to your post in "${d.topic_title || "a topic"}"`;
    case "forum_mention":
      return `${actor} mentioned you in "${d.topic_title || "a topic"}"`;
    case "topic_reply":
      return `${actor} posted in "${d.topic_title || "a topic"}" you follow`;
    case "torrent_comment":
      return `${actor} commented on "${d.torrent_name || "your torrent"}"`;
    case "pm_received":
      return `${actor} sent you a private message`;
    case "system":
      if (d.warning_type) return "You received a warning";
      return "System notification";
    default:
      return "New notification";
  }
}

export function NotificationsPage() {
  const toast = useToast();
  const { setNotifUnreadCount } = useChat();
  const [tab, setTab] = useState<"all" | "unread" | "preferences">("all");
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);

  // Preferences state
  const [preferences, setPreferences] = useState<Preference[]>([]);
  const [prefsLoading, setPrefsLoading] = useState(false);

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
        `${getConfig().API_URL}/api/v1/notifications?${params}`,
        { headers: authHeaders() },
      );
      if (!res.ok) throw new Error("Failed to fetch notifications");
      const body = await res.json();
      setNotifications(body.notifications ?? []);
      setTotal(body.total ?? 0);
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

  useEffect(() => {
    if (tab === "preferences") {
      fetchPreferences();
    }
  }, [tab, fetchPreferences]);

  async function handleMarkRead(id: number) {
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/notifications/${id}/read`,
        { method: "PUT", headers: authHeaders() },
      );
      if (!res.ok) throw new Error();
      setNotifications((prev) =>
        prev.map((n) => (n.id === id ? { ...n, read: true } : n)),
      );
      // Refetch the actual count from server for accuracy
      fetch(`${getConfig().API_URL}/api/v1/notifications/unread-count`, {
        headers: authHeaders(),
      })
        .then((r) => r.json())
        .then((d) => setNotifUnreadCount(d?.count ?? 0))
        .catch(() => {});
    } catch {
      toast.error("Failed to mark notification as read");
    }
  }

  async function handleMarkAllRead() {
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/notifications/read-all`,
        { method: "PUT", headers: authHeaders() },
      );
      if (!res.ok) throw new Error();
      setNotifications((prev) => prev.map((n) => ({ ...n, read: true })));
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

  return (
    <div className="notifs-page">
      <div className="notifs-page__header">
        <h1 className="notifs-page__title">Notifications</h1>
        {tab !== "preferences" && notifications.some((n) => !n.read) && (
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
            <div className="notifs-prefs__list">
              {preferences.map((p) => (
                <label key={p.notification_type} className="notifs-prefs__item">
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
          )}
        </div>
      ) : loading ? (
        <p className="notifs-page__empty">Loading...</p>
      ) : notifications.length === 0 ? (
        <p className="notifs-page__empty">
          {tab === "unread"
            ? "No unread notifications"
            : "No notifications yet"}
        </p>
      ) : (
        <>
          <div className="notifs-list">
            {notifications.map((n) => {
              const link = notificationLink(n);
              const content = (
                <div
                  className={`notifs-item${!n.read ? " notifs-item--unread" : ""}`}
                  key={n.id}
                >
                  <div className="notifs-item__type">
                    {TYPE_LABELS[n.type] ?? n.type}
                  </div>
                  <div className="notifs-item__message">
                    {notificationMessage(n)}
                  </div>
                  <div className="notifs-item__meta">
                    <span className="notifs-item__time">
                      {formatTime(n.created_at)}
                    </span>
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
            })}
          </div>
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
