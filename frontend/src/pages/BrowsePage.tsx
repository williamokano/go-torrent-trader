import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api } from "@/api";
import type { paths } from "@/api/schema";
import { getAccessToken } from "@/features/auth/token";
import { Input } from "@/components/form";
import { Select } from "@/components/form";
import { Pagination } from "@/components/Pagination";
import { MetadataFilterControls } from "@/components/MetadataFilterControls";
import { fetchCategorySchema, type MetadataField } from "@/utils/metadata";
import { formatBytes, timeAgo } from "@/utils/format";
import type { Torrent } from "@/types/torrent";
import { buildCategoryOptions } from "@/utils/categories";
import { CategoryIcon } from "@/components/CategoryIcon";
import { UsernameDisplay } from "@/components/UsernameDisplay";
import "./browse.css";

// The typed client can't express the data-driven meta_<field> query params, so
// the list query is assembled as a plain record and cast to the operation's
// query type; openapi-fetch serializes every key it's given.
type ListTorrentsQuery = NonNullable<
  paths["/api/v1/torrents"]["get"]["parameters"]["query"]
>;

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
  const [schema, setSchema] = useState<MetadataField[]>([]);

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

  // Load the selected category's effective schema so its metadata fields can be
  // offered as filters. Resolves to [] when no category is selected.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      const fields = category ? await fetchCategorySchema(category) : [];
      if (!cancelled) setSchema(fields);
    })();
    return () => {
      cancelled = true;
    };
  }, [category]);

  // Active metadata filter params (meta_<field>...), restricted to fields the
  // current category actually defines so a stale param can't reach the backend.
  const metaQuery = useMemo(() => {
    const out: Record<string, string> = {};
    for (const field of schema) {
      const eq = searchParams.get(`meta_${field.key}`);
      if (eq) out[`meta_${field.key}`] = eq;
      if (field.type === "number") {
        const gte = searchParams.get(`meta_${field.key}__gte`);
        const lte = searchParams.get(`meta_${field.key}__lte`);
        if (gte) out[`meta_${field.key}__gte`] = gte;
        if (lte) out[`meta_${field.key}__lte`] = lte;
      }
    }
    return out;
  }, [schema, searchParams]);

  useEffect(() => {
    let cancelled = false;

    async function fetchTorrents() {
      setLoading(true);
      setError(null);

      const listQuery: Record<string, string | number> = {
        sort: sortBy,
        order: sortDir,
        page,
        per_page: PER_PAGE,
        ...metaQuery,
      };
      if (query) listQuery.search = query;
      if (category) listQuery.cat = Number(category);

      const token = getAccessToken();
      const { data, error: apiError } = await api.GET("/api/v1/torrents", {
        params: { query: listQuery as unknown as ListTorrentsQuery },
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
  }, [query, category, sortBy, sortDir, page, metaQuery]);

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

  // Changing category drops the previous category's metadata filters, which
  // don't apply to the new schema (and would be rejected by the backend).
  const handleCategoryChange = useCallback(
    (value: string) => {
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev);
        for (const key of [...next.keys()]) {
          if (key.startsWith("meta_")) next.delete(key);
        }
        if (value) {
          next.set("cat", value);
        } else {
          next.delete("cat");
        }
        next.delete("page");
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
              onChange={(e) => handleCategoryChange(e.target.value)}
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
        {category && schema.length > 0 && (
          <div className="browse__meta-filters">
            <MetadataFilterControls
              schema={schema}
              get={(paramKey) => searchParams.get(paramKey) ?? ""}
              set={setParam}
            />
          </div>
        )}
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
                    <UsernameDisplay
                      userId={t.uploader_id!}
                      username={t.uploader_name ?? "Unknown"}
                      warned={t.uploader_warned}
                      noLink={!t.uploader_name}
                    />
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
