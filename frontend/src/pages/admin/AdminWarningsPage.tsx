import { useCallback, useEffect, useRef, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { timeAgo } from "@/utils/format";
import { Pagination } from "@/components/Pagination";
import { UsernameDisplay } from "@/components/UsernameDisplay";
import "./admin-warnings.css";

interface Warning {
  id: number;
  user_id: number;
  type: string;
  reason: string;
  issued_by: number | null;
  status: string;
  lifted_at: string | null;
  lifted_by: number | null;
  lifted_reason: string | null;
  expires_at: string | null;
  created_at: string;
  username: string;
  issued_by_name: string | null;
  lifted_by_name: string | null;
}

const PER_PAGE = 25;

const STATUS_OPTIONS = [
  { value: "all", label: "All" },
  { value: "active", label: "Active" },
  { value: "lifted", label: "Lifted" },
  { value: "resolved", label: "Resolved" },
  { value: "escalated", label: "Escalated" },
];

function StatusBadge({ status }: { status: string }) {
  return (
    <span
      className={`admin-warnings__status admin-warnings__status--${status}`}
    >
      {status}
    </span>
  );
}

function warningTypeLabel(type: string): string {
  switch (type) {
    case "manual":
      return "Manual";
    case "ratio_soft":
      return "Ratio Warning";
    case "ratio_ban":
      return "Ratio Ban";
    default:
      return type;
  }
}

export function AdminWarningsPage() {
  const toast = useToast();

  const [warnings, setWarnings] = useState<Warning[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState("active");
  const [searchInput, setSearchInput] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  // Issue warning modal
  const [showIssueModal, setShowIssueModal] = useState(false);
  const [issueUserId, setIssueUserId] = useState<number | null>(null);
  const [issueUsername, setIssueUsername] = useState("");
  const [issueReason, setIssueReason] = useState("");
  const [issueExpiresAt, setIssueExpiresAt] = useState("");
  const [issuing, setIssuing] = useState(false);

  // User autocomplete for issue modal
  const [issueSuggestions, setIssueSuggestions] = useState<
    Array<{ id: number; username: string }>
  >([]);
  const [showIssueSuggestions, setShowIssueSuggestions] = useState(false);
  const [issueSuggestionLoading, setIssueSuggestionLoading] = useState(false);
  const issueDebounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const issueSuggestionRef = useRef<HTMLDivElement>(null);

  // Lift warning modal
  const [liftingWarningId, setLiftingWarningId] = useState<number | null>(null);
  const [liftReason, setLiftReason] = useState("");
  const [lifting, setLifting] = useState(false);

  const fetchWarnings = useCallback(async () => {
    setLoading(true);
    const token = getAccessToken();
    const params = new URLSearchParams();
    params.set("page", String(page));
    params.set("per_page", String(PER_PAGE));
    if (statusFilter) params.set("status", statusFilter);
    if (searchQuery.trim()) params.set("search", searchQuery.trim());

    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/warnings?${params}`,
        { headers: { Authorization: `Bearer ${token}` } },
      );
      if (res.ok) {
        const data = await res.json();
        setWarnings(data.warnings ?? []);
        setTotal(data.total ?? 0);
      }
    } finally {
      setLoading(false);
    }
  }, [page, statusFilter, searchQuery]);

  useEffect(() => {
    fetchWarnings();
  }, [fetchWarnings]);

  // Debounce search input
  useEffect(() => {
    if (searchInput === searchQuery) return;
    if (!searchInput.trim()) {
      clearTimeout(debounceRef.current);
      setSearchQuery("");
      setPage(1);
      return;
    }
    debounceRef.current = setTimeout(() => {
      setSearchQuery(searchInput);
      setPage(1);
    }, 250);
    return () => clearTimeout(debounceRef.current);
  }, [searchInput, searchQuery]);

  // Debounced username search for issue modal autocomplete
  const searchIssueUsers = useCallback((query: string) => {
    clearTimeout(issueDebounceRef.current);
    if (query.length < 2) {
      setIssueSuggestions([]);
      setShowIssueSuggestions(false);
      return;
    }
    setIssueSuggestionLoading(true);
    issueDebounceRef.current = setTimeout(async () => {
      try {
        const token = getAccessToken();
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/users?search=${encodeURIComponent(query)}&per_page=8`,
          { headers: { Authorization: `Bearer ${token}` } },
        );
        if (res.ok) {
          const data = await res.json();
          setIssueSuggestions(
            (data?.users ?? []).map((u: { id: number; username: string }) => ({
              id: u.id,
              username: u.username,
            })),
          );
          setShowIssueSuggestions(true);
        }
      } catch {
        // ignore
      } finally {
        setIssueSuggestionLoading(false);
      }
    }, 250);
  }, []);

  // Close autocomplete when clicking outside
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (
        issueSuggestionRef.current &&
        !issueSuggestionRef.current.contains(e.target as Node)
      ) {
        setShowIssueSuggestions(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  // Clean up issue debounce on unmount
  useEffect(() => {
    return () => clearTimeout(issueDebounceRef.current);
  }, []);

  const handleIssue = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!issueUserId || !issueReason.trim()) return;

    setIssuing(true);
    const token = getAccessToken();
    try {
      const body: Record<string, unknown> = {
        user_id: issueUserId,
        reason: issueReason.trim(),
      };
      if (issueExpiresAt) {
        body.expires_at = new Date(issueExpiresAt).toISOString();
      }

      const res = await fetch(`${getConfig().API_URL}/api/v1/admin/warnings`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify(body),
      });

      if (res.ok) {
        toast.success("Warning issued");
        setShowIssueModal(false);
        setIssueUserId(null);
        setIssueUsername("");
        setIssueReason("");
        setIssueExpiresAt("");
        fetchWarnings();
      } else {
        const data = await res.json();
        toast.error(data?.error?.message ?? "Failed to issue warning");
      }
    } finally {
      setIssuing(false);
    }
  };

  const handleLift = async () => {
    if (liftingWarningId === null) return;

    setLifting(true);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/warnings/${liftingWarningId}/lift`,
        {
          method: "POST",
          headers: {
            Authorization: `Bearer ${token}`,
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ reason: liftReason.trim() }),
        },
      );

      if (res.ok || res.status === 204) {
        toast.success("Warning lifted");
        setLiftingWarningId(null);
        setLiftReason("");
        fetchWarnings();
      } else {
        const data = await res.json();
        toast.error(data?.error?.message ?? "Failed to lift warning");
      }
    } finally {
      setLifting(false);
    }
  };

  const totalPages = Math.ceil(total / PER_PAGE);

  return (
    <div>
      <h1>Warnings</h1>

      <div className="admin-warnings__controls">
        <div className="admin-warnings__filter">
          <label htmlFor="warning-status">Status</label>
          <select
            id="warning-status"
            value={statusFilter}
            onChange={(e) => {
              setStatusFilter(e.target.value);
              setPage(1);
            }}
          >
            {STATUS_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>

        <div className="admin-warnings__filter">
          <label htmlFor="warning-search">Username</label>
          <input
            id="warning-search"
            type="text"
            placeholder="Search by username"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
          />
        </div>

        <button
          className="admin-warnings__issue-btn"
          onClick={() => setShowIssueModal(true)}
        >
          Issue Warning
        </button>
      </div>

      {loading ? (
        <p>Loading...</p>
      ) : warnings.length === 0 ? (
        <p className="admin-warnings__empty">No warnings found.</p>
      ) : (
        <>
          <table className="admin-warnings__table">
            <thead>
              <tr>
                <th>User</th>
                <th>Type</th>
                <th>Reason</th>
                <th>Status</th>
                <th>Issued By</th>
                <th>Date</th>
                <th>Expires</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {warnings.map((w) => (
                <tr key={w.id}>
                  <td>
                    <UsernameDisplay
                      userId={w.user_id}
                      username={w.username}
                    />
                  </td>
                  <td>{warningTypeLabel(w.type)}</td>
                  <td className="admin-warnings__reason-cell" title={w.reason}>
                    {w.reason}
                    {w.status === "lifted" && w.lifted_reason && (
                      <div className="admin-warnings__lifted-info">
                        Lifted by {w.lifted_by_name ?? "?"}: {w.lifted_reason}
                      </div>
                    )}
                  </td>
                  <td>
                    <StatusBadge status={w.status} />
                  </td>
                  <td>
                    {w.issued_by_name ??
                      (w.issued_by ? `#${w.issued_by}` : "System")}
                  </td>
                  <td>{timeAgo(w.created_at)}</td>
                  <td>{w.expires_at ? timeAgo(w.expires_at) : "—"}</td>
                  <td>
                    {w.status === "active" && (
                      <button
                        className="admin-warnings__lift-btn"
                        onClick={() => setLiftingWarningId(w.id)}
                      >
                        Lift
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {totalPages > 1 && (
            <Pagination
              currentPage={page}
              totalPages={totalPages}
              onPageChange={setPage}
            />
          )}
        </>
      )}

      {/* Issue Warning Modal */}
      {showIssueModal && (
        <div
          className="admin-warnings__modal-overlay"
          onClick={() => setShowIssueModal(false)}
        >
          <div
            className="admin-warnings__modal"
            onClick={(e) => e.stopPropagation()}
          >
            <h3>Issue Warning</h3>
            <form onSubmit={handleIssue}>
              <div
                className="admin-warnings__modal-field"
                ref={issueSuggestionRef}
              >
                <label htmlFor="issue-username">Username</label>
                <div className="admin-warnings__autocomplete">
                  <input
                    id="issue-username"
                    type="text"
                    value={issueUsername}
                    onChange={(e) => {
                      setIssueUsername(e.target.value);
                      setIssueUserId(null);
                      searchIssueUsers(e.target.value);
                    }}
                    onFocus={() => {
                      if (issueSuggestions.length > 0)
                        setShowIssueSuggestions(true);
                    }}
                    required
                    placeholder="Search username..."
                    autoComplete="off"
                  />
                  {issueSuggestionLoading && (
                    <span className="admin-warnings__autocomplete-loading">
                      ...
                    </span>
                  )}
                  {showIssueSuggestions && issueSuggestions.length > 0 && (
                    <ul className="admin-warnings__autocomplete-list">
                      {issueSuggestions.map((u) => (
                        <li key={u.id}>
                          <button
                            type="button"
                            className="admin-warnings__autocomplete-item"
                            onClick={() => {
                              setIssueUsername(u.username);
                              setIssueUserId(u.id);
                              setShowIssueSuggestions(false);
                            }}
                          >
                            {u.username}
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </div>
              <div className="admin-warnings__modal-field">
                <label htmlFor="issue-reason">Reason</label>
                <textarea
                  id="issue-reason"
                  value={issueReason}
                  onChange={(e) => setIssueReason(e.target.value)}
                  required
                />
              </div>
              <div className="admin-warnings__modal-field">
                <label htmlFor="issue-expires">Expires At (optional)</label>
                <input
                  id="issue-expires"
                  type="datetime-local"
                  value={issueExpiresAt}
                  onChange={(e) => setIssueExpiresAt(e.target.value)}
                />
              </div>
              <div className="admin-warnings__modal-actions">
                <button
                  type="button"
                  className="admin-warnings__modal-cancel"
                  onClick={() => setShowIssueModal(false)}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="admin-warnings__modal-submit"
                  disabled={issuing || !issueUserId || !issueReason.trim()}
                >
                  {issuing ? "Issuing..." : "Issue Warning"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Lift Warning Modal */}
      {liftingWarningId !== null && (
        <div
          className="admin-warnings__modal-overlay"
          onClick={() => setLiftingWarningId(null)}
        >
          <div
            className="admin-warnings__modal"
            onClick={(e) => e.stopPropagation()}
          >
            <h3>Lift Warning</h3>
            <div className="admin-warnings__modal-field">
              <label htmlFor="lift-reason">Reason (optional)</label>
              <textarea
                id="lift-reason"
                value={liftReason}
                onChange={(e) => setLiftReason(e.target.value)}
                placeholder="Why are you lifting this warning?"
              />
            </div>
            <div className="admin-warnings__modal-actions">
              <button
                type="button"
                className="admin-warnings__modal-cancel"
                onClick={() => {
                  setLiftingWarningId(null);
                  setLiftReason("");
                }}
              >
                Cancel
              </button>
              <button
                className="admin-warnings__modal-submit"
                onClick={handleLift}
                disabled={lifting}
              >
                {lifting ? "Lifting..." : "Lift Warning"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
