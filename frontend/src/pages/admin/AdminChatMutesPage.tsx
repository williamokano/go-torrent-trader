import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { timeAgo } from "@/utils/format";
import { Pagination } from "@/components/Pagination";
import "./admin-ui.css";
import "./admin-chat-mutes.css";

interface ChatMute {
  id: number;
  user_id: number;
  username: string;
  muted_by: number | null;
  muted_by_name: string | null;
  reason: string;
  expires_at: string;
  created_at: string;
}

const PER_PAGE = 25;

export function AdminChatMutesPage() {
  const toast = useToast();

  const [mutes, setMutes] = useState<ChatMute[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [unmutingId, setUnmutingId] = useState<number | null>(null);

  const fetchMutes = useCallback(async () => {
    setLoading(true);
    const token = getAccessToken();
    const params = new URLSearchParams();
    params.set("page", String(page));
    params.set("per_page", String(PER_PAGE));

    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/chat/mutes?${params}`,
        { headers: { Authorization: `Bearer ${token}` } },
      );
      if (res.ok) {
        const data = await res.json();
        setMutes(data.mutes ?? []);
        setTotal(data.total ?? 0);
      }
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    fetchMutes();
  }, [fetchMutes]);

  const handleUnmute = async (userId: number, username: string) => {
    setUnmutingId(userId);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/chat/users/${userId}/mute`,
        {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        },
      );

      if (res.ok || res.status === 204) {
        toast.success(`${username} has been unmuted`);
        fetchMutes();
      } else {
        const data = await res.json();
        toast.error(data?.error?.message ?? "Failed to unmute user");
      }
    } finally {
      setUnmutingId(null);
    }
  };

  const totalPages = Math.ceil(total / PER_PAGE);

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title">Chat Mutes</h1>
          <p className="admin-page-header__desc">
            Members currently muted in the shoutbox, and when their mute ends.
          </p>
        </div>
      </div>

      {loading ? (
        <p>Loading...</p>
      ) : mutes.length === 0 ? (
        <div className="admin-panel">
          <p className="admin-empty">No active chat mutes.</p>
        </div>
      ) : (
        <>
          <div className="admin-panel">
            <div className="admin-table-scroll">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>Username</th>
                    <th>Reason</th>
                    <th>Muted By</th>
                    <th>Expires</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {mutes.map((m) => (
                    <tr key={m.id}>
                      <td className="admin-table__name">
                        <Link to={`/user/${m.user_id}`}>{m.username}</Link>
                      </td>
                      <td
                        className="admin-chat-mutes__reason-cell"
                        title={m.reason}
                      >
                        {m.reason || "—"}
                      </td>
                      <td>
                        {m.muted_by_name ??
                          (m.muted_by ? `#${m.muted_by}` : "System")}
                      </td>
                      <td className="admin-muted">{timeAgo(m.expires_at)}</td>
                      <td className="admin-table__actions">
                        <button
                          className="admin-btn admin-btn--ghost admin-btn--sm"
                          disabled={unmutingId === m.user_id}
                          onClick={() => handleUnmute(m.user_id, m.username)}
                        >
                          {unmutingId === m.user_id ? "Unmuting..." : "Unmute"}
                        </button>
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
    </div>
  );
}
