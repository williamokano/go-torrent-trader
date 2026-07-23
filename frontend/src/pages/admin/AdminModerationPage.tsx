import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { Pagination } from "@/components/Pagination";
import { formatBytes, timeAgo } from "@/utils/format";
import type { ModeratedTorrent } from "@/types/moderation";
import "./admin-ui.css";

const PER_PAGE = 25;

type AssignedFilter = "all" | "unassigned" | "mine";

const FILTERS: { key: AssignedFilter; label: string }[] = [
  { key: "all", label: "All pending" },
  { key: "unassigned", label: "Unassigned" },
  { key: "mine", label: "Assigned to me" },
];

export function AdminModerationPage() {
  const toast = useToast();

  const [torrents, setTorrents] = useState<ModeratedTorrent[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [assigned, setAssigned] = useState<AssignedFilter>("all");
  const [loading, setLoading] = useState(true);
  const [claimingId, setClaimingId] = useState<number | null>(null);

  const fetchQueue = useCallback(async () => {
    setLoading(true);
    const token = getAccessToken();
    const params = new URLSearchParams();
    params.set("page", String(page));
    params.set("per_page", String(PER_PAGE));
    params.set("assigned", assigned);

    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/moderation/torrents?${params}`,
        { headers: { Authorization: `Bearer ${token}` } },
      );
      if (res.ok) {
        const data = await res.json();
        setTorrents(data.torrents ?? []);
        setTotal(data.total ?? 0);
      } else {
        const data = await res.json().catch(() => null);
        toast.error(data?.error?.message ?? "Failed to load moderation queue");
      }
    } finally {
      setLoading(false);
    }
  }, [page, assigned, toast]);

  useEffect(() => {
    fetchQueue();
  }, [fetchQueue]);

  // Reset to page 1 when switching filters.
  const selectFilter = (key: AssignedFilter) => {
    setAssigned(key);
    setPage(1);
  };

  const handleClaim = async (id: number) => {
    setClaimingId(id);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/moderation/torrents/${id}/claim`,
        { method: "POST", headers: { Authorization: `Bearer ${token}` } },
      );
      if (res.ok) {
        toast.success("Claimed for review");
        fetchQueue();
      } else {
        const data = await res.json().catch(() => null);
        toast.error(data?.error?.message ?? "Failed to claim torrent");
      }
    } finally {
      setClaimingId(null);
    }
  };

  const totalPages = Math.ceil(total / PER_PAGE);

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title">Moderation Queue</h1>
          <p className="admin-page-header__desc">
            Torrents awaiting approval. Claim one to review it, then approve or
            reject from the torrent page.
          </p>
        </div>
      </div>

      <div
        className="admin-toolbar"
        style={{ marginBottom: "var(--space-md)" }}
      >
        {FILTERS.map((f) => (
          <button
            key={f.key}
            className={`admin-btn admin-btn--sm ${
              assigned === f.key ? "admin-btn--primary" : "admin-btn--ghost"
            }`}
            onClick={() => selectFilter(f.key)}
          >
            {f.label}
          </button>
        ))}
      </div>

      {loading ? (
        <p>Loading...</p>
      ) : torrents.length === 0 ? (
        <div className="admin-panel">
          <p className="admin-empty">Nothing awaiting moderation here.</p>
        </div>
      ) : (
        <>
          <div className="admin-panel">
            <div className="admin-table-scroll">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>Torrent</th>
                    <th>Uploader</th>
                    <th>Category</th>
                    <th>Size</th>
                    <th>Submitted</th>
                    <th>Moderator</th>
                    <th>Msgs</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {torrents.map((t) => {
                    const mod = t.moderation;
                    return (
                      <tr key={t.id}>
                        <td className="admin-table__name">
                          <Link to={`/torrent/${t.id}`}>{t.name}</Link>
                        </td>
                        <td>
                          {t.anonymous ? (
                            "Anonymous"
                          ) : t.uploader_name ? (
                            <Link to={`/user/${t.uploader_name}`}>
                              {t.uploader_name}
                            </Link>
                          ) : (
                            "Unknown"
                          )}
                        </td>
                        <td>{t.category_name ?? "—"}</td>
                        <td>{formatBytes(t.size ?? 0)}</td>
                        <td className="admin-muted">
                          {t.created_at ? timeAgo(t.created_at) : "—"}
                        </td>
                        <td>
                          {mod?.assigned_moderator_id ? (
                            (mod.assigned_moderator_name ??
                            `#${mod.assigned_moderator_id}`)
                          ) : (
                            <span className="admin-muted">Unassigned</span>
                          )}
                        </td>
                        <td>{mod?.message_count ?? 0}</td>
                        <td className="admin-table__actions">
                          {!mod?.assigned_moderator_id && (
                            <button
                              className="admin-btn admin-btn--ghost admin-btn--sm"
                              disabled={claimingId === t.id}
                              onClick={() => handleClaim(t.id)}
                            >
                              {claimingId === t.id ? "Claiming..." : "Claim"}
                            </button>
                          )}
                          <Link
                            to={`/torrent/${t.id}`}
                            className="admin-btn admin-btn--sm admin-btn--primary"
                          >
                            Review
                          </Link>
                        </td>
                      </tr>
                    );
                  })}
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
    </div>
  );
}
