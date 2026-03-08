import { useCallback, useEffect, useState } from "react";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useAuth } from "@/features/auth";
import { Input } from "@/components/form";
import { Pagination } from "@/components/Pagination";
import { formatDate } from "@/utils/format";
import "./invites.css";

interface Invite {
  id: number;
  email: string;
  status: "pending" | "redeemed" | "expired";
  expires_at: string;
  created_at: string;
  invitee_id?: number;
  redeemed_at?: string;
}

const PER_PAGE = 25;

export function InvitesPage() {
  const { user } = useAuth();
  const [invites, setInvites] = useState<Invite[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [email, setEmail] = useState("");
  const [sending, setSending] = useState(false);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  const fetchInvites = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const token = getAccessToken();
      const params = new URLSearchParams();
      params.set("page", String(page));
      params.set("per_page", String(PER_PAGE));

      const res = await fetch(
        `${getConfig().API_URL}/api/v1/invites?${params.toString()}`,
        {
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        },
      );

      const body = await res.json();

      if (!res.ok) {
        setError(body?.error?.message ?? "Failed to load invites");
        setLoading(false);
        return;
      }

      setInvites(body?.invites ?? []);
      setTotal(body?.total ?? 0);
    } catch {
      setError("Failed to load invites");
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    fetchInvites();
  }, [fetchInvites]);

  const totalPages = Math.max(1, Math.ceil(total / PER_PAGE));

  const handleSendInvite = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim() || sending) return;

    setSending(true);
    setError(null);
    setSuccessMsg(null);

    try {
      const token = getAccessToken();
      const res = await fetch(`${getConfig().API_URL}/api/v1/invites`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({ email: email.trim() }),
      });

      const body = await res.json();

      if (!res.ok) {
        setError(body?.error?.message ?? "Failed to send invite");
        return;
      }

      setSuccessMsg(`Invite sent to ${email.trim()}`);
      setEmail("");
      fetchInvites();
    } catch {
      setError("Failed to send invite");
    } finally {
      setSending(false);
    }
  };

  return (
    <div className="invites">
      <div className="invites__header">
        <h1 className="invites__title">Invitations</h1>
        <span className="invites__remaining">
          Remaining invites: {user?.invites ?? 0}
        </span>
      </div>

      {(user?.invites ?? 0) > 0 && (
        <form className="invites__form" onSubmit={handleSendInvite}>
          <div className="invites__form-input">
            <Input
              label="Email"
              type="email"
              placeholder="Enter email address..."
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </div>
          <button
            type="submit"
            className="invites__form-btn"
            disabled={sending || !email.trim()}
          >
            {sending ? "Sending..." : "Send Invite"}
          </button>
        </form>
      )}

      {successMsg && <div className="invites__success">{successMsg}</div>}
      {error && <div className="invites__error">{error}</div>}

      {loading ? (
        <div className="invites__loading">Loading invites...</div>
      ) : invites.length === 0 ? (
        <div className="invites__empty">No invites sent yet.</div>
      ) : (
        <table className="invites__table">
          <thead>
            <tr>
              <th>Email</th>
              <th>Status</th>
              <th>Sent</th>
              <th>Expires</th>
            </tr>
          </thead>
          <tbody>
            {invites.map((inv) => (
              <tr key={inv.id}>
                <td>{inv.email}</td>
                <td>
                  <span className={`invites__status--${inv.status}`}>
                    {inv.status}
                  </span>
                </td>
                <td>{formatDate(inv.created_at)}</td>
                <td>{formatDate(inv.expires_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {!loading && !error && totalPages > 1 && (
        <Pagination
          currentPage={page}
          totalPages={totalPages}
          onPageChange={setPage}
        />
      )}
    </div>
  );
}
