import { useCallback, useEffect, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { timeAgo } from "@/utils/format";
import { Pagination } from "@/components/Pagination";
import { UsernameDisplay } from "@/components/UsernameDisplay";
import { ConfirmModal } from "@/components/modal";
import "./admin-ui.css";
import "./admin-cheat-flags.css";

interface CheatFlag {
  id: number;
  user_id: number;
  torrent_id: number | null;
  flag_type: string;
  details: string;
  dismissed: boolean;
  dismissed_by: number | null;
  dismissed_at: string | null;
  created_at: string;
  username: string;
  torrent_name: string;
  dismisser_name: string;
}

const PER_PAGE = 25;

const FLAG_TYPE_OPTIONS = [
  { value: "", label: "All Types" },
  { value: "impossible_upload_speed", label: "Impossible Upload Speed" },
  { value: "upload_no_downloaders", label: "Upload Without Downloaders" },
  { value: "left_mismatch", label: "Left Mismatch" },
];

const DISMISSED_OPTIONS = [
  { value: "", label: "All" },
  { value: "false", label: "Active" },
  { value: "true", label: "Dismissed" },
];

function flagTypeLabel(type: string): string {
  switch (type) {
    case "impossible_upload_speed":
      return "Impossible Upload Speed";
    case "upload_no_downloaders":
      return "Upload Without Downloaders";
    case "left_mismatch":
      return "Left Mismatch";
    default:
      return type;
  }
}

function StatusBadge({ dismissed }: { dismissed: boolean }) {
  return (
    <span
      className={`admin-badge ${dismissed ? "admin-badge--muted" : "admin-badge--warn"}`}
    >
      {dismissed ? "Dismissed" : "Active"}
    </span>
  );
}

function formatDetails(details: string): string {
  try {
    const parsed = JSON.parse(details);
    return Object.entries(parsed)
      .map(([k, v]) => `${k}: ${v}`)
      .join(", ");
  } catch {
    return details;
  }
}

export function AdminCheatFlagsPage() {
  const toast = useToast();

  const [flags, setFlags] = useState<CheatFlag[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [flagTypeFilter, setFlagTypeFilter] = useState("");
  const [dismissedFilter, setDismissedFilter] = useState("false");

  const [dismissingId, setDismissingId] = useState<number | null>(null);
  const [dismissing, setDismissing] = useState(false);

  const fetchFlags = useCallback(async () => {
    setLoading(true);
    setError(null);
    const token = getAccessToken();
    const params = new URLSearchParams();
    params.set("page", String(page));
    params.set("per_page", String(PER_PAGE));
    if (flagTypeFilter) params.set("flag_type", flagTypeFilter);
    if (dismissedFilter) params.set("dismissed", dismissedFilter);

    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/cheat-flags?${params}`,
        { headers: { Authorization: `Bearer ${token}` } },
      );
      if (res.ok) {
        const data = await res.json();
        setFlags(data.cheat_flags ?? []);
        setTotal(data.total ?? 0);
      } else {
        const data = await res.json().catch(() => null);
        setError(data?.error?.message ?? "Failed to load cheat flags");
      }
    } catch {
      setError("Failed to load cheat flags");
    } finally {
      setLoading(false);
    }
  }, [page, flagTypeFilter, dismissedFilter]);

  useEffect(() => {
    fetchFlags();
  }, [fetchFlags]);

  const handleDismiss = async () => {
    if (dismissingId === null) return;

    setDismissing(true);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/cheat-flags/${dismissingId}/dismiss`,
        {
          method: "PUT",
          headers: { Authorization: `Bearer ${token}` },
        },
      );

      if (res.ok) {
        toast.success("Cheat flag dismissed");
        setDismissingId(null);
        fetchFlags();
      } else {
        const data = await res.json();
        toast.error(data?.error?.message ?? "Failed to dismiss cheat flag");
      }
    } finally {
      setDismissing(false);
    }
  };

  const totalPages = Math.ceil(total / PER_PAGE);

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title">Cheat Flags</h1>
          <p className="admin-page-header__desc">
            Suspicious announce activity detected by the tracker, ready for
            review.
          </p>
        </div>
      </div>

      <div className="admin-toolbar">
        <div className="admin-cheat-flags__filter">
          <label htmlFor="cheat-flag-type">Type</label>
          <select
            id="cheat-flag-type"
            value={flagTypeFilter}
            onChange={(e) => {
              setFlagTypeFilter(e.target.value);
              setPage(1);
            }}
          >
            {FLAG_TYPE_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>

        <div className="admin-cheat-flags__filter">
          <label htmlFor="cheat-flag-status">Status</label>
          <select
            id="cheat-flag-status"
            value={dismissedFilter}
            onChange={(e) => {
              setDismissedFilter(e.target.value);
              setPage(1);
            }}
          >
            {DISMISSED_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>
      </div>

      {loading ? (
        <p>Loading...</p>
      ) : error ? (
        <div className="admin-panel">
          <p className="admin-empty">{error}</p>
        </div>
      ) : flags.length === 0 ? (
        <div className="admin-panel">
          <p className="admin-empty">No cheat flags found.</p>
        </div>
      ) : (
        <>
          <div className="admin-panel">
            <div className="admin-table-scroll">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>User</th>
                    <th>Torrent</th>
                    <th>Type</th>
                    <th>Details</th>
                    <th>Status</th>
                    <th>Date</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {flags.map((f) => (
                    <tr key={f.id}>
                      <td className="admin-table__name">
                        <UsernameDisplay
                          userId={f.user_id}
                          username={f.username}
                        />
                      </td>
                      <td className="admin-cheat-flags__torrent-cell">
                        {f.torrent_id ? (
                          <a href={`/torrent/${f.torrent_id}`}>
                            {f.torrent_name || `#${f.torrent_id}`}
                          </a>
                        ) : (
                          "—"
                        )}
                      </td>
                      <td>{flagTypeLabel(f.flag_type)}</td>
                      <td
                        className="admin-cheat-flags__details-cell"
                        title={f.details}
                      >
                        {formatDetails(f.details)}
                      </td>
                      <td>
                        <StatusBadge dismissed={f.dismissed} />
                        {f.dismissed && f.dismisser_name && (
                          <div className="admin-cheat-flags__dismissed-info">
                            by {f.dismisser_name}
                          </div>
                        )}
                      </td>
                      <td className="admin-muted">{timeAgo(f.created_at)}</td>
                      <td className="admin-table__actions">
                        {!f.dismissed && (
                          <button
                            className="admin-btn admin-btn--ghost admin-btn--sm"
                            disabled={dismissing}
                            onClick={() => setDismissingId(f.id)}
                          >
                            Dismiss
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {totalPages > 1 && (
            <div style={{ marginTop: "var(--space-md)" }}>
              <Pagination
                currentPage={page}
                totalPages={totalPages}
                onPageChange={setPage}
              />
            </div>
          )}
        </>
      )}

      <ConfirmModal
        isOpen={dismissingId !== null}
        title="Dismiss Cheat Flag"
        message="Are you sure you want to dismiss this cheat flag? This action cannot be undone."
        confirmLabel="Dismiss"
        loading={dismissing}
        onConfirm={handleDismiss}
        onCancel={() => !dismissing && setDismissingId(null)}
      />
    </div>
  );
}
