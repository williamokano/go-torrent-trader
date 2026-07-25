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

export type StreamState = "connecting" | "live" | "reconnecting";

/**
 * The feed is capped rather than unbounded: a page left open for a week should
 * not accumulate every release ever announced.
 */
const MAX_ITEMS = 100;

interface AnnounceStream {
  announcements: Announcement[];
  state: StreamState;
}

/**
 * Subscribes to the live announcement stream.
 *
 * EventSource is used rather than a WebSocket because the feed is one-way and
 * the browser handles reconnection for us. It cannot set headers, which is why
 * the token goes in the query string — the same trade the chat socket makes.
 */
export function useAnnounceStream(): AnnounceStream {
  const [announcements, setAnnouncements] = useState<Announcement[]>([]);
  const [state, setState] = useState<StreamState>("connecting");
  // Tracked in a ref rather than derived from state, so the message handler
  // does not need to be re-created (and the stream re-opened) on every event.
  const seen = useRef<Set<number>>(new Set());

  useEffect(() => {
    const token = getAccessToken();
    if (!token) return;

    const source = new EventSource(
      `${getConfig().API_URL}/api/v1/announce-stream?token=${encodeURIComponent(token)}`,
    );

    source.onopen = () => setState("live");

    // EventSource retries on its own; onerror fires for each failed attempt, so
    // this reports the state rather than trying to reconnect.
    source.onerror = () => setState("reconnecting");

    source.addEventListener("announcement", (event) => {
      let announcement: Announcement;
      try {
        announcement = JSON.parse((event as MessageEvent).data);
      } catch {
        return; // A frame we cannot read is not worth breaking the feed over.
      }

      setState("live");
      setAnnouncements((previous) => {
        // A reconnect can replay an announcement the page already has, and a
        // coalesced summary shares its representative row's torrent id.
        if (
          !announcement.coalesced &&
          seen.current.has(announcement.torrent_id)
        ) {
          return previous;
        }
        if (!announcement.coalesced) {
          seen.current.add(announcement.torrent_id);
        }
        return [announcement, ...previous].slice(0, MAX_ITEMS);
      });
    });

    return () => source.close();
  }, []);

  return { announcements, state };
}
