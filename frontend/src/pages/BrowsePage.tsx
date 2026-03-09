import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api } from "@/api";
import { getAccessToken } from "@/features/auth/token";
import { Input } from "@/components/form";
import { Select } from "@/components/form";
import { Pagination } from "@/components/Pagination";
import { formatBytes, timeAgo } from "@/utils/format";
import type { Torrent } from "@/types/torrent";
import { buildCategoryOptions } from "@/utils/categories";
import { CategoryIcon } from "@/components/CategoryIcon";
import "./browse.css";

const SORT_OPTIONS = [
  { value: "created_at", label: "Date" },
  { value: "name", label: "Name" },
  { value: "size", label: "Size" },
  { value: "seeders", label: "Seeders" },
  { value: "leechers", label: "Leechers" },
];

const PER_PAGE = 5;

type SortField = "name" | "created_at" | "size" | "seeders" | "leechers";

function healthClass(seeders: number): string {
  if (seeders > 5) return "browse__health--good";
  if (seeders >= 1) return "browse__health--warning";
  return "browse__health--dead";
}

export function BrowsePage() {
  const [searchParams, setSearchParams] = useSearchParams();

  const query = searchParams.get("q") ?? "";
  const category = searchParams.get("cat") ?? "";
  const sortBy = (searchParams.get("sort") as SortField) || "created_at";
  const sortDir = searchParams.get("dir") === "asc" ? "asc" : "desc";
  const page = Math.max(1, Number(searchParams.get("page")) || 1);

  // Debounced search: local input updates immediately, URL param after delay
  const [searchInput, setSearchInput] = useState(query);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const [torrents, setTorrents] = useState<Torrent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [categoryOptions, setCategoryOptions] = useState<
    { value: string; label: string }[]
  >([{ value: "", label: "All Categories" }]);

  useEffect(() => {
    async function fetchCategories() {
      const { data } = await api.GET("/api/v1/categories");
      if (data?.categories) {
        setCategoryOptions(
          buildCategoryOptions(
            data.categories as {
              id: number;
              name: string;
              parent_id: number | null;
              sort_order: number;
            }[],
            "All Categories",
          ),
        );
      }
    }
    fetchCategories();
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function fetchTorrents() {
      setLoading(true);
      setError(null);

      const token = getAccessToken();
      const { data, error: apiError } = await api.GET("/api/v1/torrents", {
        params: {
          query: {
            search: query || undefined,
            cat: category ? Number(category) : undefined,
            sort: sortBy,
            order: sortDir,
            page,
            per_page: PER_PAGE,
          },
        },
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });

      if (cancelled) return;

      if (apiError) {
        const msg =
          (apiError as { error?: { message?: string } }).error?.message ??
          "Failed to load torrents";
        setError(msg);
        setLoading(false);
        return;
      }

      setTorrents(data?.torrents ?? []);
      setTotal(data?.total ?? 0);
      setLoading(false);
    }

    fetchTorrents();
    return () => {
      cancelled = true;
    };
  }, [query, category, sortBy, sortDir, page]);

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

  // Debounce search input → URL param update
  useEffect(() => {
    if (searchInput === query) return;
    // Clear search: update immediately (no delay)
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

  const handleSort = useCallback(
    (field: SortField) => {
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev);
        if (next.get("sort") === field && next.get("dir") !== "asc") {
          next.set("dir", "asc");
        } else {
          next.set("sort", field);
          next.delete("dir");
        }
        next.delete("page");
        return next;
      });
    },
    [setSearchParams],
  );

  const sortIndicator = (field: SortField) => {
    if (sortBy !== field) return "";
    return sortDir === "asc" ? " \u25B2" : " \u25BC";
  };

  return (
    <div className="browse">
      <div className="browse__header">
        <h1 className="browse__title">Browse Torrents</h1>
        <div className="browse__controls">
          <div className="browse__search">
            <Input
              label="Search"
              placeholder="Search torrents..."
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
            />
          </div>
          <div className="browse__filter">
            <Select
              label="Category"
              options={categoryOptions}
              value={category}
              onChange={(e) => setParam("cat", e.target.value)}
            />
          </div>
          <div className="browse__sort">
            <Select
              label="Sort by"
              options={SORT_OPTIONS}
              value={sortBy}
              onChange={(e) => setParam("sort", e.target.value)}
            />
          </div>
        </div>
      </div>

      {loading ? (
        <div className="browse__loading">Loading torrents...</div>
      ) : error ? (
        <div className="browse__error">{error}</div>
      ) : torrents.length === 0 ? (
        <div className="browse__empty">No torrents found.</div>
      ) : (
        <table className="browse__table">
          <thead>
            <tr>
              <th title="Category">Cat.</th>
              <th onClick={() => handleSort("name")}>
                Name{sortIndicator("name")}
              </th>
              <th>Uploader</th>
              <th onClick={() => handleSort("size")}>
                Size{sortIndicator("size")}
              </th>
              <th onClick={() => handleSort("seeders")}>
                S{sortIndicator("seeders")}
              </th>
              <th onClick={() => handleSort("leechers")}>
                L{sortIndicator("leechers")}
              </th>
              <th onClick={() => handleSort("created_at")}>
                Uploaded{sortIndicator("created_at")}
              </th>
            </tr>
          </thead>
          <tbody>
            {torrents.map((t) => (
              <tr key={t.id}>
                <td>
                  <CategoryIcon
                    name={t.category_name ?? "?"}
                    imageUrl={t.category_image_url}
                  />
                </td>
                <td>
                  <span
                    className={`browse__health ${healthClass(t.seeders ?? 0)}`}
                  />
                  <Link
                    className="browse__torrent-name"
                    to={`/torrent/${t.id}`}
                  >
                    {t.name}
                  </Link>
                </td>
                <td>
                  {t.anonymous ? (
                    <span className="browse__anonymous">Anonymous</span>
                  ) : (
                    <Link to={`/user/${t.uploader_id}`}>
                      {t.uploader_name ?? "Unknown"}
                    </Link>
                  )}
                </td>
                <td>{formatBytes(t.size ?? 0)}</td>
                <td>{t.seeders ?? 0}</td>
                <td>{t.leechers ?? 0}</td>
                <td>{timeAgo(t.created_at ?? "")}</td>
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
