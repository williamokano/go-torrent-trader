import { Link } from "react-router-dom";
import { useAnnounceStream } from "@/lib/useAnnounceStream";
import type { Announcement, StreamState } from "@/lib/useAnnounceStream";
import { formatBytes, timeAgo } from "@/utils/format";
import "./live-releases.css";

const STATE_LABELS: Record<StreamState, string> = {
  connecting: "connecting",
  live: "live",
  reconnecting: "reconnecting",
};

export function LiveReleasesPage() {
  const { announcements, state } = useAnnounceStream();

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
        <span
          className={`live__status live__status--${state}`}
          role="status"
          aria-live="polite"
        >
          {STATE_LABELS[state]}
        </span>
      </div>

      {announcements.length === 0 ? (
        <p className="live__empty">Waiting for releases…</p>
      ) : (
        <ul className="live__list">
          {announcements.map((announcement, index) => (
            <li
              // A coalesced summary shares its representative's torrent id, so
              // the id alone is not unique; the index keeps the list keys
              // stable because entries are only ever prepended.
              key={`${announcement.torrent_id}-${index}`}
              className="live__item"
            >
              <ReleaseRow announcement={announcement} />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function ReleaseRow({ announcement }: { announcement: Announcement }) {
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
