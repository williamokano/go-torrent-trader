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
  description?: string;
  type: "select" | "text" | "number" | "textarea";
  options?: { value: string; label: string }[];
}

const SETTING_DEFINITIONS: SettingConfig[] = [
  // Registration
  {
    key: "registration_mode",
    label: "Registration Mode",
    description: "Controls whether new users can register freely or need an invite code.",
    type: "select",
    options: [
      { value: "invite_only", label: "Invite Only" },
      { value: "open", label: "Open Registration" },
    ],
  },
  // Ratio warnings
  {
    key: "ratio_warning_threshold",
    label: "Ratio Warning Threshold",
    description: "Users with a ratio below this value will receive an automatic warning. Example: 0.3 means users downloading 3x more than they upload.",
    type: "number",
  },
  {
    key: "ratio_minimum_downloaded",
    label: "Ratio Minimum Downloaded (bytes)",
    description: "Minimum bytes downloaded before ratio rules apply. Prevents warnings for new users. Default: 5368709120 (5 GB).",
    type: "number",
  },
  {
    key: "ratio_warn_days",
    label: "Ratio Warning Delay (days)",
    description: "Number of days a user must be below the ratio threshold before receiving a soft warning.",
    type: "number",
  },
  {
    key: "ratio_ban_days",
    label: "Ratio Ban Delay (days)",
    description: "Number of days after the soft warning before the user's account is automatically disabled. Must be greater than the warning delay.",
    type: "number",
  },
  {
    key: "ratio_warning_message",
    label: "Ratio Warning Message",
    description: "PM sent when a user receives a ratio warning. Variables: {{username}}, {{ratio}}, {{threshold}}, {{days_elapsed}}, {{days_remaining}}.",
    type: "textarea",
  },
  {
    key: "ratio_ban_message",
    label: "Ratio Ban Message",
    description: "PM sent when a user is auto-banned for low ratio. Variables: {{username}}, {{ratio}}, {{threshold}}, {{days_elapsed}}.",
    type: "textarea",
  },
  // Chat anti-spam
  {
    key: "chat_rate_limit_window",
    label: "Chat Rate Limit Window (seconds)",
    description: "Time window in seconds for counting chat messages. If a user exceeds the max messages within this window, they get a strike.",
    type: "number",
  },
  {
    key: "chat_rate_limit_max",
    label: "Chat Rate Limit Max Messages",
    description: "Maximum number of chat messages allowed within the rate limit window before a strike is issued.",
    type: "number",
  },
  {
    key: "chat_spam_strike_count",
    label: "Chat Spam Strike Count",
    description: "Number of consecutive rate limit violations before the user is automatically muted. Strikes reset when the user sends a message without hitting the limit.",
    type: "number",
  },
  {
    key: "chat_spam_mute_minutes",
    label: "Chat Spam Mute Duration (minutes)",
    description: "How long a user is automatically muted after exceeding the strike count.",
    type: "number",
  },
  {
    key: "chat_strike_reset_seconds",
    label: "Chat Strike Reset Cooldown (seconds)",
    description: "Strikes reset to zero after this many seconds of no rate limit violations. Prevents strikes from accumulating across long gaps of normal behavior.",
    type: "number",
  },
  {
    key: "chat_rate_limit_message",
    label: "Chat Rate Limit Message",
    description: "Message shown to the user when they hit the rate limit. Displayed as a toast notification.",
    type: "text",
  },
  {
    key: "chat_spam_mute_message",
    label: "Chat Spam Mute Message",
    description: "Reason recorded when a user is auto-muted for spam. Visible to staff in the mute record.",
    type: "text",
  },
  // Tracker connection limits
  {
    key: "tracker_max_peers_per_torrent",
    label: "Max Peers Per Torrent",
    description:
      "Maximum number of peers allowed on a single torrent. New peers are rejected once this limit is reached. Set to 0 to disable. Default: 50.",
    type: "number",
  },
  {
    key: "tracker_max_peers_per_user",
    label: "Max Peers Per User",
    description:
      "Maximum number of concurrent peers a single user can have across all torrents. New peers are rejected once this limit is reached. Set to 0 to disable. Default: 100.",
    type: "number",
  },
  // Warning escalation
  {
    key: "warning_escalation_enabled",
    label: "Warning Escalation Enabled",
    description:
      "Master toggle for automatic warning escalation. When enabled, accumulating manual warnings can trigger privilege restrictions or account bans.",
    type: "select",
    options: [
      { value: "false", label: "Disabled" },
      { value: "true", label: "Enabled" },
    ],
  },
  {
    key: "warning_count_restrict",
    label: "Warnings Before Restriction",
    description:
      "Number of active manual warnings before the user's privileges are automatically restricted.",
    type: "number",
  },
  {
    key: "warning_count_ban",
    label: "Warnings Before Ban",
    description:
      "Number of active manual warnings before the user's account is automatically disabled. Must be greater than the restriction threshold.",
    type: "number",
  },
  {
    key: "warning_restrict_type",
    label: "Restriction Type",
    description:
      "Which privilege to restrict when the warning count reaches the restriction threshold.",
    type: "select",
    options: [
      { value: "download", label: "Download" },
      { value: "upload", label: "Upload" },
      { value: "chat", label: "Chat" },
      { value: "all", label: "All" },
    ],
  },
  {
    key: "warning_restrict_days",
    label: "Restriction Duration (days)",
    description:
      "How many days the automatic privilege restriction lasts. After this period, the restriction is lifted automatically by the maintenance job.",
    type: "number",
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
  const [saving, setSaving] = useState(false);

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

  const hasChanges = settings.some((s) => editValues[s.key] !== s.value);

  const handleSaveAll = async () => {
    setSaving(true);
    const changed = settings.filter((s) => editValues[s.key] !== s.value);
    const token = getAccessToken();
    let failed = false;

    for (const s of changed) {
      try {
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/admin/settings/${s.key}`,
          {
            method: "PUT",
            headers: {
              "Content-Type": "application/json",
              ...(token ? { Authorization: `Bearer ${token}` } : {}),
            },
            body: JSON.stringify({ value: editValues[s.key] }),
          },
        );
        if (!res.ok) {
          const body = await res.json();
          toast.error(
            `Failed to save ${getSettingDef(s.key).label}: ${body?.error?.message ?? "unknown error"}`,
          );
          failed = true;
        }
      } catch {
        toast.error(`Failed to save ${getSettingDef(s.key).label}`);
        failed = true;
      }
    }

    if (!failed) {
      toast.success("Settings saved");
    }
    fetchSettings();
    setSaving(false);
  };

  if (loading) return <p>Loading settings...</p>;

  return (
    <div>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          marginBottom: "var(--space-lg)",
        }}
      >
        <h1 style={{ fontSize: "var(--text-xl)", margin: 0 }}>Site Settings</h1>
        <button
          onClick={handleSaveAll}
          disabled={!hasChanges || saving}
          style={{
            padding: "var(--space-xs) var(--space-md)",
            backgroundColor: "var(--color-accent)",
            color: "white",
            border: "none",
            borderRadius: "var(--radius-md)",
            cursor: !hasChanges || saving ? "not-allowed" : "pointer",
            opacity: !hasChanges || saving ? 0.5 : 1,
            fontSize: "var(--text-sm)",
          }}
        >
          {saving ? "Saving..." : "Save Changes"}
        </button>
      </div>

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
          </tr>
        </thead>
        <tbody>
          {settings.map((s) => {
            const def = getSettingDef(s.key);
            return (
              <tr key={s.key}>
                <td
                  style={{
                    padding: "var(--space-xs) var(--space-sm)",
                    borderBottom: "1px solid var(--color-border)",
                    fontWeight: 500,
                  }}
                >
                  <div>{def.label}</div>
                  {def.description && (
                    <div
                      style={{
                        fontSize: "var(--text-xs)",
                        color: "var(--color-text-muted)",
                        fontWeight: 400,
                        marginTop: "2px",
                        lineHeight: 1.4,
                      }}
                    >
                      {def.description}
                    </div>
                  )}
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
                  ) : def.type === "textarea" ? (
                    <textarea
                      value={editValues[s.key] ?? s.value}
                      onChange={(e) =>
                        setEditValues((prev) => ({
                          ...prev,
                          [s.key]: e.target.value,
                        }))
                      }
                      rows={3}
                      style={{
                        padding: "4px 8px",
                        borderRadius: "var(--radius-sm)",
                        border: "1px solid var(--color-border)",
                        backgroundColor: "var(--color-bg-primary)",
                        color: "var(--color-text-primary)",
                        fontSize: "var(--text-sm)",
                        width: "100%",
                        fontFamily: "inherit",
                        resize: "vertical",
                      }}
                    />
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
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
