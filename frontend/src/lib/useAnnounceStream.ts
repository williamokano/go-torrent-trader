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
  /**
   * Increments on every dropped stream. The page watches it to re-check access:
   * a revoked privilege closes the stream, and the refusal is only readable on
   * an endpoint that can return a status code.
   */
  reconnects: number;
}

/**
 * Subscribes to the live announcement stream.
 *
 * EventSource is used rather than a WebSocket because the feed is one-way. Its
 * built-in retry is deliberately not relied on, though: it always reconnects to
 * the URL it was constructed with, and the token is in that URL. After a token
 * refresh the browser would retry a request the server must reject, forever. So
 * a failed stream is torn down and rebuilt with a freshly read token.
 *
 * `feed` is the slug of the feed to watch. Changing it tears the stream down and
 * opens the other one, and clears what is on screen — the announcements shown
 * must be the ones this feed carries, not a mix of both.
 */
interface StreamOptions {
  /** When false no stream is opened at all — e.g. the member may not watch. */
  enabled?: boolean;
}

export function useAnnounceStream(
  feed?: string,
  { enabled = true }: StreamOptions = {},
): AnnounceStream {
  // The feed each list belongs to travels with it. React runs an effect cleanup
  // after paint, so the previous feed's EventSource is still attached for a
  // moment after the reset — a frame arriving in that window would otherwise be
  // appended to the new feed's freshly cleared list, under the new feed's name.
  // Stamping the list lets the old handler recognise that it is stale.
  const [announcements, setAnnouncements] = useState<{
    feed?: string;
    items: FeedItem[];
  }>({ feed, items: [] });
  const [state, setState] = useState<StreamState>("connecting");
  const [reconnects, setReconnects] = useState(0);
  // A ref rather than state: the message handler must not be re-created (and the
  // stream re-opened) on every event.
  const nextKey = useRef(0);

  // A feed change is a different subscription, so what is on screen belongs to
  // the feed that is gone. The dedupe set goes with it: an announcement this
  // feed has not shown must not be suppressed because the other one had.
  //
  // Reset during render rather than in an effect: React applies it before
  // anything is painted, so the old feed's rows are never briefly shown under
  // the new feed's name — and an effect doing this would cascade a second
  // render for every switch.
  const [watching, setWatching] = useState(feed);
  if (watching !== feed) {
    setWatching(feed);
    setAnnouncements({ feed, items: [] });
    setState("connecting");
  }

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
        // A frame from the feed this page has already left.
        if (previous.feed !== feed) return previous;

        // A reconnect can replay an announcement the page already has. A
        // coalesced summary is exempt: it shares its representative row's id
        // but is a different thing to show.
        //
        // Read off the list rather than kept in a parallel set: the list is
        // capped, so this is bounded for free, it cannot drift from what is on
        // screen, and switching feeds clears the dedupe by clearing the list.
        const duplicate =
          !announcement.coalesced &&
          previous.items.some(
            (item) =>
              !item.coalesced && item.torrent_id === announcement.torrent_id,
          );
        if (duplicate) return previous;

        nextKey.current += 1;
        return {
          feed: previous.feed,
          items: [
            { ...announcement, key: nextKey.current },
            ...previous.items,
          ].slice(0, MAX_ITEMS),
        };
      });
    }

    function connect() {
      if (!enabled) return;
      const token = getAccessToken();
      if (!token) return;

      // Omitting the slug hits the legacy route, which resolves to the default
      // feed — the same thing the page did before feeds existed.
      const path = feed
        ? `/api/v1/announce-stream/${encodeURIComponent(feed)}`
        : "/api/v1/announce-stream";
      const stream = new EventSource(
        `${getConfig().API_URL}${path}?token=${encodeURIComponent(token)}`,
      );
      source = stream;

      stream.onopen = () => setState("live");
      stream.onerror = () => {
        setState("reconnecting");
        setReconnects((n) => n + 1);
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
  }, [feed, enabled]);

  return { announcements: announcements.items, state, reconnects };
}
