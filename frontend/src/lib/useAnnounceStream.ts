import { useEffect, useRef, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";

/** One announcement as the backend's connector pipeline publishes it. */
export interface Announcement {
  event: string;
  title: string;
  body: string;
  torrent_id: number;
  name: string;
  info_hash: string;
  category_id: number;
  category: string;
  size: number;
  file_count: number;
  uploader: string;
  anonymous: boolean;
  freeleech: boolean;
  silver: boolean;
  url: string;
  published_at: string;
  /** >0 when this stands in for a batch the rate limit could not send one by one. */
  coalesced?: number;
}

/**
 * An announcement plus a stable identity for rendering.
 *
 * The torrent id is not usable as a React key: a coalesced summary carries its
 * representative row's id, and the list only ever grows at the front, so an
 * index-based key would shift under every entry on each new event and remount
 * the whole list.
 */
export interface FeedItem extends Announcement {
  key: number;
}

export type StreamState = "connecting" | "live" | "reconnecting";

/**
 * The feed is capped rather than unbounded: a page left open for a week should
 * not accumulate every release ever announced.
 */
const MAX_ITEMS = 100;

/** How long to wait before rebuilding a dropped stream. */
const RECONNECT_DELAY_MS = 5000;

interface AnnounceStream {
  announcements: FeedItem[];
  state: StreamState;
}

/**
 * Subscribes to the live announcement stream.
 *
 * EventSource is used rather than a WebSocket because the feed is one-way. Its
 * built-in retry is deliberately not relied on, though: it always reconnects to
 * the URL it was constructed with, and the token is in that URL. After a token
 * refresh the browser would retry a request the server must reject, forever. So
 * a failed stream is torn down and rebuilt with a freshly read token.
 */
export function useAnnounceStream(): AnnounceStream {
  const [announcements, setAnnouncements] = useState<FeedItem[]>([]);
  const [state, setState] = useState<StreamState>("connecting");
  // Refs rather than state: the message handler must not be re-created (and the
  // stream re-opened) on every event.
  const seen = useRef<Set<number>>(new Set());
  const nextKey = useRef(0);

  useEffect(() => {
    let source: EventSource | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let cancelled = false;

    function handleAnnouncement(event: MessageEvent) {
      let announcement: Announcement;
      try {
        announcement = JSON.parse(event.data);
      } catch {
        return; // A frame we cannot read is not worth breaking the feed over.
      }

      setState("live");
      setAnnouncements((previous) => {
        // A reconnect can replay an announcement the page already has. A
        // coalesced summary is exempt: it shares its representative row's id
        // but is a different thing to show.
        if (
          !announcement.coalesced &&
          seen.current.has(announcement.torrent_id)
        ) {
          return previous;
        }

        nextKey.current += 1;
        const next = [
          { ...announcement, key: nextKey.current },
          ...previous,
        ].slice(0, MAX_ITEMS);

        // Rebuilt from what is retained, so the dedupe set stays bounded by the
        // cap instead of growing for the life of the page.
        seen.current = new Set(
          next.filter((item) => !item.coalesced).map((item) => item.torrent_id),
        );
        return next;
      });
    }

    function connect() {
      const token = getAccessToken();
      if (!token) return;

      const stream = new EventSource(
        `${getConfig().API_URL}/api/v1/announce-stream?token=${encodeURIComponent(token)}`,
      );
      source = stream;

      stream.onopen = () => setState("live");
      stream.onerror = () => {
        setState("reconnecting");
        stream.close();
        if (cancelled) return;
        retryTimer = setTimeout(connect, RECONNECT_DELAY_MS);
      };
      stream.addEventListener("announcement", handleAnnouncement);
    }

    connect();

    return () => {
      cancelled = true;
      clearTimeout(retryTimer);
      source?.close();
    };
  }, []);

  return { announcements, state };
}
