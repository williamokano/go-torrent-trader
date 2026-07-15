import { useCallback, useEffect, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";

interface Group {
  id: number;
  name: string;
  level: number;
  is_admin: boolean;
  is_moderator: boolean;
}

interface PromotionRule {
  group_id: number;
  min_ratio: number;
  min_uploaded: number;
  min_age_days: number;
  min_torrents: number;
  min_seed_hours: number;
}

interface RuleForm {
  min_ratio: string;
  min_uploaded: string;
  min_age_days: string;
  min_torrents: string;
  min_seed_hours: string;
}

const zeroForm: RuleForm = {
  min_ratio: "0",
  min_uploaded: "0",
  min_age_days: "0",
  min_torrents: "0",
  min_seed_hours: "0",
};

const THRESHOLDS: { key: keyof RuleForm; label: string }[] = [
  { key: "min_ratio", label: "Min Ratio" },
  { key: "min_uploaded", label: "Min Upload (bytes)" },
  { key: "min_age_days", label: "Min Age (days)" },
  { key: "min_torrents", label: "Min Torrents" },
  { key: "min_seed_hours", label: "Min Seed (hrs)" },
];

const cell: React.CSSProperties = {
  padding: "var(--space-xs) var(--space-sm)",
  borderBottom: "1px solid var(--color-border)",
};
const head: React.CSSProperties = {
  ...cell,
  textAlign: "left",
  color: "var(--color-text-muted)",
  whiteSpace: "nowrap",
};

function ruleToForm(r: PromotionRule): RuleForm {
  return {
    min_ratio: String(r.min_ratio),
    min_uploaded: String(r.min_uploaded),
    min_age_days: String(r.min_age_days),
    min_torrents: String(r.min_torrents),
    min_seed_hours: String(r.min_seed_hours),
  };
}

export function AdminPromotionPage() {
  const toast = useToast();
  const [groups, setGroups] = useState<Group[]>([]);
  const [rules, setRules] = useState<Record<number, PromotionRule>>({});
  const [forms, setForms] = useState<Record<number, RuleForm>>({});
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(false);

  const authHeaders = useCallback((json = false): Record<string, string> => {
    const token = getAccessToken();
    return {
      ...(json ? { "Content-Type": "application/json" } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    };
  }, []);

  const fetchAll = useCallback(async () => {
    setLoading(true);
    try {
      const [gRes, rRes] = await Promise.all([
        fetch(`${getConfig().API_URL}/api/v1/admin/groups`, {
          headers: authHeaders(),
        }),
        fetch(`${getConfig().API_URL}/api/v1/admin/promotion/rules`, {
          headers: authHeaders(),
        }),
      ]);
      if (gRes.ok) {
        const body = await gRes.json();
        setGroups(body.groups ?? []);
      }
      if (rRes.ok) {
        const body = await rRes.json();
        const list: PromotionRule[] = body.rules ?? [];
        const byId: Record<number, PromotionRule> = {};
        const formById: Record<number, RuleForm> = {};
        for (const r of list) {
          byId[r.group_id] = r;
          formById[r.group_id] = ruleToForm(r);
        }
        setRules(byId);
        setForms(formById);
      }
    } catch {
      toast.error("Failed to load promotion configuration");
    } finally {
      setLoading(false);
    }
  }, [authHeaders, toast]);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  const upsertRule = async (groupId: number, form: RuleForm) => {
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/promotion/rules/${groupId}`,
      {
        method: "PUT",
        headers: authHeaders(true),
        body: JSON.stringify({
          min_ratio: Number(form.min_ratio) || 0,
          min_uploaded: Number(form.min_uploaded) || 0,
          min_age_days: Number(form.min_age_days) || 0,
          min_torrents: Number(form.min_torrents) || 0,
          min_seed_hours: Number(form.min_seed_hours) || 0,
        }),
      },
    );
    if (res.ok) {
      toast.success("Rule saved");
      fetchAll();
    } else {
      const body = await res.json().catch(() => null);
      toast.error(body?.error?.message ?? "Failed to save rule");
    }
  };

  const removeRule = async (groupId: number) => {
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/promotion/rules/${groupId}`,
      { method: "DELETE", headers: authHeaders() },
    );
    if (res.ok) {
      toast.success("Removed from ladder");
      fetchAll();
    } else {
      toast.error("Failed to remove rule");
    }
  };

  const runNow = async () => {
    setRunning(true);
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/promotion/run`,
        { method: "POST", headers: authHeaders() },
      );
      const body = await res.json().catch(() => null);
      if (!res.ok) {
        toast.error(body?.error?.message ?? "Run failed");
      } else if (body?.skipped) {
        toast.info(`Skipped: ${body.reason}`);
      } else {
        toast.success(
          `Promoted ${body?.promoted ?? 0}, demoted ${body?.demoted ?? 0}`,
        );
      }
      fetchAll();
    } finally {
      setRunning(false);
    }
  };

  if (loading) return <p>Loading...</p>;

  // Only non-staff groups can be ladder rungs; order by level ascending.
  const candidates = groups
    .filter((g) => !g.is_admin && !g.is_moderator)
    .sort((a, b) => a.level - b.level);

  return (
    <div>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          marginBottom: "var(--space-md)",
        }}
      >
        <h1 style={{ fontSize: "var(--text-xl)", margin: 0 }}>
          Class Promotion
        </h1>
        <button
          onClick={runNow}
          disabled={running}
          style={{
            padding: "var(--space-xs) var(--space-md)",
            backgroundColor: "var(--color-accent)",
            color: "white",
            border: "none",
            borderRadius: "var(--radius-md)",
            cursor: running ? "not-allowed" : "pointer",
            opacity: running ? 0.5 : 1,
          }}
        >
          {running ? "Running..." : "Run now"}
        </button>
      </div>

      <p
        style={{
          color: "var(--color-text-muted)",
          fontSize: "var(--text-sm)",
          marginBottom: "var(--space-md)",
        }}
      >
        Groups with a rule form the promotion ladder, ordered by level. Each run
        moves a user at most one rung up (if they meet the next class's rule) or
        one rung down (if they no longer meet their own). Enable the engine and
        set the cycle in Site Settings. Staff groups can never be ladder rungs.
      </p>

      <div style={{ overflowX: "auto" }}>
        <table
          style={{
            width: "100%",
            borderCollapse: "collapse",
            fontSize: "var(--text-sm)",
          }}
        >
          <thead>
            <tr>
              <th style={head}>Class</th>
              <th style={head}>Level</th>
              {THRESHOLDS.map((t) => (
                <th key={t.key} style={head}>
                  {t.label}
                </th>
              ))}
              <th style={{ ...head, textAlign: "right" }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {candidates.map((g) => {
              const onLadder = rules[g.id] != null;
              const form = forms[g.id] ?? zeroForm;
              return (
                <tr key={g.id}>
                  <td style={{ ...cell, fontWeight: 600 }}>{g.name}</td>
                  <td style={cell}>{g.level}</td>
                  {THRESHOLDS.map((t) => (
                    <td key={t.key} style={cell}>
                      {onLadder ? (
                        <input
                          type="number"
                          aria-label={`${g.name} ${t.label}`}
                          value={form[t.key]}
                          onChange={(e) =>
                            setForms((prev) => ({
                              ...prev,
                              [g.id]: { ...form, [t.key]: e.target.value },
                            }))
                          }
                          style={{ width: 90 }}
                        />
                      ) : (
                        <span style={{ color: "var(--color-text-muted)" }}>
                          —
                        </span>
                      )}
                    </td>
                  ))}
                  <td
                    style={{
                      ...cell,
                      textAlign: "right",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {onLadder ? (
                      <>
                        <button
                          onClick={() => upsertRule(g.id, form)}
                          style={{
                            marginRight: "var(--space-xs)",
                            cursor: "pointer",
                          }}
                        >
                          Save
                        </button>
                        <button
                          onClick={() => removeRule(g.id)}
                          style={{ cursor: "pointer" }}
                        >
                          Remove
                        </button>
                      </>
                    ) : (
                      <button
                        onClick={() => upsertRule(g.id, zeroForm)}
                        style={{ cursor: "pointer" }}
                      >
                        Add to ladder
                      </button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
