import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { formatBytes, formatRatio, formatDate } from "@/utils/format";
import type { AnnounceLogEntry, AnnounceLogPeriod } from "@/types/announceLog";
import "./announce-log.css";

interface AnnounceLogPanelProps {
  userId: number;
  /** True when the viewer is looking at their own log rather than a member's. */
  isOwnLog: boolean;
}

const PER_PAGE = 25;

/**
 * Shows what the tracker has recorded about a member's client, in two layers: the
 * monthly totals that are kept permanently, and the raw announces that are kept
 * only for the site's retention window.
 *
 * Fetches its own data rather than sharing the profile page's activity state,
 * because it pages one list while showing an unpaged summary alongside it.
 *
 * Read-only and on-screen by design: there is no bulk export, and the announces
 * carry no IP addresses because the tracker does not retain them.
 */
export function AnnounceLogPanel({ userId, isOwnLog }: AnnounceLogPanelProps) {
  const [events, setEvents] = useState<AnnounceLogEntry[]>([]);
  const [monthly, setMonthly] = useState<AnnounceLogPeriod[]>([]);
  const [total, setTotal] = useState(0);
  const [retentionDays, setRetentionDays] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(
    async (targetPage: number) => {
      setLoading(true);
      setError(null);
      try {
        const token = getAccessToken();
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/users/${userId}/announce-log?page=${targetPage}&per_page=${PER_PAGE}`,
          { headers: token ? { Authorization: `Bearer ${token}` } : {} },
        );
        if (!res.ok) {
          setError("Failed to load the announce log");
          return;
        }
        const body = await res.json();
        setEvents(body.events ?? []);
        setMonthly(body.monthly ?? []);
        setTotal(body.total ?? 0);
        setRetentionDays(body.retention_days ?? 0);
      } catch {
        setError("Failed to load the announce log");
      } finally {
        setLoading(false);
      }
    },
    [userId],
  );

  useEffect(() => {
    load(page);
  }, [load, page]);

  const totalPages = Math.ceil(total / PER_PAGE);

  return (
    <div className="announce-log">
      <p className="announce-log__note">
        {isOwnLog
          ? "Every announce your client makes is recorded: what it transferred, and when. No IP addresses are kept."
          : "Every announce this member's client makes is recorded: what it transferred, and when. No IP addresses are kept."}{" "}
        {/* "up to", not "for": the nightly job deletes on a schedule, and rows
            can outlive the window when it is behind or has not run. Promising an
            exact age would be a claim the site cannot keep. */}
        {retentionDays > 0
          ? `Individual announces are kept for up to ${retentionDays} days; the monthly totals below are kept permanently.`
          : "Individual announces are kept indefinitely on this site."}
      </p>

      {error ? (
        <p className="profile-activity__error">{error}</p>
      ) : loading ? (
        <p className="profile-activity__loading">Loading...</p>
      ) : (
        <>
          <h3 className="announce-log__subtitle">Monthly totals</h3>
          {monthly.length === 0 ? (
            <p className="profile-activity__empty">
              No months have been totalled yet. Totals are compiled once a day.
            </p>
          ) : (
            <table className="profile-activity__table">
              <thead>
                <tr>
                  <th>Month</th>
                  <th>Uploaded</th>
                  <th>Downloaded</th>
                  <th>Counted</th>
                  <th>Ratio</th>
                  <th>Announces</th>
                  <th>Seeding</th>
                </tr>
              </thead>
              <tbody>
                {monthly.map((m) => (
                  <tr key={m.year_month}>
                    <td>{m.year_month}</td>
                    <td>{formatBytes(m.uploaded)}</td>
                    <td>{formatBytes(m.downloaded)}</td>
                    <td>{formatBytes(m.counted_downloaded)}</td>
                    <td>{formatRatio(m.ratio)}</td>
                    <td>{m.announces}</td>
                    <td>{m.seed_announces}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          <h3 className="announce-log__subtitle">Recent announces</h3>
          {events.length === 0 ? (
            <p className="profile-activity__empty">No announces recorded.</p>
          ) : (
            <>
              <table className="profile-activity__table">
                <thead>
                  <tr>
                    <th>When</th>
                    <th>Torrent</th>
                    <th>Event</th>
                    <th>Up</th>
                    <th>Down</th>
                    <th>Counted</th>
                    <th>Seeding</th>
                  </tr>
                </thead>
                <tbody>
                  {events.map((e) => (
                    <tr key={e.id}>
                      <td>{formatDate(e.announced_at)}</td>
                      <td>
                        {e.torrent_id ? (
                          <Link to={`/torrent/${e.torrent_id}`}>
                            {e.torrent_name}
                          </Link>
                        ) : (
                          e.torrent_name
                        )}
                      </td>
                      <td>{e.event}</td>
                      <td>{formatBytes(e.uploaded_delta)}</td>
                      <td>{formatBytes(e.downloaded_delta)}</td>
                      <td>{formatBytes(e.counted_downloaded_delta)}</td>
                      <td>{e.seeder ? "Yes" : "No"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {totalPages > 1 && (
                <div className="profile-activity__pagination">
                  <button
                    className="profile-activity__page-btn"
                    disabled={page <= 1}
                    onClick={() => setPage(page - 1)}
                  >
                    Previous
                  </button>
                  <span className="profile-activity__page-info">
                    Page {page} of {totalPages}
                  </span>
                  <button
                    className="profile-activity__page-btn"
                    disabled={page >= totalPages}
                    onClick={() => setPage(page + 1)}
                  >
                    Next
                  </button>
                </div>
              )}
            </>
          )}
        </>
      )}
    </div>
  );
}
