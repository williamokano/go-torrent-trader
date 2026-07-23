export interface Notification {
  id: number;
  type: string;
  data: Record<string, unknown>;
  read: boolean;
  created_at: string;
}

/**
 * A read-time collapse of one or more notifications sharing a grouping key
 * (currently: same topic for `topic_reply`). Every other type comes back
 * from the API as its own singleton group (count === 1) so the UI has one
 * shape to render either way -- see backend/internal/service/notification_group.go.
 */
export interface NotificationGroup {
  key: string;
  type: string;
  count: number;
  unread: boolean;
  last_actors: string[];
  latest_created_at: string;
  data: Record<string, unknown>;
  notifications: Notification[];
}

/** The route a notification links to, or null if it isn't navigable. */
export function notificationLink(n: Notification): string | null {
  const d = n.data;
  switch (n.type) {
    case "mention": {
      // Structured: the backend stores source + ids + page; we own the routes.
      const page =
        typeof d.page === "number" && d.page > 1 ? `?page=${d.page}` : "";
      if (d.source === "forum_post" && d.topic_id)
        return `/forums/topics/${d.topic_id}${page}#post-${d.post_id}`;
      if (d.source === "torrent_comment" && d.torrent_id)
        return `/torrent/${d.torrent_id}${page}#comment-${d.comment_id}`;
      return null;
    }
    case "forum_reply":
    case "forum_mention": // legacy
    case "topic_reply":
      if (d.topic_id) return `/forums/topics/${d.topic_id}`;
      break;
    case "torrent_comment":
      if (d.torrent_id) return `/torrent/${d.torrent_id}`;
      break;
    case "moderation_message":
    case "moderation_decision":
      if (d.torrent_id) return `/torrent/${d.torrent_id}`;
      break;
    case "pm_received":
      return "/messages";
  }
  return null;
}

/** Human-readable summary line for a notification. */
export function notificationMessage(n: Notification): string {
  const d = n.data;
  const actor = (d.actor_username as string) || "Someone";
  switch (n.type) {
    case "forum_reply":
      return `${actor} replied to your post in "${d.topic_title || "a topic"}"`;
    case "mention":
      return `${actor} mentioned you${d.context_title ? ` in "${d.context_title}"` : ""}`;
    case "forum_mention": // legacy
      return `${actor} mentioned you in "${d.topic_title || "a topic"}"`;
    case "topic_reply":
      return `${actor} posted in "${d.topic_title || "a topic"}" you follow`;
    case "torrent_comment":
      return `${actor} commented on "${d.torrent_name || "your torrent"}"`;
    case "moderation_message":
      return `${actor} posted in the moderation review of "${d.torrent_name || "a torrent"}"`;
    case "moderation_decision": {
      const name = (d.torrent_name as string) || "your torrent";
      return d.decision === "rejected"
        ? `Your torrent "${name}" was rejected`
        : `Your torrent "${name}" was approved`;
    }
    case "pm_received":
      return `${actor} sent you a private message`;
    case "system":
      if (d.warning_type) return "You received a warning";
      return "System notification";
    default:
      return "New notification";
  }
}

/** Human-readable summary line for a (possibly collapsed) notification group. */
export function groupMessage(g: NotificationGroup): string {
  if (g.count <= 1) {
    return g.notifications[0]
      ? notificationMessage(g.notifications[0])
      : "New notification";
  }
  const topicTitle = (g.data.topic_title as string) || "a topic";
  // "new" only while the group still has something unread -- once every
  // reply in it has been read, calling them "new" would be misleading.
  const label = g.unread ? "new replies" : "replies";
  return `${g.count} ${label} in "${topicTitle}" from ${formatActorList(g.last_actors)}`;
}

/** The route a notification group links to, or null if it isn't navigable. */
export function groupLink(g: NotificationGroup): string | null {
  if (g.count <= 1) {
    return g.notifications[0] ? notificationLink(g.notifications[0]) : null;
  }
  if (g.data.topic_id) return `/forums/topics/${g.data.topic_id}`;
  return null;
}

function formatActorList(actors: string[]): string {
  if (actors.length === 0) return "several people";
  if (actors.length === 1) return actors[0];
  if (actors.length === 2) return `${actors[0]} and ${actors[1]}`;
  return `${actors.slice(0, -1).join(", ")}, and ${actors[actors.length - 1]}`;
}
