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
  stages_advanced?: number;
  stages_decayed?: number;
  error?: string;
}

interface HnRStage {
  stage: number;
  min_active_hnr: number;
  min_days_in_prev: number;
  action: string;
  restriction_types: string[];
  restriction_days: number;
  message_template: string;
}

interface StageDraft {
  min_active_hnr: string;
  min_days_in_prev: string;
  action: string;
  restriction_types: string[];
  restriction_days: string;
  message_template: string;
}

const ACTION_OPTIONS: { value: string; label: string }[] = [
  { value: "notify", label: "Notify" },
  { value: "warn", label: "Warn" },
  { value: "restrict", label: "Restrict" },
  { value: "final_notice", label: "Final notice" },
  { value: "ban", label: "Ban" },
];

const RESTRICTION_TYPE_OPTIONS: { value: string; label: string }[] = [
  { value: "download", label: "Download" },
  { value: "upload", label: "Upload" },
  { value: "chat", label: "Chat" },
  { value: "invite", label: "Invite" },
  { value: "feed", label: "Live feeds" },
  { value: "forum", label: "Forum" },
];

function stageToDraft(s: HnRStage): StageDraft {
  return {
    min_active_hnr: String(s.min_active_hnr),
    min_days_in_prev: String(s.min_days_in_prev),
    action: s.action,
    restriction_types: s.restriction_types ?? [],
    restriction_days: String(s.restriction_days),
    message_template: s.message_template,
  };
}

const emptyStageDraft: StageDraft = {
  min_active_hnr: "1",
  min_days_in_prev: "0",
  action: "notify",
  restriction_types: [],
  restriction_days: "0",
  message_template: "",
};

function stageDraftsEqual(a: StageDraft, b: StageDraft): boolean {
  return (
    a.min_active_hnr === b.min_active_hnr &&
    a.min_days_in_prev === b.min_days_in_prev &&
    a.action === b.action &&
    a.restriction_days === b.restriction_days &&
    a.message_template === b.message_template &&
    a.restriction_types.length === b.restriction_types.length &&
    a.restriction_types.every((t) => b.restriction_types.includes(t))
  );
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

  const [stages, setStages] = useState<HnRStage[]>([]);
  const [stageInitial, setStageInitial] = useState<Record<number, StageDraft>>(
    {},
  );
  const [stageDraft, setStageDraft] = useState<Record<number, StageDraft>>({});
  const [stageSavingId, setStageSavingId] = useState<number | null>(null);
  const [newStageNumber, setNewStageNumber] = useState("");
  const [newStageDraft, setNewStageDraft] =
    useState<StageDraft>(emptyStageDraft);
  const [addingStage, setAddingStage] = useState(false);

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
      const [gRes, rRes, runsRes, stagesRes] = await Promise.all([
        fetch(`${getConfig().API_URL}/api/v1/admin/groups`, {
          headers: authHeaders(),
        }),
        fetch(`${getConfig().API_URL}/api/v1/admin/hnr/rules`, {
          headers: authHeaders(),
        }),
        fetch(`${getConfig().API_URL}/api/v1/admin/hnr/runs?limit=5`, {
          headers: authHeaders(),
        }),
        fetch(`${getConfig().API_URL}/api/v1/admin/hnr/stages`, {
          headers: authHeaders(),
        }),
      ]);
      const gBody = gRes.ok ? await gRes.json() : { groups: [] };
      const rBody = rRes.ok ? await rRes.json() : { rules: [] };
      const runsBody = runsRes.ok ? await runsRes.json() : { runs: [] };
      const stagesBody = stagesRes.ok ? await stagesRes.json() : { stages: [] };
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

      const nextStages: HnRStage[] = (stagesBody.stages ?? []).sort(
        (a: HnRStage, b: HnRStage) => a.stage - b.stage,
      );
      const stageRows: Record<number, StageDraft> = {};
      for (const s of nextStages) stageRows[s.stage] = stageToDraft(s);
      setStages(nextStages);
      setStageInitial(stageRows);
      setStageDraft(stageRows);
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

  const setStageField = (
    stage: number,
    key: keyof StageDraft,
    value: string | string[],
  ) => {
    setStageDraft((prev) => ({
      ...prev,
      [stage]: { ...prev[stage], [key]: value },
    }));
  };

  const toggleStageRestrictionType = (stage: number, type: string) => {
    setStageDraft((prev) => {
      const current = prev[stage].restriction_types;
      const next = current.includes(type)
        ? current.filter((t) => t !== type)
        : [...current, type];
      return { ...prev, [stage]: { ...prev[stage], restriction_types: next } };
    });
  };

  const stageDraftToPayload = (d: StageDraft) => ({
    min_active_hnr: Number(d.min_active_hnr) || 0,
    min_days_in_prev: Number(d.min_days_in_prev) || 0,
    action: d.action,
    restriction_types: d.action === "restrict" ? d.restriction_types : [],
    restriction_days: Number(d.restriction_days) || 0,
    message_template: d.message_template,
  });

  const saveStage = async (stage: number) => {
    setStageSavingId(stage);
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/hnr/stages/${stage}`,
        {
          method: "PUT",
          headers: authHeaders(true),
          body: JSON.stringify(stageDraftToPayload(stageDraft[stage])),
        },
      );
      const body = await res.json().catch(() => null);
      if (!res.ok) {
        toast.error(body?.error?.message ?? `Failed to save stage ${stage}`);
      } else {
        toast.success(`Saved stage ${stage}`);
        fetchAll();
      }
    } finally {
      setStageSavingId(null);
    }
  };

  const deleteStage = async (stage: number) => {
    setStageSavingId(stage);
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/hnr/stages/${stage}`,
        { method: "DELETE", headers: authHeaders() },
      );
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        toast.error(body?.error?.message ?? `Failed to delete stage ${stage}`);
      } else {
        toast.success(`Deleted stage ${stage}`);
        fetchAll();
      }
    } finally {
      setStageSavingId(null);
    }
  };

  const addStage = async () => {
    const stageNum = Number(newStageNumber);
    if (!stageNum || stageNum < 1) {
      toast.error("Enter a stage number of at least 1");
      return;
    }
    if (stages.some((s) => s.stage === stageNum)) {
      toast.error(`Stage ${stageNum} already exists`);
      return;
    }
    setAddingStage(true);
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/hnr/stages/${stageNum}`,
        {
          method: "PUT",
          headers: authHeaders(true),
          body: JSON.stringify(stageDraftToPayload(newStageDraft)),
        },
      );
      const body = await res.json().catch(() => null);
      if (!res.ok) {
        toast.error(body?.error?.message ?? "Failed to add stage");
      } else {
        toast.success(`Added stage ${stageNum}`);
        setNewStageNumber("");
        setNewStageDraft(emptyStageDraft);
        fetchAll();
      }
    } finally {
      setAddingStage(false);
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
          Penalty ladder
        </h2>
        <p className="admin-page-header__desc">
          Site-wide, ordered by stage. A member climbs one rung per daemon run —
          never straight to a harsher stage, however many active hit-and-runs
          they suddenly have — gated by each stage&apos;s dwell time against the
          previous one. Falling active counts de-escalate immediately, in as
          many steps as the drop spans.
        </p>
        <div className="admin-table-scroll">
          <table className="admin-table">
            <thead>
              <tr>
                <th>Stage</th>
                <th>Min active H&amp;R</th>
                <th>Dwell (days)</th>
                <th>Action</th>
                <th>Restriction types</th>
                <th>Restrict days</th>
                <th>Message template</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {stages.map((s) => {
                const row = stageDraft[s.stage] ?? stageToDraft(s);
                const changed =
                  stageInitial[s.stage] &&
                  !stageDraftsEqual(stageInitial[s.stage], row);
                return (
                  <tr key={s.stage}>
                    <td className="admin-num">{s.stage}</td>
                    <td>
                      <input
                        className="admin-num-input"
                        type="number"
                        min="1"
                        aria-label={`Stage ${s.stage} min active hit-and-runs`}
                        value={row.min_active_hnr}
                        onChange={(e) =>
                          setStageField(
                            s.stage,
                            "min_active_hnr",
                            e.target.value,
                          )
                        }
                      />
                    </td>
                    <td>
                      <input
                        className="admin-num-input"
                        type="number"
                        min="0"
                        aria-label={`Stage ${s.stage} dwell days`}
                        value={row.min_days_in_prev}
                        onChange={(e) =>
                          setStageField(
                            s.stage,
                            "min_days_in_prev",
                            e.target.value,
                          )
                        }
                      />
                    </td>
                    <td>
                      <select
                        aria-label={`Stage ${s.stage} action`}
                        value={row.action}
                        onChange={(e) =>
                          setStageField(s.stage, "action", e.target.value)
                        }
                      >
                        {ACTION_OPTIONS.map((opt) => (
                          <option key={opt.value} value={opt.value}>
                            {opt.label}
                          </option>
                        ))}
                      </select>
                    </td>
                    <td>
                      {row.action === "restrict" ? (
                        <div className="admin-checkbox-group">
                          {RESTRICTION_TYPE_OPTIONS.map((opt) => (
                            <label key={opt.value}>
                              <input
                                type="checkbox"
                                checked={row.restriction_types.includes(
                                  opt.value,
                                )}
                                onChange={() =>
                                  toggleStageRestrictionType(s.stage, opt.value)
                                }
                              />
                              {opt.label}
                            </label>
                          ))}
                        </div>
                      ) : (
                        <span className="admin-page-header__desc">
                          only for restrict
                        </span>
                      )}
                    </td>
                    <td>
                      <input
                        className="admin-num-input"
                        type="number"
                        min="0"
                        disabled={row.action !== "restrict"}
                        aria-label={`Stage ${s.stage} restriction days`}
                        value={row.restriction_days}
                        onChange={(e) =>
                          setStageField(
                            s.stage,
                            "restriction_days",
                            e.target.value,
                          )
                        }
                      />
                    </td>
                    <td>
                      <textarea
                        className="admin-settings__input admin-settings__input--textarea"
                        aria-label={`Stage ${s.stage} message template`}
                        rows={2}
                        value={row.message_template}
                        onChange={(e) =>
                          setStageField(
                            s.stage,
                            "message_template",
                            e.target.value,
                          )
                        }
                      />
                    </td>
                    <td>
                      <div style={{ display: "flex", gap: "0.5rem" }}>
                        <Button
                          variant="primary"
                          size="sm"
                          disabled={!changed}
                          loading={stageSavingId === s.stage}
                          onClick={() => saveStage(s.stage)}
                        >
                          Save
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          loading={stageSavingId === s.stage}
                          onClick={() => deleteStage(s.stage)}
                        >
                          Delete
                        </Button>
                      </div>
                    </td>
                  </tr>
                );
              })}
              <tr>
                <td>
                  <input
                    className="admin-num-input"
                    type="number"
                    min="1"
                    placeholder="#"
                    aria-label="New stage number"
                    value={newStageNumber}
                    onChange={(e) => setNewStageNumber(e.target.value)}
                  />
                </td>
                <td>
                  <input
                    className="admin-num-input"
                    type="number"
                    min="1"
                    aria-label="New stage min active hit-and-runs"
                    value={newStageDraft.min_active_hnr}
                    onChange={(e) =>
                      setNewStageDraft((prev) => ({
                        ...prev,
                        min_active_hnr: e.target.value,
                      }))
                    }
                  />
                </td>
                <td>
                  <input
                    className="admin-num-input"
                    type="number"
                    min="0"
                    aria-label="New stage dwell days"
                    value={newStageDraft.min_days_in_prev}
                    onChange={(e) =>
                      setNewStageDraft((prev) => ({
                        ...prev,
                        min_days_in_prev: e.target.value,
                      }))
                    }
                  />
                </td>
                <td>
                  <select
                    aria-label="New stage action"
                    value={newStageDraft.action}
                    onChange={(e) =>
                      setNewStageDraft((prev) => ({
                        ...prev,
                        action: e.target.value,
                      }))
                    }
                  >
                    {ACTION_OPTIONS.map((opt) => (
                      <option key={opt.value} value={opt.value}>
                        {opt.label}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  {newStageDraft.action === "restrict" ? (
                    <div className="admin-checkbox-group">
                      {RESTRICTION_TYPE_OPTIONS.map((opt) => (
                        <label key={opt.value}>
                          <input
                            type="checkbox"
                            checked={newStageDraft.restriction_types.includes(
                              opt.value,
                            )}
                            onChange={() =>
                              setNewStageDraft((prev) => ({
                                ...prev,
                                restriction_types:
                                  prev.restriction_types.includes(opt.value)
                                    ? prev.restriction_types.filter(
                                        (t) => t !== opt.value,
                                      )
                                    : [...prev.restriction_types, opt.value],
                              }))
                            }
                          />
                          {opt.label}
                        </label>
                      ))}
                    </div>
                  ) : (
                    <span className="admin-page-header__desc">
                      only for restrict
                    </span>
                  )}
                </td>
                <td>
                  <input
                    className="admin-num-input"
                    type="number"
                    min="0"
                    disabled={newStageDraft.action !== "restrict"}
                    aria-label="New stage restriction days"
                    value={newStageDraft.restriction_days}
                    onChange={(e) =>
                      setNewStageDraft((prev) => ({
                        ...prev,
                        restriction_days: e.target.value,
                      }))
                    }
                  />
                </td>
                <td>
                  <textarea
                    className="admin-settings__input admin-settings__input--textarea"
                    aria-label="New stage message template"
                    rows={2}
                    placeholder="{{username}} has {{count}} active hit-and-runs."
                    value={newStageDraft.message_template}
                    onChange={(e) =>
                      setNewStageDraft((prev) => ({
                        ...prev,
                        message_template: e.target.value,
                      }))
                    }
                  />
                </td>
                <td>
                  <Button
                    variant="primary"
                    size="sm"
                    loading={addingStage}
                    onClick={addStage}
                  >
                    Add stage
                  </Button>
                </td>
              </tr>
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
                  <th>Advanced</th>
                  <th>Decayed</th>
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
                    <td className="admin-num">{r.stages_advanced ?? 0}</td>
                    <td className="admin-num">{r.stages_decayed ?? 0}</td>
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
