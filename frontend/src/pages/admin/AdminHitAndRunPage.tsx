import { useCallback, useEffect, useMemo, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { Button } from "@/components/form";
import "./admin-ui.css";

interface Group {
  id: number;
  name: string;
  level: number;
  is_admin: boolean;
  is_moderator: boolean;
}

interface HnRRule {
  group_id: number;
  required_seed_hours: number;
  required_ratio: number;
  inactivity_grace_hours: number;
  max_days_to_satisfy: number;
}

interface HnRRun {
  id: number;
  started_at: string;
  finished_at?: string;
  status: string;
  trigger: string;
  scanned: number;
  breached: number;
  satisfied: number;
  error?: string;
}

interface RowState {
  onLadder: boolean;
  required_seed_hours: string;
  required_ratio: string;
  inactivity_grace_hours: string;
  max_days_to_satisfy: string;
}

const emptyRow: RowState = {
  onLadder: false,
  required_seed_hours: "240",
  required_ratio: "1",
  inactivity_grace_hours: "48",
  max_days_to_satisfy: "30",
};

const THRESHOLDS: { key: keyof RowState; label: string; step: string }[] = [
  { key: "required_seed_hours", label: "Seed Hours", step: "1" },
  { key: "required_ratio", label: "Ratio", step: "0.05" },
  { key: "inactivity_grace_hours", label: "Grace (hrs)", step: "1" },
  { key: "max_days_to_satisfy", label: "Max Days (0=none)", step: "1" },
];

function ruleToRow(r: HnRRule): RowState {
  return {
    onLadder: true,
    required_seed_hours: String(r.required_seed_hours),
    required_ratio: String(r.required_ratio),
    inactivity_grace_hours: String(r.inactivity_grace_hours),
    max_days_to_satisfy: String(r.max_days_to_satisfy),
  };
}

type RowMap = Record<number, RowState>;

function rowsEqual(a: RowState, b: RowState): boolean {
  return (
    a.onLadder === b.onLadder &&
    a.required_seed_hours === b.required_seed_hours &&
    a.required_ratio === b.required_ratio &&
    a.inactivity_grace_hours === b.inactivity_grace_hours &&
    a.max_days_to_satisfy === b.max_days_to_satisfy
  );
}

export function AdminHitAndRunPage() {
  const toast = useToast();
  const [groups, setGroups] = useState<Group[]>([]);
  const [initial, setInitial] = useState<RowMap>({});
  const [draft, setDraft] = useState<RowMap>({});
  const [runs, setRuns] = useState<HnRRun[]>([]);
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
      const [gRes, rRes, runsRes] = await Promise.all([
        fetch(`${getConfig().API_URL}/api/v1/admin/groups`, {
          headers: authHeaders(),
        }),
        fetch(`${getConfig().API_URL}/api/v1/admin/hnr/rules`, {
          headers: authHeaders(),
        }),
        fetch(`${getConfig().API_URL}/api/v1/admin/hnr/runs?limit=5`, {
          headers: authHeaders(),
        }),
      ]);
      const gBody = gRes.ok ? await gRes.json() : { groups: [] };
      const rBody = rRes.ok ? await rRes.json() : { rules: [] };
      const runsBody = runsRes.ok ? await runsRes.json() : { runs: [] };
      const nextGroups: Group[] = gBody.groups ?? [];
      const rules: HnRRule[] = rBody.rules ?? [];
      const ruleById: Record<number, HnRRule> = {};
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
      setRuns(runsBody.runs ?? []);
    } catch {
      toast.error("Couldn't load hit-and-run configuration");
    } finally {
      setLoading(false);
    }
  }, [authHeaders, toast]);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  // Staff classes can never be subject to HnR; show the rest in level order.
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
    required_seed_hours: Number(row.required_seed_hours) || 0,
    required_ratio: Number(row.required_ratio) || 0,
    inactivity_grace_hours: Number(row.inactivity_grace_hours) || 0,
    max_days_to_satisfy: Number(row.max_days_to_satisfy) || 0,
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
            `${getConfig().API_URL}/api/v1/admin/hnr/rules/${id}`,
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
      const res = await fetch(`${getConfig().API_URL}/api/v1/admin/hnr/run`, {
        method: "POST",
        headers: authHeaders(),
      });
      const body = await res.json().catch(() => null);
      if (!res.ok) {
        toast.error(body?.error?.message ?? "The run didn't complete");
      } else if (body?.skipped) {
        toast.info("A run was already queued — this one dropped");
      } else {
        toast.success(
          `Scanned ${body?.scanned ?? 0}, breached ${body?.breached ?? 0}, satisfied ${body?.satisfied ?? 0}`,
        );
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
          <h1 className="admin-page-header__title">Hit-and-Run</h1>
          <p className="admin-page-header__desc">
            Classes with a rule are tracked for hit-and-run: a member must seed
            a snatch for the required hours, or reach the required ratio
            (against the torrent&apos;s full size — freeleech torrents are still
            eligible), before the inactivity grace or the hard day cap turns it
            into a breach. Classes with no rule (VIP, by default) are exempt.
            Turn the master switch on and set the daemon&apos;s cadence in Site
            Settings.
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
                <th className="admin-table__toggle">Tracked</th>
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
                        aria-label={`Track ${g.name} for hit-and-run`}
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

      <div className="admin-panel" style={{ marginTop: "1.5rem" }}>
        <h2 className="admin-page-header__title" style={{ fontSize: "1.1rem" }}>
          Recent runs
        </h2>
        {runs.length === 0 ? (
          <p className="admin-page-header__desc">
            No runs recorded yet — the daemon runs hourly, or press Run now.
          </p>
        ) : (
          <div className="admin-table-scroll">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>Started</th>
                  <th>Trigger</th>
                  <th>Status</th>
                  <th>Scanned</th>
                  <th>Breached</th>
                  <th>Satisfied</th>
                </tr>
              </thead>
              <tbody>
                {runs.map((r) => (
                  <tr key={r.id}>
                    <td>{new Date(r.started_at).toLocaleString()}</td>
                    <td>{r.trigger}</td>
                    <td>{r.status}</td>
                    <td className="admin-num">{r.scanned}</td>
                    <td className="admin-num">{r.breached}</td>
                    <td className="admin-num">{r.satisfied}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
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
