import { useCallback, useEffect, useState } from "react";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";

interface SiteSetting {
  key: string;
  value: string;
  updated_at: string;
}

export function AdminSettingsPage() {
  const [settings, setSettings] = useState<SiteSetting[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [registrationMode, setRegistrationMode] = useState("invite_only");

  const fetchSettings = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const token = getAccessToken();
      const res = await fetch(`${getConfig().API_URL}/api/v1/admin/settings`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      const body = await res.json();
      if (!res.ok) {
        setError(body?.error?.message ?? "Failed to load settings");
        return;
      }
      const items: SiteSetting[] = body?.settings ?? [];
      setSettings(items);
      const regMode = items.find((s) => s.key === "registration_mode");
      if (regMode) {
        setRegistrationMode(regMode.value);
      }
    } catch {
      setError("Failed to load settings");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSettings();
  }, [fetchSettings]);

  const handleSaveRegistrationMode = async () => {
    setSaving(true);
    setError(null);
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/settings/registration_mode`,
        {
          method: "PUT",
          headers: {
            "Content-Type": "application/json",
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
          body: JSON.stringify({ value: registrationMode }),
        },
      );
      const body = await res.json();
      if (!res.ok) {
        setError(body?.error?.message ?? "Failed to save setting");
        return;
      }
      fetchSettings();
    } catch {
      setError("Failed to save setting");
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return <div>Loading settings...</div>;
  }

  return (
    <div>
      <h2>Site Settings</h2>

      {error && (
        <div
          style={{ color: "var(--color-error, #ef4444)", marginBottom: "1rem" }}
        >
          {error}
        </div>
      )}

      <div style={{ marginBottom: "1.5rem" }}>
        <label
          htmlFor="registration-mode"
          style={{ display: "block", marginBottom: "0.5rem", fontWeight: 600 }}
        >
          Registration Mode
        </label>
        <select
          id="registration-mode"
          value={registrationMode}
          onChange={(e) => setRegistrationMode(e.target.value)}
          style={{
            padding: "0.5rem",
            borderRadius: "4px",
            border: "1px solid var(--color-border)",
            backgroundColor: "var(--color-bg-primary)",
            color: "var(--color-text-primary)",
            marginRight: "0.5rem",
          }}
        >
          <option value="invite_only">Invite Only</option>
          <option value="open">Open</option>
        </select>
        <button
          onClick={handleSaveRegistrationMode}
          disabled={saving}
          style={{
            padding: "0.5rem 1rem",
            backgroundColor: "var(--color-accent)",
            color: "var(--color-bg-primary)",
            border: "none",
            borderRadius: "4px",
            cursor: saving ? "not-allowed" : "pointer",
            opacity: saving ? 0.5 : 1,
          }}
        >
          {saving ? "Saving..." : "Save"}
        </button>
      </div>

      {settings.length > 0 && (
        <table
          style={{
            width: "100%",
            borderCollapse: "collapse",
            fontSize: "0.875rem",
          }}
        >
          <thead>
            <tr>
              <th
                style={{
                  textAlign: "left",
                  padding: "0.5rem",
                  borderBottom: "1px solid var(--color-border)",
                }}
              >
                Key
              </th>
              <th
                style={{
                  textAlign: "left",
                  padding: "0.5rem",
                  borderBottom: "1px solid var(--color-border)",
                }}
              >
                Value
              </th>
            </tr>
          </thead>
          <tbody>
            {settings.map((s) => (
              <tr key={s.key}>
                <td
                  style={{
                    padding: "0.5rem",
                    borderBottom: "1px solid var(--color-border)",
                  }}
                >
                  {s.key}
                </td>
                <td
                  style={{
                    padding: "0.5rem",
                    borderBottom: "1px solid var(--color-border)",
                  }}
                >
                  {s.value}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
