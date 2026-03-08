import { useCallback, useEffect, useState } from "react";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useToast } from "@/components/toast";
import { Select } from "@/components/form";

interface SiteSetting {
  key: string;
  value: string;
  updated_at: string;
}

export function AdminSettingsPage() {
  const toast = useToast();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [registrationMode, setRegistrationMode] = useState("invite_only");

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
        const regMode = items.find((s) => s.key === "registration_mode");
        if (regMode) {
          setRegistrationMode(regMode.value);
        }
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

  const handleSave = async () => {
    setSaving(true);
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
      if (res.ok) {
        toast.success("Settings saved");
      } else {
        const body = await res.json();
        toast.error(body?.error?.message ?? "Failed to save");
      }
    } catch {
      toast.error("Failed to save settings");
    } finally {
      setSaving(false);
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

      <div style={{ maxWidth: 400 }}>
        <Select
          label="Registration Mode"
          options={[
            { value: "invite_only", label: "Invite Only" },
            { value: "open", label: "Open Registration" },
          ]}
          value={registrationMode}
          onChange={(e) => setRegistrationMode(e.target.value)}
        />
        <p
          style={{
            fontSize: "var(--text-xs)",
            color: "var(--color-text-muted)",
            margin: "var(--space-xs) 0 var(--space-md)",
          }}
        >
          {registrationMode === "open"
            ? "Anyone can register without an invite code."
            : "Users must provide a valid invite code to register."}
        </p>
        <button
          onClick={handleSave}
          disabled={saving}
          style={{
            padding: "var(--space-xs) var(--space-md)",
            backgroundColor: "var(--color-accent)",
            color: "white",
            border: "none",
            borderRadius: "var(--radius-md)",
            cursor: saving ? "not-allowed" : "pointer",
            opacity: saving ? 0.6 : 1,
            fontSize: "var(--text-sm)",
          }}
        >
          {saving ? "Saving..." : "Save"}
        </button>
      </div>
    </div>
  );
}
