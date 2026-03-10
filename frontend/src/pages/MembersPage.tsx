import { useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { Input } from "@/components/form";
import { Pagination } from "@/components/Pagination";
import { formatBytes, formatRatio, formatDate } from "@/utils/format";
import { UsernameDisplay } from "@/components/UsernameDisplay";
import "./members.css";

interface MemberUser {
  id: number;
  username: string;
  group_id: number;
  group_name: string;
  uploaded: number;
  downloaded: number;
  ratio: number;
  donor: boolean;
  warned: boolean;
  created_at: string;
}

const PER_PAGE = 25;

export function MembersPage() {
  const [searchParams, setSearchParams] = useSearchParams();

  const query = searchParams.get("q") ?? "";
  const page = Math.max(1, Number(searchParams.get("page")) || 1);

  const [searchInput, setSearchInput] = useState(query);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const [members, setMembers] = useState<MemberUser[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function fetchMembers() {
      setLoading(true);
      setError(null);

      try {
        const token = getAccessToken();
        const params = new URLSearchParams();
        if (query) params.set("search", query);
        params.set("page", String(page));
        params.set("per_page", String(PER_PAGE));

        const res = await fetch(
          `${getConfig().API_URL}/api/v1/users?${params.toString()}`,
          {
            headers: token ? { Authorization: `Bearer ${token}` } : {},
          },
        );

        if (cancelled) return;

        const body = await res.json();

        if (!res.ok) {
          setError(body?.error?.message ?? "Failed to load members");
          setLoading(false);
          return;
        }

        setMembers(body?.users ?? []);
        setTotal(body?.total ?? 0);
      } catch {
        if (!cancelled) {
          setError("Failed to load members");
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    fetchMembers();
    return () => {
      cancelled = true;
    };
  }, [query, page]);

  const totalPages = Math.max(1, Math.ceil(total / PER_PAGE));

  const setParam = useCallback(
    (key: string, value: string) => {
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev);
        if (value) {
          next.set(key, value);
        } else {
          next.delete(key);
        }
        if (key !== "page") next.delete("page");
        return next;
      });
    },
    [setSearchParams],
  );

  useEffect(() => {
    if (searchInput === query) return;
    if (!searchInput.trim()) {
      clearTimeout(debounceRef.current);
      setParam("q", "");
      return;
    }
    debounceRef.current = setTimeout(() => {
      setParam("q", searchInput);
    }, 250);
    return () => clearTimeout(debounceRef.current);
  }, [searchInput, query, setParam]);

  return (
    <div className="members">
      <div className="members__header">
        <h1 className="members__title">Members</h1>
        <div className="members__search">
          <Input
            label="Search"
            placeholder="Search members..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
          />
        </div>
      </div>

      {loading ? (
        <div className="members__loading">Loading members...</div>
      ) : error ? (
        <div className="members__error">{error}</div>
      ) : members.length === 0 ? (
        <div className="members__empty">No members found.</div>
      ) : (
        <table className="members__table">
          <thead>
            <tr>
              <th>Username</th>
              <th>Group</th>
              <th>Uploaded</th>
              <th>Downloaded</th>
              <th>Ratio</th>
              <th>Joined</th>
            </tr>
          </thead>
          <tbody>
            {members.map((m) => (
              <tr key={m.id}>
                <td>
                  <UsernameDisplay
                    userId={m.id}
                    username={m.username}
                    warned={m.warned}
                    className="members__username"
                  />
                  {m.donor && (
                    <span className="members__donor-badge">Donor</span>
                  )}
                </td>
                <td>{m.group_name}</td>
                <td>{formatBytes(m.uploaded)}</td>
                <td>{formatBytes(m.downloaded)}</td>
                <td
                  className={
                    m.ratio >= 1
                      ? "members__ratio--good"
                      : "members__ratio--bad"
                  }
                >
                  {formatRatio(m.ratio)}
                </td>
                <td>{formatDate(m.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {!loading && !error && totalPages > 1 && (
        <Pagination
          currentPage={page}
          totalPages={totalPages}
          onPageChange={(p) => setParam("page", String(p))}
        />
      )}
    </div>
  );
}
