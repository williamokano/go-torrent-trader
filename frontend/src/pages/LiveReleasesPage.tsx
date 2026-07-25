import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useAnnounceStream } from "@/lib/useAnnounceStream";
import type { FeedItem, StreamState } from "@/lib/useAnnounceStream";
import { formatBytes, timeAgo } from "@/utils/format";
import "./live-releases.css";

/** One subscribable feed, as the API lists it. */
interface Feed {
  slug: string;
  name: string;
}

/** The feed the unslugged legacy stream URL resolves to, server-side. */
const LEGACY_FEED = "default";

/**
 * Resolves which feed the page is actually watching.
 *
 * Everything on screen has to agree with the stream that is open. Showing one
 * feed's name over another feed's announcements is worse than showing none —
 * on a private tracker it would mean reading "safe for work" while the
 * unfiltered feed streams in.
 *
 * So the picker and the subscription both come from here:
 *   - the feed asked for in the URL, taken at its word until the list has
 *     loaded, so a link opens the feed it names without first connecting to
 *     another one;
 *   - once the list is in, that feed only if it still exists;
 *   - otherwise the one the legacy route resolves to, so a plain /live matches
 *     what an old bookmark and an already-open tab get;
 *   - otherwise the first feed, for an install that has no "default";
 *   - and undefined for a plain /live with no list, which keeps the legacy
 *     route working when the feed list cannot be fetched at all.
 */
function resolveFeed(feeds: Feed[], requested?: string) {
  if (feeds.length === 0) return requested;
  if (requested && feeds.some((feed) => feed.slug === requested)) {
    return requested;
  }
  return feeds.find((feed) => feed.slug === LEGACY_FEED)?.slug ?? feeds[0].slug;
}

const STATE_LABELS: Record<StreamState, string> = {
  connecting: "connecting",
  live: "live",
  reconnecting: "reconnecting",
};

export function LiveReleasesPage() {
  const [feeds, setFeeds] = useState<Feed[]>([]);
  // EventSource cannot read a status code — a 403 reaches it as an
  // indistinguishable onerror, so the page would retry every few seconds
  // forever and show "reconnecting" to someone who is never getting in. The
  // feed list answers with a real status, so that is where access is read.
  const [denied, setDenied] = useState(false);
  // The feed lives in the URL rather than in state, so a feed can be linked to
  // and the back button moves between them.
  const [searchParams, setSearchParams] = useSearchParams();
  const requested = searchParams.get("feed") ?? undefined;
  const selected = resolveFeed(feeds, requested);
  // A feed named in the URL that no longer exists: the admin renamed or removed
  // it. Falling back silently would leave the page reading as though nothing had
  // happened while showing a different feed's releases.
  const missing = Boolean(
    requested && feeds.length > 0 && selected !== requested,
  );

  // No stream is opened for a member who may not watch: it would be refused,
  // and retried, and refused again.
  const { announcements, state, reconnects } = useAnnounceStream(
    denied ? undefined : selected,
    {
      enabled: !denied,
    },
  );

  // Re-read on every reconnect, not only at mount. Access can be revoked while
  // the page is open: the server closes the stream, and without this the page
  // would retry every few seconds forever, reading "reconnecting" to someone who
  // is never getting back in. It recovers the same way — access restored, the
  // next attempt succeeds and the denial clears.
  useEffect(() => {
    let cancelled = false;

    async function fetchFeeds() {
      try {
        const token = getAccessToken();
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/announce-feeds`,
          token ? { headers: { Authorization: `Bearer ${token}` } } : undefined,
        );
        if (res.status === 403) {
          // Distinguished by code rather than status alone: a future middleware
          // could 403 for something that has nothing to do with feed access.
          const body = await res.json().catch(() => null);
          if (!cancelled && body?.error?.code === "forbidden") setDenied(true);
          return;
        }
        if (!res.ok) {
          // A server-side failure is not a refusal — keep whatever the page has
          // and let the stream retry.
          return;
        }
        const data = await res.json();
        if (!cancelled) {
          setDenied(false);
          setFeeds(data.feeds ?? []);
        }
      } catch {
        // The picker is an extra: without it the page still watches the default
        // feed, which is what it did before feeds existed.
      }
    }
    fetchFeeds();

    return () => {
      cancelled = true;
    };
  }, [reconnects]);

  return (
    <div className="live">
      <div className="live__header">
        <div>
          <h1 className="live__title">Live Releases</h1>
          <p className="live__desc">
            New torrents appear here the moment they are published. Nothing is
            kept between visits — this is a window on what is happening now.
          </p>
        </div>
        <div className="live__controls">
          {!denied && feeds.length > 1 && (
            <label className="live__feed">
              <span className="live__feed-label">Feed</span>
              <select
                className="live__feed-select"
                value={selected}
                onChange={(e) =>
                  // Functional form: the feed is the only query parameter today,
                  // and replacing the whole string would silently drop the next
                  // one added.
                  setSearchParams((previous) => {
                    previous.set("feed", e.target.value);
                    return previous;
                  })
                }
              >
                {feeds.map((feed) => (
                  <option key={feed.slug} value={feed.slug}>
                    {feed.name}
                  </option>
                ))}
              </select>
            </label>
          )}
          {!denied && (
            <span
              className={`live__status live__status--${state}`}
              role="status"
              aria-live="polite"
            >
              {STATE_LABELS[state]}
            </span>
          )}
        </div>
      </div>

      {denied ? (
        <p className="live__denied" role="status" aria-live="polite">
          Your account does not have access to the live feeds. Ask a staff
          member if you think that is wrong.
        </p>
      ) : null}

      {!denied && missing && (
        <p className="live__missing" role="status">
          That feed no longer exists — showing{" "}
          {feeds.find((feed) => feed.slug === selected)?.name ?? selected}{" "}
          instead.
        </p>
      )}

      {denied ? null : announcements.length === 0 ? (
        <p className="live__empty">Waiting for releases…</p>
      ) : (
        <ul className="live__list">
          {announcements.map((announcement) => (
            <li key={announcement.key} className="live__item">
              <ReleaseRow announcement={announcement} />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function ReleaseRow({ announcement }: { announcement: FeedItem }) {
  if (announcement.coalesced) {
    return (
      <div className="live__summary">
        {announcement.coalesced} more releases published — see{" "}
        <Link to="/browse">browse</Link>.
      </div>
    );
  }

  return (
    <>
      <Link to={`/torrent/${announcement.torrent_id}`} className="live__name">
        {announcement.name}
      </Link>
      <div className="live__meta">
        <span className="live__category">{announcement.category}</span>
        <span className="live__size">{formatBytes(announcement.size)}</span>
        {announcement.freeleech && (
          <span className="live__badge live__badge--freeleech">Freeleech</span>
        )}
        <span className="live__uploader">by {announcement.uploader}</span>
        <span
          className="live__time"
          title={new Date(announcement.published_at).toLocaleString()}
        >
          {timeAgo(announcement.published_at)}
        </span>
      </div>
    </>
  );
}
