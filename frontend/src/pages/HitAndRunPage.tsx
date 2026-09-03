import { useCallback, useEffect, useRef, useState } from "react";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useAuth } from "@/features/auth";
import { useToast } from "@/components/toast";
import { Button } from "@/components/form";
import { formatBytes, formatRatio, timeAgo } from "@/utils/format";
import "./hit-and-run.css";

interface HnRRecordView {
  id: number;
  torrent_id: number;
  torrent_name: string;
  torrent_size: number;
  state: string;
  display_status: "breach" | "monitoring" | "satisfied" | "cleared" | "waived";
  completed_at: string;
  last_seen_at: string;
  seeded_seconds: number;
  uploaded: number;
  breached_at?: string;
  resolved_at?: string;
  currently_seeding: boolean;
  required_seed_hours?: number;
  required_ratio?: number;
  inactivity_grace_hours?: number;
}

// Light polling while the tab is on screen, so a member who starts seeding
// without reloading still sees a torrent leave "in breach" on its own —
// mirroring AdminConnectorsPage's own status refresh, but paused in the
// background since nobody is watching a hidden tab tick over.
const POLL_MS = 15_000;

function formatHours(seconds: number): string {
  const hours = seconds / 3600;
  if (hours < 1) return `${Math.round(seconds / 60)}m`;
  if (hours < 48) return `${hours.toFixed(1)}h`;
  return `${(hours / 24).toFixed(1)}d`;
}

function percent(have: number, need: number): number {
  if (need <= 0) return 100;
  return Math.max(0, Math.min(100, Math.round((have / need) * 100)));
}

function ProgressBar({ percent: pct }: { percent: number }) {
  return (
    <div className="hnr-progress">
      <div className="hnr-progress__fill" style={{ width: `${pct}%` }} />
    </div>
  );
}

// A projected clear time only means something while currently seeding —
// otherwise the clock isn't running and any projection would be fiction.
function projectedClear(rec: HnRRecordView): string | null {
  if (!rec.currently_seeding) return null;
  const needsSeedHours = rec.required_seed_hours ?? 0;
  const remainingSeedSeconds = Math.max(
    0,
    needsSeedHours * 3600 - rec.seeded_seconds,
  );
  if (remainingSeedSeconds === 0) return null; // seed-time arm already satisfied
  const eta = new Date(Date.now() + remainingSeedSeconds * 1000);
  return eta.toLocaleString();
}

interface ClearButtonProps {
  price: number | undefined;
  balance: number;
  clearing: boolean;
  onClear: () => void;
}

function ClearButton({ price, balance, clearing, onClear }: ClearButtonProps) {
  const affordable = price !== undefined && balance >= price;
  return (
    <Button
      variant="secondary"
      size="sm"
      onClick={onClear}
      disabled={price === undefined || !affordable}
      loading={clearing}
      title={
        price === undefined
          ? undefined
          : affordable
            ? undefined
            : "Not enough points"
      }
    >
      {price === undefined
        ? "Clear with points"
        : `Clear for ${price.toLocaleString()} pts`}
    </Button>
  );
}

function MonitoredCard({
  rec,
  price,
  balance,
  clearing,
  onClear,
}: {
  rec: HnRRecordView;
  price: number | undefined;
  balance: number;
  clearing: boolean;
  onClear: () => void;
}) {
  const needsSeedHours = rec.required_seed_hours ?? 0;
  const needsRatio = rec.required_ratio ?? 0;
  const seedPct = percent(rec.seeded_seconds / 3600, needsSeedHours);
  const requiredUpload = needsRatio > 0 ? needsRatio * rec.torrent_size : 0;
  const uploadPct = percent(rec.uploaded, requiredUpload);
  const eta = projectedClear(rec);

  return (
    <div className="hnr-card hnr-card--monitoring">
      <div className="hnr-card__header">
        <span className="hnr-card__name">{rec.torrent_name}</span>
        {rec.currently_seeding ? (
          <span className="hnr-badge hnr-badge--seeding">Seeding now</span>
        ) : (
          <span className="hnr-badge hnr-badge--idle">Not seeding</span>
        )}
      </div>
      <div className="hnr-card__metrics">
        {needsSeedHours > 0 && (
          <div className="hnr-metric">
            <div className="hnr-metric__label">
              Seed time: {formatHours(rec.seeded_seconds)} /{" "}
              {formatHours(needsSeedHours * 3600)} ({seedPct}%)
            </div>
            <ProgressBar percent={seedPct} />
          </div>
        )}
        {needsRatio > 0 && (
          <div className="hnr-metric">
            <div className="hnr-metric__label">
              Uploaded: {formatBytes(rec.uploaded)} /{" "}
              {formatBytes(requiredUpload)} ({uploadPct}%)
            </div>
            <ProgressBar percent={uploadPct} />
          </div>
        )}
      </div>
      <div className="hnr-card__footer">
        {eta ? (
          <span>Projected clear: {eta}</span>
        ) : (
          <span className="hnr-card__footer-muted">
            Clock stopped — not currently seeding
          </span>
        )}
      </div>
      <div className="hnr-card__actions">
        <ClearButton
          price={price}
          balance={balance}
          clearing={clearing}
          onClear={onClear}
        />
      </div>
    </div>
  );
}

function BreachCard({
  rec,
  price,
  balance,
  clearing,
  onClear,
}: {
  rec: HnRRecordView;
  price: number | undefined;
  balance: number;
  clearing: boolean;
  onClear: () => void;
}) {
  return (
    <div className="hnr-card hnr-card--breach">
      <div className="hnr-card__header">
        <span className="hnr-card__name">{rec.torrent_name}</span>
        <span className="hnr-badge hnr-badge--breach">In breach</span>
      </div>
      <div className="hnr-card__footer">
        {rec.breached_at ? (
          <span>Breached {timeAgo(rec.breached_at)}</span>
        ) : (
          <span>Last seen {timeAgo(rec.last_seen_at)}</span>
        )}
        {" · "}
        <span>
          Seeded {formatHours(rec.seeded_seconds)}, uploaded{" "}
          {formatBytes(rec.uploaded)} (ratio{" "}
          {formatRatio(rec.uploaded / Math.max(1, rec.torrent_size))})
        </span>
      </div>
      <div className="hnr-card__actions">
        <ClearButton
          price={price}
          balance={balance}
          clearing={clearing}
          onClear={onClear}
        />
      </div>
    </div>
  );
}

const RESOLVED_LABELS: Record<string, string> = {
  satisfied: "Satisfied",
  cleared: "Cleared with points",
  waived: "Waived",
};

function ResolvedRow({ rec }: { rec: HnRRecordView }) {
  return (
    <div className="hnr-row">
      <span className="hnr-row__name">{rec.torrent_name}</span>
      <span
        className={`hnr-badge hnr-badge--${rec.display_status === "cleared" ? "cleared" : "resolved"}`}
      >
        {RESOLVED_LABELS[rec.display_status] ?? rec.display_status}
      </span>
      <span className="hnr-row__when">
        {rec.resolved_at ? timeAgo(rec.resolved_at) : ""}
      </span>
    </div>
  );
}

function authHeaders(): Record<string, string> {
  const token = getAccessToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export function HitAndRunPage() {
  const { user, refreshUser } = useAuth();
  const toast = useToast();
  const [records, setRecords] = useState<HnRRecordView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [prices, setPrices] = useState<Record<number, number>>({});
  const [clearingId, setClearingId] = useState<number | null>(null);
  const [clearingAll, setClearingAll] = useState(false);
  const initialLoad = useRef(true);

  const balance = user?.bonus_points ?? 0;

  const fetchQuotes = useCallback(async (ids: number[]) => {
    const entries = await Promise.all(
      ids.map(async (id) => {
        try {
          const res = await fetch(
            `${getConfig().API_URL}/api/v1/hnr/${id}/clear-price`,
            { headers: authHeaders() },
          );
          if (!res.ok) return null;
          const body = await res.json();
          return [id, body?.price as number] as const;
        } catch {
          return null;
        }
      }),
    );
    setPrices((prev) => {
      const next = { ...prev };
      for (const entry of entries) {
        if (entry) next[entry[0]] = entry[1];
      }
      return next;
    });
  }, []);

  const fetchRecords = useCallback(async () => {
    if (initialLoad.current) setLoading(true);
    try {
      const res = await fetch(`${getConfig().API_URL}/api/v1/hnr`, {
        headers: authHeaders(),
      });
      if (res.ok) {
        const body = await res.json();
        const items: HnRRecordView[] = body?.records ?? [];
        setRecords(items);
        setError(null);
        const openIds = items
          .filter(
            (r) =>
              r.display_status === "breach" ||
              r.display_status === "monitoring",
          )
          .map((r) => r.id);
        if (openIds.length > 0) fetchQuotes(openIds);
      } else if (initialLoad.current) {
        setError("Failed to load hit-and-run records");
      }
    } catch {
      if (initialLoad.current) setError("Failed to load hit-and-run records");
    } finally {
      setLoading(false);
      initialLoad.current = false;
    }
  }, [fetchQuotes]);

  useEffect(() => {
    fetchRecords();
  }, [fetchRecords]);

  useEffect(() => {
    function tick() {
      if (document.visibilityState === "visible") fetchRecords();
    }
    const timer = setInterval(tick, POLL_MS);
    document.addEventListener("visibilitychange", tick);
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", tick);
    };
  }, [fetchRecords]);

  const handleClear = async (id: number) => {
    setClearingId(id);
    try {
      const res = await fetch(`${getConfig().API_URL}/api/v1/hnr/${id}/clear`, {
        method: "POST",
        headers: authHeaders(),
      });
      const body = await res.json().catch(() => null);
      if (res.ok) {
        toast.success(`Cleared for ${(body?.price ?? 0).toLocaleString()} pts`);
        await refreshUser();
        fetchRecords();
      } else {
        toast.error(body?.error?.message ?? "Failed to clear");
      }
    } finally {
      setClearingId(null);
    }
  };

  const handleClearAll = async () => {
    setClearingAll(true);
    try {
      const res = await fetch(`${getConfig().API_URL}/api/v1/hnr/clear-all`, {
        method: "POST",
        headers: authHeaders(),
      });
      const body = await res.json().catch(() => null);
      if (res.ok) {
        const cleared = body?.cleared ?? 0;
        if (cleared === 0) {
          toast.info("Nothing affordable to clear right now");
        } else {
          toast.success(
            `Cleared ${cleared} for ${(body?.total_spent ?? 0).toLocaleString()} pts` +
              (body?.stopped_insufficient_points
                ? " — ran out of points before the rest"
                : ""),
          );
        }
        await refreshUser();
        fetchRecords();
      } else {
        toast.error(body?.error?.message ?? "Failed to clear");
      }
    } finally {
      setClearingAll(false);
    }
  };

  const breach = records.filter((r) => r.display_status === "breach");
  const monitoring = records.filter((r) => r.display_status === "monitoring");
  const resolved = records.filter(
    (r) =>
      r.display_status === "satisfied" ||
      r.display_status === "cleared" ||
      r.display_status === "waived",
  );
  const openCount = breach.length + monitoring.length;

  return (
    <div className="hnr-page">
      <div className="hnr-page__header">
        <div>
          <h1 className="hnr-page__title">Hit &amp; Run</h1>
          <p className="hnr-page__desc">
            Every download you've completed carries an obligation to seed it
            back, per your class's rules. This page reflects your live seeding
            state, not just the last hourly check. Your balance:{" "}
            {balance.toLocaleString()} pts.
          </p>
        </div>
        {openCount > 0 && (
          <Button
            variant="secondary"
            onClick={handleClearAll}
            loading={clearingAll}
          >
            Clear all affordable
          </Button>
        )}
      </div>

      {loading ? (
        <p>Loading…</p>
      ) : error ? (
        <p className="hnr-page__error">{error}</p>
      ) : records.length === 0 ? (
        <p className="hnr-page__empty">
          Nothing tracked yet. Complete a download and it will show up here.
        </p>
      ) : (
        <>
          <section className="hnr-section">
            <h2 className="hnr-section__title">
              In breach{breach.length > 0 ? ` (${breach.length})` : ""}
            </h2>
            {breach.length === 0 ? (
              <p className="hnr-section__empty">
                Nothing in breach. Good record.
              </p>
            ) : (
              <div className="hnr-card-grid">
                {breach.map((rec) => (
                  <BreachCard
                    key={rec.id}
                    rec={rec}
                    price={prices[rec.id]}
                    balance={balance}
                    clearing={clearingId === rec.id}
                    onClear={() => handleClear(rec.id)}
                  />
                ))}
              </div>
            )}
          </section>

          <section className="hnr-section">
            <h2 className="hnr-section__title">
              Seeding in progress
              {monitoring.length > 0 ? ` (${monitoring.length})` : ""}
            </h2>
            {monitoring.length === 0 ? (
              <p className="hnr-section__empty">
                Nothing currently being tracked.
              </p>
            ) : (
              <div className="hnr-card-grid">
                {monitoring.map((rec) => (
                  <MonitoredCard
                    key={rec.id}
                    rec={rec}
                    price={prices[rec.id]}
                    balance={balance}
                    clearing={clearingId === rec.id}
                    onClear={() => handleClear(rec.id)}
                  />
                ))}
              </div>
            )}
          </section>

          <section className="hnr-section">
            <h2 className="hnr-section__title">Resolved</h2>
            {resolved.length === 0 ? (
              <p className="hnr-section__empty">Nothing resolved yet.</p>
            ) : (
              <div className="hnr-row-list">
                {resolved.map((rec) => (
                  <ResolvedRow key={rec.id} rec={rec} />
                ))}
              </div>
            )}
          </section>
        </>
      )}
    </div>
  );
}
