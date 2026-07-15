export interface Notification {
  id: number;
  type: string;
  data: Record<string, unknown>;
  read: boolean;
  created_at: string;
}

/** The route a notification links to, or null if it isn't navigable. */
export function notificationLink(n: Notification): string | null {
  const d = n.data;
  switch (n.type) {
    case "mention":
      // Self-describing: the deep-link is built by the publisher.
      return (d.url as string) || null;
    case "forum_reply":
    case "forum_mention": // legacy
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
    case "pm_received":
      return `${actor} sent you a private message`;
    case "system":
      if (d.warning_type) return "You received a warning";
      return "System notification";
    default:
      return "New notification";
  }
}
