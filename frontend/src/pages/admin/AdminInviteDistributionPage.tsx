import { useCallback, useEffect, useMemo, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { Button } from "@/components/form";
import "./admin-ui.css";

const GIB = 1024 * 1024 * 1024;

interface Group {
  id: number;
  name: string;
  level: number;
  is_admin: boolean;
  is_moderator: boolean;
}

interface InviteDistributionRule {
  group_id: number;
  min_ratio: number;
  min_downloaded_bytes: number;
  max_downloaded_bytes: number;
  max_invites: number;
}

interface RowState {
  onLadder: boolean;
  min_ratio: string;
  min_downloaded_gib: string;
  max_downloaded_gib: string;
  max_invites: string;
}

const emptyRow: RowState = {
  onLadder: false,
  min_ratio: "0",
  min_downloaded_gib: "0",
  max_downloaded_gib: "0",
  max_invites: "0",
};

const THRESHOLDS: { key: keyof RowState; label: string; step: string }[] = [
  { key: "min_ratio", label: "Min Ratio", step: "0.05" },
  { key: "min_downloaded_gib", label: "Min Downloaded (GiB)", step: "1" },
  {
    key: "max_downloaded_gib",
    label: "Max Downloaded (GiB, 0=unbounded)",
    step: "1",
  },
  {
    key: "max_invites",
    label: "Max Invites (ceiling, 0=grants nothing)",
    step: "1",
  },
];

function ruleToRow(r: InviteDistributionRule): RowState {
  return {
    onLadder: true,
    min_ratio: String(r.min_ratio),
    min_downloaded_gib: String(r.min_downloaded_bytes / GIB),
    max_downloaded_gib: String(r.max_downloaded_bytes / GIB),
    max_invites: String(r.max_invites),
  };
}

type RowMap = Record<number, RowState>;

function rowsEqual(a: RowState, b: RowState): boolean {
  return (
    a.onLadder === b.onLadder &&
    a.min_ratio === b.min_ratio &&
    a.min_downloaded_gib === b.min_downloaded_gib &&
    a.max_downloaded_gib === b.max_downloaded_gib &&
    a.max_invites === b.max_invites
  );
}

export function AdminInviteDistributionPage() {
  const toast = useToast();
  const [groups, setGroups] = useState<Group[]>([]);
  const [initial, setInitial] = useState<RowMap>({});
  const [draft, setDraft] = useState<RowMap>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
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
        fetch(`${getConfig().API_URL}/api/v1/admin/invite-distribution/rules`, {
          headers: authHeaders(),
        }),
      ]);
      const gBody = gRes.ok ? await gRes.json() : { groups: [] };
      const rBody = rRes.ok ? await rRes.json() : { rules: [] };
      const nextGroups: Group[] = gBody.groups ?? [];
      const rules: InviteDistributionRule[] = rBody.rules ?? [];
      const ruleById: Record<number, InviteDistributionRule> = {};
      for (const r of rules) ruleById[r.group_id] = r;

      const rows: RowMap = {};
      for (const g of nextGroups) {
        rows[g.id] = ruleById[g.id]
          ? ruleToRow(ruleById[g.id])
          : { ...emptyRow };
      }
      setGroups(nextGroups);
      setInitial(rows);
      setDraft(rows);
    } catch {
      toast.error("Couldn't load the invite distribution rules");
    } finally {
      setLoading(false);
    }
  }, [authHeaders, toast]);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  // Non-staff classes are the only candidates; show them in level order.
  const candidates = useMemo(
    () =>
      groups
        .filter((g) => !g.is_admin && !g.is_moderator)
        .sort((a, b) => a.level - b.level),
    [groups],
  );

  const changedIds = useMemo(
    () =>
      candidates
        .filter((g) => initial[g.id] && !rowsEqual(initial[g.id], draft[g.id]))
        .map((g) => g.id),
    [candidates, initial, draft],
  );

  const setField = (
    groupId: number,
    key: keyof RowState,
    value: string | boolean,
  ) => {
    setDraft((prev) => ({
      ...prev,
      [groupId]: { ...prev[groupId], [key]: value },
    }));
  };

  const rowToPayload = (row: RowState) => ({
    min_ratio: Number(row.min_ratio) || 0,
    min_downloaded_bytes: Math.round(
      (Number(row.min_downloaded_gib) || 0) * GIB,
    ),
    max_downloaded_bytes: Math.round(
      (Number(row.max_downloaded_gib) || 0) * GIB,
    ),
    max_invites: Number(row.max_invites) || 0,
  });

  const saveChanges = async () => {
    setSaving(true);
    let failed = 0;
    await Promise.all(
      changedIds.map(async (id) => {
        const row = draft[id];
        const removing = initial[id].onLadder && !row.onLadder;
        try {
          const res = await fetch(
            `${getConfig().API_URL}/api/v1/admin/invite-distribution/rules/${id}`,
            removing
              ? { method: "DELETE", headers: authHeaders() }
              : {
                  method: "PUT",
                  headers: authHeaders(true),
                  body: JSON.stringify(rowToPayload(row)),
                },
          );
          if (!res.ok) failed++;
        } catch {
          failed++;
        }
      }),
    );
    setSaving(false);
    if (failed === 0) {
      toast.success(
        `Saved ${changedIds.length} ${
          changedIds.length === 1 ? "change" : "changes"
        }`,
      );
    } else {
      toast.error(`${failed} change${failed === 1 ? "" : "s"} failed to save`);
    }
    fetchAll();
  };

  const runNow = async () => {
    setRunning(true);
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/invite-distribution/run`,
        { method: "POST", headers: authHeaders() },
      );
      const body = await res.json().catch(() => null);
      if (!res.ok) {
        toast.error(body?.error?.message ?? "The run didn't complete");
      } else if (body?.skipped) {
        toast.info(
          body.reason === "disabled"
            ? "Invite distribution is off — enable it in Site Settings to run"
            : `Nothing to do: ${body.reason}`,
        );
      } else {
        toast.success(`Granted ${body?.granted ?? 0} invite(s)`);
      }
      fetchAll();
    } finally {
      setRunning(false);
    }
  };

  if (loading) return <p>Loading…</p>;

  const dirty = changedIds.length > 0;

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title">Invite Distribution</h1>
          <p className="admin-page-header__desc">
            Classes with a rule are eligible for automatic invite grants. Each
            cycle grants +1 invite to a member who meets the ratio and
            downloaded-bytes criteria, as long as their current invite balance
            is below the class's ceiling — never past it. Toggle classes on, set
            their thresholds, and save — then enable the engine and set the
            cycle in Site Settings. Staff classes can never receive auto-grants.
          </p>
        </div>
        <div className="admin-page-header__actions">
          <Button variant="secondary" onClick={runNow} loading={running}>
            Run now
          </Button>
        </div>
      </div>

      <div className="admin-panel">
        <div className="admin-table-scroll">
          <table className="admin-table">
            <thead>
              <tr>
                <th className="admin-table__toggle">Eligible</th>
                <th>Class</th>
                <th>Level</th>
                {THRESHOLDS.map((t) => (
                  <th key={t.key}>{t.label}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {candidates.map((g) => {
                const row = draft[g.id] ?? emptyRow;
                return (
                  <tr
                    key={g.id}
                    className={row.onLadder ? undefined : "admin-row--off"}
                  >
                    <td className="admin-table__toggle">
                      <input
                        type="checkbox"
                        aria-label={`Include ${g.name} in invite distribution`}
                        checked={row.onLadder}
                        onChange={(e) =>
                          setField(g.id, "onLadder", e.target.checked)
                        }
                      />
                    </td>
                    <td className="admin-table__name">{g.name}</td>
                    <td className="admin-num">{g.level}</td>
                    {THRESHOLDS.map((t) => (
                      <td key={t.key}>
                        <input
                          className="admin-num-input"
                          type="number"
                          min="0"
                          step={t.step}
                          aria-label={`${g.name} ${t.label}`}
                          disabled={!row.onLadder}
                          value={row[t.key] as string}
                          onChange={(e) =>
                            setField(g.id, t.key, e.target.value)
                          }
                        />
                      </td>
                    ))}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      {dirty && (
        <div className="admin-savebar">
          <span className="admin-savebar__count">
            {changedIds.length} unsaved{" "}
            {changedIds.length === 1 ? "change" : "changes"}
          </span>
          <div className="admin-savebar__actions">
            <Button
              variant="ghost"
              onClick={() => setDraft(initial)}
              disabled={saving}
            >
              Discard
            </Button>
            <Button variant="primary" onClick={saveChanges} loading={saving}>
              Save changes
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
