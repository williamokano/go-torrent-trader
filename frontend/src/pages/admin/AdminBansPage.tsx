import { useCallback, useEffect, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { timeAgo } from "@/utils/format";
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
      <h1>Bans</h1>

      <div className="admin-bans__section">
        <div className="admin-bans__section-header">
          <h2>Email Bans</h2>
        </div>

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
            className="admin-bans__add-btn"
            disabled={addingEmail || !emailPattern.trim()}
          >
            {addingEmail ? "Adding..." : "Add Ban"}
          </button>
        </form>

        {loadingEmails ? (
          <p>Loading...</p>
        ) : emailBans.length === 0 ? (
          <p className="admin-bans__empty">No email bans configured.</p>
        ) : (
          <table className="admin-bans__table">
            <thead>
              <tr>
                <th>Pattern</th>
                <th>Reason</th>
                <th>Created</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {emailBans.map((ban) => (
                <tr key={ban.id}>
                  <td>
                    <code>{ban.pattern}</code>
                  </td>
                  <td>{ban.reason ?? "-"}</td>
                  <td>{timeAgo(ban.created_at)}</td>
                  <td>
                    <button
                      className="admin-bans__delete-btn"
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
        )}
      </div>

      <div className="admin-bans__section">
        <div className="admin-bans__section-header">
          <h2>IP Bans</h2>
        </div>

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
            className="admin-bans__add-btn"
            disabled={addingIP || !ipRange.trim()}
          >
            {addingIP ? "Adding..." : "Add Ban"}
          </button>
        </form>

        {loadingIPs ? (
          <p>Loading...</p>
        ) : ipBans.length === 0 ? (
          <p className="admin-bans__empty">No IP bans configured.</p>
        ) : (
          <table className="admin-bans__table">
            <thead>
              <tr>
                <th>IP Range</th>
                <th>Reason</th>
                <th>Created</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {ipBans.map((ban) => (
                <tr key={ban.id}>
                  <td>
                    <code>{ban.ip_range}</code>
                  </td>
                  <td>{ban.reason ?? "-"}</td>
                  <td>{timeAgo(ban.created_at)}</td>
                  <td>
                    <button
                      className="admin-bans__delete-btn"
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
        )}
      </div>
    </div>
  );
}
