import { useCallback, useEffect, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { timeAgo } from "@/utils/format";
import "./admin-ui.css";
import "./admin-bans.css";

interface EmailBan {
  id: number;
  pattern: string;
  reason: string | null;
  created_by: number | null;
  created_at: string;
}

interface IPBan {
  id: number;
  ip_range: string;
  reason: string | null;
  created_by: number | null;
  created_at: string;
}

export function AdminBansPage() {
  const toast = useToast();

  const [emailBans, setEmailBans] = useState<EmailBan[]>([]);
  const [ipBans, setIPBans] = useState<IPBan[]>([]);
  const [loadingEmails, setLoadingEmails] = useState(true);
  const [loadingIPs, setLoadingIPs] = useState(true);

  const [emailPattern, setEmailPattern] = useState("");
  const [emailReason, setEmailReason] = useState("");
  const [addingEmail, setAddingEmail] = useState(false);

  const [ipRange, setIPRange] = useState("");
  const [ipReason, setIPReason] = useState("");
  const [addingIP, setAddingIP] = useState(false);

  const [deletingId, setDeletingId] = useState<number | null>(null);

  const fetchEmailBans = useCallback(async () => {
    setLoadingEmails(true);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/bans/emails`,
        { headers: { Authorization: `Bearer ${token}` } },
      );
      if (res.ok) {
        const data = await res.json();
        setEmailBans(data.email_bans ?? []);
      }
    } finally {
      setLoadingEmails(false);
    }
  }, []);

  const fetchIPBans = useCallback(async () => {
    setLoadingIPs(true);
    const token = getAccessToken();
    try {
      const res = await fetch(`${getConfig().API_URL}/api/v1/admin/bans/ips`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setIPBans(data.ip_bans ?? []);
      }
    } finally {
      setLoadingIPs(false);
    }
  }, []);

  useEffect(() => {
    fetchEmailBans();
    fetchIPBans();
  }, [fetchEmailBans, fetchIPBans]);

  const handleAddEmailBan = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!emailPattern.trim()) return;

    setAddingEmail(true);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/bans/emails`,
        {
          method: "POST",
          headers: {
            Authorization: `Bearer ${token}`,
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            pattern: emailPattern.trim(),
            reason: emailReason.trim() || null,
          }),
        },
      );
      if (res.ok) {
        toast.success("Email ban added");
        setEmailPattern("");
        setEmailReason("");
        fetchEmailBans();
      } else {
        const data = await res.json();
        toast.error(data?.error?.message ?? "Failed to add email ban");
      }
    } finally {
      setAddingEmail(false);
    }
  };

  const handleDeleteEmailBan = async (id: number) => {
    setDeletingId(id);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/bans/emails/${id}`,
        {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        },
      );
      if (res.ok || res.status === 204) {
        toast.success("Email ban removed");
        fetchEmailBans();
      } else {
        toast.error("Failed to remove email ban");
      }
    } finally {
      setDeletingId(null);
    }
  };

  const handleAddIPBan = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!ipRange.trim()) return;

    setAddingIP(true);
    const token = getAccessToken();
    try {
      const res = await fetch(`${getConfig().API_URL}/api/v1/admin/bans/ips`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          ip_range: ipRange.trim(),
          reason: ipReason.trim() || null,
        }),
      });
      if (res.ok) {
        toast.success("IP ban added");
        setIPRange("");
        setIPReason("");
        fetchIPBans();
      } else {
        const data = await res.json();
        toast.error(data?.error?.message ?? "Failed to add IP ban");
      }
    } finally {
      setAddingIP(false);
    }
  };

  const handleDeleteIPBan = async (id: number) => {
    setDeletingId(id);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/bans/ips/${id}`,
        {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        },
      );
      if (res.ok || res.status === 204) {
        toast.success("IP ban removed");
        fetchIPBans();
      } else {
        toast.error("Failed to remove IP ban");
      }
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title">Bans</h1>
          <p className="admin-page-header__desc">
            Block registrations and connections by email pattern or IP range.
          </p>
        </div>
      </div>

      <div className="admin-stack">
        <div className="admin-panel">
          <div className="admin-panel__section">
            <h2 className="admin-panel__title">Email Bans</h2>
            <form className="admin-bans__form" onSubmit={handleAddEmailBan}>
              <div className="admin-bans__form-field">
                <label htmlFor="email-pattern">Pattern</label>
                <input
                  id="email-pattern"
                  type="text"
                  placeholder="%@mailinator.com"
                  value={emailPattern}
                  onChange={(e) => setEmailPattern(e.target.value)}
                />
              </div>
              <div className="admin-bans__form-field">
                <label htmlFor="email-reason">Reason (optional)</label>
                <input
                  id="email-reason"
                  type="text"
                  placeholder="Disposable email provider"
                  value={emailReason}
                  onChange={(e) => setEmailReason(e.target.value)}
                />
              </div>
              <button
                type="submit"
                className="admin-btn admin-btn--primary"
                disabled={addingEmail || !emailPattern.trim()}
              >
                {addingEmail ? "Adding..." : "Add Ban"}
              </button>
            </form>
          </div>

          {loadingEmails ? (
            <p className="admin-empty">Loading...</p>
          ) : emailBans.length === 0 ? (
            <p className="admin-empty">No email bans configured.</p>
          ) : (
            <div className="admin-table-scroll">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>Pattern</th>
                    <th>Reason</th>
                    <th>Created</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {emailBans.map((ban) => (
                    <tr key={ban.id}>
                      <td className="admin-num">{ban.pattern}</td>
                      <td>{ban.reason ?? "-"}</td>
                      <td className="admin-muted">{timeAgo(ban.created_at)}</td>
                      <td className="admin-table__actions">
                        <button
                          className="admin-btn admin-btn--danger admin-btn--sm"
                          onClick={() => handleDeleteEmailBan(ban.id)}
                          disabled={deletingId === ban.id}
                        >
                          {deletingId === ban.id ? "..." : "Delete"}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        <div className="admin-panel">
          <div className="admin-panel__section">
            <h2 className="admin-panel__title">IP Bans</h2>
            <form className="admin-bans__form" onSubmit={handleAddIPBan}>
              <div className="admin-bans__form-field">
                <label htmlFor="ip-range">IP / CIDR Range</label>
                <input
                  id="ip-range"
                  type="text"
                  placeholder="10.0.0.0/8"
                  value={ipRange}
                  onChange={(e) => setIPRange(e.target.value)}
                />
              </div>
              <div className="admin-bans__form-field">
                <label htmlFor="ip-reason">Reason (optional)</label>
                <input
                  id="ip-reason"
                  type="text"
                  placeholder="Known VPN range"
                  value={ipReason}
                  onChange={(e) => setIPReason(e.target.value)}
                />
              </div>
              <button
                type="submit"
                className="admin-btn admin-btn--primary"
                disabled={addingIP || !ipRange.trim()}
              >
                {addingIP ? "Adding..." : "Add Ban"}
              </button>
            </form>
          </div>

          {loadingIPs ? (
            <p className="admin-empty">Loading...</p>
          ) : ipBans.length === 0 ? (
            <p className="admin-empty">No IP bans configured.</p>
          ) : (
            <div className="admin-table-scroll">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>IP Range</th>
                    <th>Reason</th>
                    <th>Created</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {ipBans.map((ban) => (
                    <tr key={ban.id}>
                      <td className="admin-num">{ban.ip_range}</td>
                      <td>{ban.reason ?? "-"}</td>
                      <td className="admin-muted">{timeAgo(ban.created_at)}</td>
                      <td className="admin-table__actions">
                        <button
                          className="admin-btn admin-btn--danger admin-btn--sm"
                          onClick={() => handleDeleteIPBan(ban.id)}
                          disabled={deletingId === ban.id}
                        >
                          {deletingId === ban.id ? "..." : "Delete"}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
