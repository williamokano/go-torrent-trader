import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { formatBytes, formatRatio, formatDate } from "@/utils/format";
import { useToast } from "@/components/toast";
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
 * because it pages one list while showing an unpaged summary alongside it — and
 * because the CSV export it offers is the member's copy of their personal data,
 * which is worth keeping as one self-contained piece.
 */
export function AnnounceLogPanel({ userId, isOwnLog }: AnnounceLogPanelProps) {
  const toast = useToast();

  const [events, setEvents] = useState<AnnounceLogEntry[]>([]);
  const [monthly, setMonthly] = useState<AnnounceLogPeriod[]>([]);
  const [total, setTotal] = useState(0);
  const [retentionDays, setRetentionDays] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [exporting, setExporting] = useState(false);

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

  const handleExport = async () => {
    setExporting(true);
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/users/${userId}/announce-log/export`,
        { headers: token ? { Authorization: `Bearer ${token}` } : {} },
      );
      if (!res.ok) {
        toast.error("Failed to export the announce log");
        return;
      }
      // Fetched rather than linked because the API is token-authenticated: a plain
      // href would arrive without the Authorization header and be rejected.
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `announce-log-${userId}.csv`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch {
      toast.error("Failed to export the announce log");
    } finally {
      setExporting(false);
    }
  };

  const totalPages = Math.ceil(total / PER_PAGE);

  return (
    <div className="announce-log">
      <div className="announce-log__header">
        <p className="announce-log__note">
          {isOwnLog
            ? "Every announce your client makes is recorded, including its IP address and peer ID."
            : "Every announce this member's client makes is recorded, including its IP address and peer ID."}{" "}
          {/* "up to", not "for": the nightly job deletes on a schedule, and rows
              can outlive the window when it is behind or has not run. Promising an
              exact age would be a claim the site cannot keep. */}
          {retentionDays > 0
            ? `Individual announces are kept for up to ${retentionDays} days; the monthly totals below are kept permanently.`
            : "Individual announces are kept indefinitely on this site."}
        </p>
        <button
          className="announce-log__export"
          onClick={handleExport}
          disabled={exporting}
        >
          {exporting ? "Preparing..." : "Download CSV"}
        </button>
      </div>

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
                    <th>From</th>
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
                      <td>
                        {e.ip}:{e.port}
                      </td>
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
