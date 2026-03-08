import { useCallback, useEffect, useState } from "react";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useToast } from "@/components/toast";

interface SiteSetting {
  key: string;
  value: string;
  updated_at: string;
}

interface SettingConfig {
  key: string;
  label: string;
  type: "select" | "text" | "number";
  options?: { value: string; label: string }[];
}

const SETTING_DEFINITIONS: SettingConfig[] = [
  {
    key: "registration_mode",
    label: "Registration Mode",
    type: "select",
    options: [
      { value: "invite_only", label: "Invite Only" },
      { value: "open", label: "Open Registration" },
    ],
  },
];

function getSettingDef(key: string): SettingConfig {
  return (
    SETTING_DEFINITIONS.find((d) => d.key === key) ?? {
      key,
      label: key,
      type: "text",
    }
  );
}

export function AdminSettingsPage() {
  const toast = useToast();
  const [settings, setSettings] = useState<SiteSetting[]>([]);
  const [loading, setLoading] = useState(true);
  const [editValues, setEditValues] = useState<Record<string, string>>({});
  const [savingKey, setSavingKey] = useState<string | null>(null);

  const fetchSettings = useCallback(async () => {
    setLoading(true);
    try {
      const token = getAccessToken();
      const res = await fetch(`${getConfig().API_URL}/api/v1/admin/settings`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      const body = await res.json();
      if (res.ok) {
        const items: SiteSetting[] = body?.settings ?? [];
        setSettings(items);
        const values: Record<string, string> = {};
        for (const s of items) {
          values[s.key] = s.value;
        }
        setEditValues(values);
      }
    } catch {
      toast.error("Failed to load settings");
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    fetchSettings();
  }, [fetchSettings]);

  const handleSave = async (key: string) => {
    setSavingKey(key);
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/settings/${key}`,
        {
          method: "PUT",
          headers: {
            "Content-Type": "application/json",
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
          body: JSON.stringify({ value: editValues[key] }),
        },
      );
      if (res.ok) {
        toast.success("Setting saved");
        fetchSettings();
      } else {
        const body = await res.json();
        toast.error(body?.error?.message ?? "Failed to save");
      }
    } catch {
      toast.error("Failed to save setting");
    } finally {
      setSavingKey(null);
    }
  };

  if (loading) return <p>Loading settings...</p>;

  return (
    <div>
      <h1
        style={{ fontSize: "var(--text-xl)", marginBottom: "var(--space-lg)" }}
      >
        Site Settings
      </h1>

      <table
        style={{
          width: "100%",
          borderCollapse: "collapse",
          fontSize: "var(--text-sm)",
        }}
      >
        <thead>
          <tr>
            <th
              style={{
                textAlign: "left",
                padding: "var(--space-xs) var(--space-sm)",
                borderBottom: "1px solid var(--color-border)",
                color: "var(--color-text-muted)",
                fontWeight: 600,
              }}
            >
              Setting
            </th>
            <th
              style={{
                textAlign: "left",
                padding: "var(--space-xs) var(--space-sm)",
                borderBottom: "1px solid var(--color-border)",
                color: "var(--color-text-muted)",
                fontWeight: 600,
              }}
            >
              Value
            </th>
            <th
              style={{
                textAlign: "left",
                padding: "var(--space-xs) var(--space-sm)",
                borderBottom: "1px solid var(--color-border)",
                color: "var(--color-text-muted)",
                fontWeight: 600,
                width: 80,
              }}
            >
              Actions
            </th>
          </tr>
        </thead>
        <tbody>
          {settings.map((s) => {
            const def = getSettingDef(s.key);
            const changed = editValues[s.key] !== s.value;
            return (
              <tr key={s.key}>
                <td
                  style={{
                    padding: "var(--space-xs) var(--space-sm)",
                    borderBottom: "1px solid var(--color-border)",
                    fontWeight: 500,
                  }}
                >
                  {def.label}
                </td>
                <td
                  style={{
                    padding: "var(--space-xs) var(--space-sm)",
                    borderBottom: "1px solid var(--color-border)",
                  }}
                >
                  {def.type === "select" && def.options ? (
                    <select
                      value={editValues[s.key] ?? s.value}
                      onChange={(e) =>
                        setEditValues((prev) => ({
                          ...prev,
                          [s.key]: e.target.value,
                        }))
                      }
                      style={{
                        padding: "4px 8px",
                        borderRadius: "var(--radius-sm)",
                        border: "1px solid var(--color-border)",
                        backgroundColor: "var(--color-bg-primary)",
                        color: "var(--color-text-primary)",
                        fontSize: "var(--text-sm)",
                      }}
                    >
                      {def.options.map((opt) => (
                        <option key={opt.value} value={opt.value}>
                          {opt.label}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <input
                      type={def.type === "number" ? "number" : "text"}
                      value={editValues[s.key] ?? s.value}
                      onChange={(e) =>
                        setEditValues((prev) => ({
                          ...prev,
                          [s.key]: e.target.value,
                        }))
                      }
                      style={{
                        padding: "4px 8px",
                        borderRadius: "var(--radius-sm)",
                        border: "1px solid var(--color-border)",
                        backgroundColor: "var(--color-bg-primary)",
                        color: "var(--color-text-primary)",
                        fontSize: "var(--text-sm)",
                        width: 200,
                      }}
                    />
                  )}
                </td>
                <td
                  style={{
                    padding: "var(--space-xs) var(--space-sm)",
                    borderBottom: "1px solid var(--color-border)",
                  }}
                >
                  {changed && (
                    <button
                      onClick={() => handleSave(s.key)}
                      disabled={savingKey === s.key}
                      style={{
                        padding: "2px 10px",
                        backgroundColor: "var(--color-accent)",
                        color: "white",
                        border: "none",
                        borderRadius: "var(--radius-sm)",
                        cursor: savingKey === s.key ? "not-allowed" : "pointer",
                        opacity: savingKey === s.key ? 0.6 : 1,
                        fontSize: "var(--text-xs)",
                      }}
                    >
                      {savingKey === s.key ? "..." : "Save"}
                    </button>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
