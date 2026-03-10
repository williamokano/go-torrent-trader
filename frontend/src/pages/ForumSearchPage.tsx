import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { timeAgo } from "@/utils/format";
import { UsernameDisplay } from "@/components/UsernameDisplay";
import { Pagination } from "@/components/Pagination";
import "./forums.css";

interface SearchResult {
  post_id: number;
  body: string;
  topic_id: number;
  topic_title: string;
  forum_id: number;
  forum_name: string;
  user_id: number;
  username: string;
  created_at: string;
}

interface ForumOption {
  id: number;
  name: string;
}

interface CategoryData {
  id: number;
  name: string;
  forums: ForumOption[];
}

const PER_PAGE = 25;
const DEBOUNCE_MS = 250;

function truncateBody(body: string, maxLen = 200): string {
  if (body.length <= maxLen) return body;
  return body.slice(0, maxLen).trimEnd() + "...";
}

export function ForumSearchPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const query = searchParams.get("q") ?? "";
  const forumId = searchParams.get("forum_id") ?? "";
  const page = parseInt(searchParams.get("page") ?? "1", 10) || 1;

  const [inputValue, setInputValue] = useState(query);
  const [results, setResults] = useState<SearchResult[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);
  const [forums, setForums] = useState<ForumOption[]>([]);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Fetch forum list for the filter dropdown
  useEffect(() => {
    let cancelled = false;
    async function fetchForums() {
      try {
        const token = getAccessToken();
        const res = await fetch(`${getConfig().API_URL}/api/v1/forums`, {
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        });
        if (!res.ok) return;
        const data = await res.json();
        const cats: CategoryData[] = data.categories ?? [];
        const allForums: ForumOption[] = [];
        for (const cat of cats) {
          for (const f of cat.forums) {
            allForums.push({ id: f.id, name: f.name });
          }
        }
        if (!cancelled) setForums(allForums);
      } catch {
        // ignore – dropdown just won't show
      }
    }
    fetchForums();
    return () => {
      cancelled = true;
    };
  }, []);

  // Execute search when URL params change
  const executeSearch = useCallback(async () => {
    if (!query.trim()) {
      setResults([]);
      setTotal(0);
      setSearched(false);
      return;
    }
    setLoading(true);
    setSearched(true);
    try {
      const token = getAccessToken();
      const params = new URLSearchParams({
        q: query,
        page: String(page),
        per_page: String(PER_PAGE),
      });
      if (forumId) params.set("forum_id", forumId);

      const res = await fetch(
        `${getConfig().API_URL}/api/v1/forums/search?${params}`,
        {
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        },
      );
      if (!res.ok) throw new Error("Search failed");
      const data = await res.json();
      setResults(data.results ?? []);
      setTotal(data.total ?? 0);
    } catch {
      setResults([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [query, forumId, page]);

  useEffect(() => {
    executeSearch();
  }, [executeSearch]);

  // Sync inputValue when URL query changes (e.g. back/forward nav)
  useEffect(() => {
    setInputValue(query);
  }, [query]);

  function updateParams(updates: Record<string, string>) {
    const next = new URLSearchParams(searchParams);
    for (const [k, v] of Object.entries(updates)) {
      if (v) {
        next.set(k, v);
      } else {
        next.delete(k);
      }
    }
    setSearchParams(next, { replace: true });
  }

  function handleInputChange(value: string) {
    setInputValue(value);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      updateParams({ q: value, page: "" });
    }, DEBOUNCE_MS);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (debounceRef.current) clearTimeout(debounceRef.current);
    updateParams({ q: inputValue, page: "" });
  }

  function handleForumChange(value: string) {
    updateParams({ forum_id: value, page: "" });
  }

  function handlePageChange(newPage: number) {
    updateParams({ page: String(newPage) });
  }

  const totalPages = Math.ceil(total / PER_PAGE);

  return (
    <div className="forum-search-page">
      <div className="forum-search-page__breadcrumb">
        <Link to="/forums">Forums</Link> &raquo; Search
      </div>

      <h1>Forum Search</h1>

      <form className="forum-search-page__form" onSubmit={handleSubmit}>
        <input
          type="text"
          className="forum-search-page__input"
          placeholder="Search forums..."
          value={inputValue}
          onChange={(e) => handleInputChange(e.target.value)}
          autoFocus
        />
        <select
          className="forum-search-page__forum-filter"
          value={forumId}
          onChange={(e) => handleForumChange(e.target.value)}
        >
          <option value="">All Forums</option>
          {forums.map((f) => (
            <option key={f.id} value={String(f.id)}>
              {f.name}
            </option>
          ))}
        </select>
        <button type="submit" className="btn btn--primary">
          Search
        </button>
      </form>

      {loading && (
        <div className="forum-search-page__loading">Searching...</div>
      )}

      {!loading && searched && results.length === 0 && (
        <p className="forum-empty">
          No results found for &ldquo;{query}&rdquo;.
        </p>
      )}

      {!loading && results.length > 0 && (
        <>
          <p className="forum-search-page__count">
            {total} result{total !== 1 ? "s" : ""} for &ldquo;{query}&rdquo;
          </p>
          <div className="forum-search-results">
            {results.map((r) => (
              <div key={r.post_id} className="forum-search-result">
                <div className="forum-search-result__topic">
                  <Link to={`/forums/topics/${r.topic_id}`}>
                    {r.topic_title}
                  </Link>
                </div>
                <div className="forum-search-result__snippet">
                  {truncateBody(r.body)}
                </div>
                <div className="forum-search-result__meta">
                  <Link to={`/forums/${r.forum_id}`}>{r.forum_name}</Link>
                  {" - "}
                  <UsernameDisplay userId={r.user_id} username={r.username} />
                  {" - "}
                  <span>{timeAgo(r.created_at)}</span>
                </div>
              </div>
            ))}
          </div>
          <Pagination
            currentPage={page}
            totalPages={totalPages}
            onPageChange={handlePageChange}
          />
        </>
      )}
    </div>
  );
}
