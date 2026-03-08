import { useCallback, useEffect, useState } from "react";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useAuth } from "@/features/auth";
import { Pagination } from "@/components/Pagination";
import { formatDate, formatBytes, formatRatio } from "@/utils/format";
import "./invites.css";

interface InviteeView {
  id: number;
  username: string;
  uploaded: number;
  downloaded: number;
  ratio: number;
  enabled: boolean;
  warned: boolean;
  created_at: string;
}

interface Invite {
  id: number;
  token: string;
  status: "pending" | "redeemed" | "expired";
  expires_at: string;
  created_at: string;
  invitee_id?: number;
  invitee_name?: string;
  invitee?: InviteeView;
  redeemed_at?: string;
}

const PER_PAGE = 25;

function getInviteLink(token: string): string {
  return `${window.location.origin}/signup?invite=${token}`;
}

export function InvitesPage() {
  const { user, refreshUser } = useAuth();
  const [invites, setInvites] = useState<Invite[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [generating, setGenerating] = useState(false);
  const [generatedLink, setGeneratedLink] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [copiedAction, setCopiedAction] = useState<string | null>(null);

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

  const handleGenerateInvite = async () => {
    if (generating) return;

    setGenerating(true);
    setError(null);
    setGeneratedLink(null);
    setCopied(false);

    try {
      const token = getAccessToken();
      const res = await fetch(`${getConfig().API_URL}/api/v1/invites`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
      });

      const body = await res.json();

      if (!res.ok) {
        setError(body?.error?.message ?? "Failed to generate invite");
        return;
      }

      const inviteToken = body?.invite?.token;
      if (inviteToken) {
        setGeneratedLink(getInviteLink(inviteToken));
      }
      fetchInvites();
      refreshUser();
    } catch {
      setError("Failed to generate invite");
    } finally {
      setGenerating(false);
    }
  };

  const handleCopy = async () => {
    if (!generatedLink) return;
    try {
      await navigator.clipboard.writeText(generatedLink);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback: select the text
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

      <div className="invites__generate">
        {(user?.invites ?? 0) > 0 ? (
          <button
            type="button"
            className="invites__form-btn"
            disabled={generating}
            onClick={handleGenerateInvite}
          >
            {generating ? "Generating..." : "Generate Invite"}
          </button>
        ) : (
          <p className="invites__no-invites">
            You have no invites available. Staff can grant invites from the
            admin panel.
          </p>
        )}
      </div>

      {generatedLink && (
        <div className="invites__generated-link">
          <span className="invites__generated-link-label">Invite link:</span>
          <code className="invites__generated-link-url">{generatedLink}</code>
          <button
            type="button"
            className="invites__copy-btn"
            onClick={handleCopy}
          >
            {copied ? "Copied!" : "Copy"}
          </button>
        </div>
      )}

      {error && <div className="invites__error">{error}</div>}

      {loading ? (
        <div className="invites__loading">Loading invites...</div>
      ) : invites.length === 0 ? (
        <div className="invites__empty">No invites created yet.</div>
      ) : (
        <div className="invites__table-wrapper">
          <table className="invites__table">
            <thead>
              <tr>
                <th>Code</th>
                <th>Status</th>
                <th>Invitee</th>
                <th>Up</th>
                <th>Down</th>
                <th>%</th>
                <th>Joined</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {invites.map((inv) => (
                <tr key={inv.id}>
                  <td>
                    <code className="invites__token">{inv.token}</code>
                  </td>
                  <td>
                    <span className={`invites__status--${inv.status}`}>
                      {inv.status}
                    </span>
                  </td>
                  <td>
                    {inv.invitee ? (
                      <span>
                        <a href={`/user/${inv.invitee.id}`}>
                          {inv.invitee.username}
                        </a>
                        {!inv.invitee.enabled && (
                          <span className="invites__badge--banned">
                            {" "}
                            Banned
                          </span>
                        )}
                        {inv.invitee.warned && (
                          <span className="invites__badge--warned">
                            {" "}
                            Warned
                          </span>
                        )}
                      </span>
                    ) : (
                      "-"
                    )}
                  </td>
                  <td>
                    {inv.invitee ? formatBytes(inv.invitee.uploaded) : "-"}
                  </td>
                  <td>
                    {inv.invitee ? formatBytes(inv.invitee.downloaded) : "-"}
                  </td>
                  <td>{inv.invitee ? formatRatio(inv.invitee.ratio) : "-"}</td>
                  <td>
                    {inv.invitee ? formatDate(inv.invitee.created_at) : "-"}
                  </td>
                  <td className="invites__actions">
                    {inv.status === "pending" ? (
                      <>
                        <button
                          type="button"
                          className="invites__copy-btn"
                          onClick={async () => {
                            try {
                              await navigator.clipboard.writeText(inv.token);
                              setCopiedAction(`code-${inv.id}`);
                              setTimeout(() => setCopiedAction(null), 2000);
                            } catch {
                              /* fallback */
                            }
                          }}
                        >
                          {copiedAction === `code-${inv.id}`
                            ? "Copied!"
                            : "Copy Code"}
                        </button>
                        <button
                          type="button"
                          className="invites__copy-btn"
                          onClick={async () => {
                            try {
                              await navigator.clipboard.writeText(
                                getInviteLink(inv.token),
                              );
                              setCopiedAction(`link-${inv.id}`);
                              setTimeout(() => setCopiedAction(null), 2000);
                            } catch {
                              /* fallback */
                            }
                          }}
                        >
                          {copiedAction === `link-${inv.id}`
                            ? "Copied!"
                            : "Copy Link"}
                        </button>
                      </>
                    ) : (
                      <span className="invites__no-action">-</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
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
