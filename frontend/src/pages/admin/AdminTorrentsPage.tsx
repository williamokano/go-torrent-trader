import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { Input, Select } from "@/components/form";
import { Pagination } from "@/components/Pagination";
import { ConfirmModal } from "@/components/modal/ConfirmModal";
import { formatBytes, timeAgo } from "@/utils/format";
import "./admin-ui.css";

interface AdminTorrent {
  id: number;
  name: string;
  info_hash: string;
  size: number;
  seeders: number;
  leechers: number;
  uploader_id: number;
  uploader: string;
  banned: boolean;
  free: boolean;
  silver: boolean;
  hnr_exempt: boolean;
  visible: boolean;
  created_at: string;
}

type BulkAction = "ban" | "unban" | "delete";

interface BulkResult {
  id: number;
  status: "ok" | "not_found" | "forbidden" | "error";
}

/** A queued confirmation. Single-torrent delete and bulk actions share one modal. */
type PendingAction =
  | { kind: "delete-one"; id: number }
  | { kind: "bulk"; action: BulkAction; ids: number[] };

const PER_PAGE = 25;

/** Matches the backend's rule: 40 hex characters cannot be a torrent name. */
const INFO_HASH_RE = /^[0-9a-fA-F]{40}$/;

const BULK_LABELS: Record<BulkAction, string> = {
  ban: "Ban",
  unban: "Unban",
  delete: "Delete",
};

/** Spelled out rather than derived — "delete" + "ned" is not a word. */
const BULK_PAST_TENSE: Record<BulkAction, string> = {
  ban: "banned",
  unban: "unbanned",
  delete: "deleted",
};

/**
 * What the confirm button says while the request is in flight. The modal's default
 * is "Deleting...", which is alarming copy to show someone who clicked Unban.
 */
const BULK_PROGRESS: Record<BulkAction, string> = {
  ban: "Banning...",
  unban: "Unbanning...",
  delete: "Deleting...",
};

export function AdminTorrentsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const toast = useToast();

  const query = searchParams.get("q") ?? "";
  // Whitelisted, not passed through. Anything else became `banned=false` while the
  // Select — being controlled with an unmatched value — displayed "All": the UI said
  // one thing and the query did another. A hand-typed or bookmarked URL is enough.
  const rawStatus = searchParams.get("status") ?? "";
  const status =
    rawStatus === "banned" || rawStatus === "active" ? rawStatus : "";
  const page = Math.max(1, Number(searchParams.get("page")) || 1);

  const [searchInput, setSearchInput] = useState(query);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const [torrents, setTorrents] = useState<AdminTorrent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<number[]>([]);
  const [pending, setPending] = useState<PendingAction | null>(null);
  const [working, setWorking] = useState(false);

  const queryIsHash = INFO_HASH_RE.test(query.trim());
  // The backend takes `name=` to mean "search the name even though this parses as
  // a hash". Without a way to reach it, a release genuinely titled like a SHA1 —
  // or a git commit id — was unfindable by name through this page, which is the
  // opposite of the escape hatch's purpose. In the URL rather than in state so a
  // bookmarked or shared lookup keeps meaning what it meant.
  const forceNameSearch = searchParams.get("by") === "name";
  // One flag for both the request and the controls, so they cannot disagree.
  const hashLookup = queryIsHash && !forceNameSearch;

  const fetchTorrents = useCallback(async () => {
    setLoading(true);
    setError(null);
    const token = getAccessToken();
    const params = new URLSearchParams();
    if (query) {
      // `name=` forces the name branch; `search=` lets the backend route a
      // 40-hex-character term to the hash.
      params.set(forceNameSearch ? "name" : "search", query);
    }
    // A hash identifies exactly one torrent, so conjoining a status predicate with
    // it can only ever hide the answer. A takedown notice looked up with Status left
    // on "Banned" would report "not on this tracker" for a torrent that is.
    if (status && !hashLookup) {
      params.set("banned", status === "banned" ? "true" : "false");
    }
    params.set("page", String(page));
    params.set("per_page", String(PER_PAGE));

    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/torrents?${params}`,
        {
          headers: { Authorization: `Bearer ${token}` },
        },
      );
      if (res.ok) {
        const data = await res.json();
        setTorrents(data.torrents ?? []);
        setTotal(data.total ?? 0);
      } else {
        // Distinct from the empty state. "No torrents found" in answer to a failed
        // request is a wrong answer an operator will act on — and with the banned
        // filter it is a believable one.
        setError("Couldn't load torrents.");
      }
    } catch {
      setError("Couldn't load torrents.");
    } finally {
      setLoading(false);
    }
  }, [query, status, page, hashLookup, forceNameSearch]);

  useEffect(() => {
    fetchTorrents();
  }, [fetchTorrents]);

  // Selection is cleared wherever the visible set changes, in the handler rather
  // than an effect: a checkbox left ticked after the list moved on would ban a
  // torrent the operator can no longer see.
  const updateParams = (mutate: (next: URLSearchParams) => void) => {
    const next = new URLSearchParams(searchParams);
    mutate(next);
    setSelected([]);
    setSearchParams(next);
  };

  const handleSearchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    setSearchInput(val);
    clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      updateParams((next) => {
        if (val) {
          next.set("q", val);
        } else {
          next.delete("q");
        }
        next.delete("page");
      });
    }, 250);
  };

  const handleStatusChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const val = e.target.value;
    updateParams((next) => {
      if (val) {
        next.set("status", val);
      } else {
        next.delete("status");
      }
      next.delete("page");
    });
  };

  // Switches a hash-shaped term between the hash lookup and the name search.
  // Resets pagination: the two searches return unrelated result sets, so page 3
  // of one is meaningless in the other.
  const toggleSearchField = () => {
    updateParams((next) => {
      if (forceNameSearch) {
        next.delete("by");
      } else {
        next.set("by", "name");
      }
      next.delete("page");
    });
  };

  const handlePageChange = (newPage: number) => {
    updateParams((next) => next.set("page", String(newPage)));
  };

  const toggleOne = (id: number) => {
    setSelected((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id],
    );
  };

  const allOnPageSelected =
    torrents.length > 0 && torrents.every((t) => selected.includes(t.id));

  const toggleAllOnPage = () => {
    setSelected(allOnPageSelected ? [] : torrents.map((t) => t.id));
  };

  const runDeleteOne = async (id: number) => {
    const token = getAccessToken();
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/torrents/${id}`,
      { method: "DELETE", headers: { Authorization: `Bearer ${token}` } },
    );
    if (res.ok) {
      toast.success("Torrent deleted");
    } else {
      toast.error("Failed to delete torrent");
    }
  };

  const runBulk = async (action: BulkAction, ids: number[]) => {
    const token = getAccessToken();
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/torrents/bulk`,
      {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ action, ids }),
      },
    );

    if (!res.ok) {
      const body = await res.json().catch(() => null);
      toast.error(body?.error?.message ?? "Bulk action failed");
      return;
    }

    // A 200 does not mean every torrent was actioned. Reporting only the total
    // would tell an operator their ban worked when a third of it silently did not.
    const body = await res.json();
    const succeeded: number = body.succeeded ?? 0;
    const failed: number = body.failed ?? 0;
    const past = BULK_PAST_TENSE[action];

    if (failed === 0) {
      toast.success(
        `${succeeded} torrent${succeeded === 1 ? "" : "s"} ${past}`,
      );
      return;
    }
    const reasons = summariseFailures(body.results ?? []);
    toast.error(`${succeeded} ${past}, ${failed} could not be: ${reasons}`);
  };

  const handleConfirm = async () => {
    if (!pending) return;
    setWorking(true);
    try {
      if (pending.kind === "delete-one") {
        await runDeleteOne(pending.id);
      } else {
        await runBulk(pending.action, pending.ids);
      }
      setSelected([]);
      await fetchTorrents();
    } catch {
      // Without this the rejection escapes, the modal closes, and nothing is said:
      // the operator clicks "Delete 20 torrents", the dialog vanishes, and they have
      // no way to know the request never left.
      toast.error(
        "Couldn't reach the server — none of it may have been applied",
      );
    } finally {
      setWorking(false);
      setPending(null);
    }
  };

  const totalPages = Math.ceil(total / PER_PAGE);
  const searchLooksLikeHash = INFO_HASH_RE.test(searchInput.trim());

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title">Torrents</h1>
          <p className="admin-page-header__desc">
            Search and moderate every torrent on the tracker.
          </p>
        </div>
      </div>

      <div className="admin-toolbar">
        <div className="admin-toolbar__search">
          <Input
            label="Search"
            placeholder="Torrent name or info hash..."
            value={searchInput}
            onChange={handleSearchChange}
          />
          {searchLooksLikeHash && (
            <p className="admin-muted admin-hint">
              {forceNameSearch ? (
                <>
                  Searching by name.{" "}
                  <button
                    type="button"
                    className="admin-linkbtn"
                    onClick={toggleSearchField}
                  >
                    Look up by info hash instead
                  </button>
                </>
              ) : (
                <>
                  Looking up by info hash
                  {status && ", ignoring the status filter"}.{" "}
                  <button
                    type="button"
                    className="admin-linkbtn"
                    onClick={toggleSearchField}
                  >
                    Search by name instead
                  </button>
                </>
              )}
            </p>
          )}
        </div>
        <Select
          label="Status"
          // Disabled rather than merely ignored. Rendering "Banned" as the
          // selected value while the request deliberately drops it is the same
          // UI-says-one-thing-query-does-another defect this page fixed for an
          // unrecognised ?status= — an operator would read an Active row as
          // proof the torrent is banned.
          disabled={hashLookup}
          value={hashLookup ? "" : status}
          onChange={handleStatusChange}
          options={[
            { value: "", label: hashLookup ? "Not applied" : "All" },
            { value: "active", label: "Not banned" },
            { value: "banned", label: "Banned" },
          ]}
        />
      </div>

      {selected.length > 0 && (
        <div className="admin-panel admin-bulkbar">
          <span className="admin-bulkbar__count">
            {selected.length} selected
          </span>
          <div className="admin-bulkbar__actions">
            {(["ban", "unban", "delete"] as BulkAction[]).map((action) => (
              <button
                key={action}
                className={`admin-btn admin-btn--sm ${
                  action === "delete" ? "admin-btn--danger" : "admin-btn--ghost"
                }`}
                onClick={() =>
                  setPending({ kind: "bulk", action, ids: selected })
                }
              >
                {BULK_LABELS[action]} selected
              </button>
            ))}
            <button
              className="admin-btn admin-btn--ghost admin-btn--sm"
              onClick={() => setSelected([])}
            >
              Clear
            </button>
          </div>
        </div>
      )}

      {loading ? (
        <p>Loading...</p>
      ) : error ? (
        <div className="admin-panel">
          <p className="admin-empty">{error}</p>
          <button
            className="admin-btn admin-btn--ghost admin-btn--sm"
            onClick={fetchTorrents}
          >
            Try again
          </button>
        </div>
      ) : torrents.length === 0 ? (
        <div className="admin-panel">
          <p className="admin-empty">No torrents found.</p>
        </div>
      ) : (
        <>
          <div className="admin-panel">
            <div className="admin-table-scroll">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>
                      <input
                        type="checkbox"
                        aria-label="Select all on this page"
                        checked={allOnPageSelected}
                        onChange={toggleAllOnPage}
                      />
                    </th>
                    <th>Name</th>
                    <th>Info hash</th>
                    <th>Size</th>
                    <th>S/L</th>
                    <th>Uploader</th>
                    <th>Status</th>
                    <th>Created</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {torrents.map((torrent) => (
                    <tr key={torrent.id}>
                      <td>
                        <input
                          type="checkbox"
                          aria-label={`Select ${torrent.name}`}
                          checked={selected.includes(torrent.id)}
                          onChange={() => toggleOne(torrent.id)}
                        />
                      </td>
                      <td className="admin-table__name">
                        <Link to={`/torrent/${torrent.id}`}>
                          {torrent.name}
                        </Link>
                      </td>
                      <td>
                        {/* The full hash is in the DOM and truncated by CSS, not
                            sliced: a tooltip cannot be selected, and copying it
                            back out — into a report, a reply to a takedown — is
                            the reason this column exists. Guarded because one
                            missing field should not blank the whole moderation
                            page; that the backend sends it is asserted there, in
                            TestAdminListTorrentsReturnsTheHashAndFlags. */}
                        <code className="admin-hash" title={torrent.info_hash}>
                          {torrent.info_hash ?? ""}
                        </code>
                      </td>
                      <td className="admin-num">{formatBytes(torrent.size)}</td>
                      <td className="admin-num">
                        {torrent.seeders}/{torrent.leechers}
                      </td>
                      <td>
                        <Link to={`/admin/users/${torrent.uploader_id}`}>
                          {torrent.uploader || `User #${torrent.uploader_id}`}
                        </Link>
                      </td>
                      <td className="admin-flags">
                        {torrent.banned ? (
                          <span className="admin-badge admin-badge--danger">
                            Banned
                          </span>
                        ) : (
                          <span className="admin-badge admin-badge--ok">
                            Active
                          </span>
                        )}
                        {torrent.free && (
                          <span className="admin-badge admin-badge--accent">
                            Free
                          </span>
                        )}
                        {torrent.silver && (
                          <span className="admin-badge admin-badge--accent">
                            Half
                          </span>
                        )}
                        {torrent.hnr_exempt && (
                          <span className="admin-badge admin-badge--muted">
                            No H&amp;R
                          </span>
                        )}
                        {!torrent.visible && (
                          <span className="admin-badge admin-badge--muted">
                            Hidden
                          </span>
                        )}
                      </td>
                      <td className="admin-muted">
                        {timeAgo(torrent.created_at)}
                      </td>
                      <td className="admin-table__actions">
                        <button
                          className="admin-btn admin-btn--danger admin-btn--sm"
                          onClick={() =>
                            setPending({ kind: "delete-one", id: torrent.id })
                          }
                        >
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          <div style={{ marginTop: "var(--space-md)" }}>
            <Pagination
              currentPage={page}
              totalPages={totalPages}
              onPageChange={handlePageChange}
            />
          </div>
        </>
      )}

      <ConfirmModal
        isOpen={pending !== null}
        title={confirmTitle(pending)}
        message={confirmMessage(pending)}
        confirmLabel={
          pending?.kind === "bulk" ? BULK_LABELS[pending.action] : "Delete"
        }
        loadingLabel={
          pending?.kind === "bulk"
            ? BULK_PROGRESS[pending.action]
            : "Deleting..."
        }
        danger={pending?.kind !== "bulk" || pending.action !== "unban"}
        loading={working}
        onConfirm={handleConfirm}
        onCancel={() => setPending(null)}
      />
    </div>
  );
}

function confirmTitle(pending: PendingAction | null): string {
  if (!pending) return "";
  if (pending.kind === "delete-one") return "Delete Torrent";
  return `${BULK_LABELS[pending.action]} ${pending.ids.length} Torrent${
    pending.ids.length === 1 ? "" : "s"
  }`;
}

function confirmMessage(pending: PendingAction | null): string {
  if (!pending) return "";
  if (pending.kind === "delete-one") {
    return "Are you sure you want to permanently delete this torrent and its files? This cannot be undone.";
  }
  const count = `${pending.ids.length} torrent${pending.ids.length === 1 ? "" : "s"}`;
  switch (pending.action) {
    case "delete":
      return `Permanently delete ${count} and their files? This cannot be undone.`;
    case "ban":
      return `Ban ${count}? Members will no longer be able to download or announce on them.`;
    case "unban":
      return `Unban ${count}? They become downloadable again.`;
  }
}

/**
 * Turns the per-torrent breakdown into something an operator can act on. "3 could
 * not be" is not useful on its own; "3 not found" tells them the ids were stale.
 */
function summariseFailures(results: BulkResult[]): string {
  const counts = new Map<string, number>();
  for (const r of results) {
    if (r.status === "ok") continue;
    counts.set(r.status, (counts.get(r.status) ?? 0) + 1);
  }
  const labels: Record<string, string> = {
    not_found: "not found",
    forbidden: "not permitted",
    error: "failed",
  };
  return [...counts.entries()]
    .map(([status, n]) => `${n} ${labels[status] ?? status}`)
    .join(", ");
}
