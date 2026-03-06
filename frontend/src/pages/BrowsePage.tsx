import { useCallback, useMemo } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Input } from "@/components/form";
import { Select } from "@/components/form";
import { Pagination } from "@/components/Pagination";
import { formatBytes, timeAgo } from "@/utils/format";
import "./browse.css";

interface TorrentListItem {
  id: number;
  name: string;
  category_id: number;
  size: number;
  seeders: number;
  leechers: number;
  times_completed: number;
  created_at: string;
  free: boolean;
  uploader: string;
}

const CATEGORIES = [
  { value: "", label: "All Categories" },
  { value: "1", label: "Linux ISOs" },
  { value: "2", label: "Software" },
  { value: "3", label: "Music" },
  { value: "4", label: "E-Books" },
  { value: "5", label: "Other" },
];

const SORT_OPTIONS = [
  { value: "created_at", label: "Date" },
  { value: "name", label: "Name" },
  { value: "size", label: "Size" },
  { value: "seeders", label: "Seeders" },
  { value: "leechers", label: "Leechers" },
];

const MOCK_TORRENTS: TorrentListItem[] = [
  {
    id: 1,
    name: "Ubuntu 24.04 LTS Desktop",
    category_id: 1,
    size: 4_800_000_000,
    seeders: 42,
    leechers: 5,
    times_completed: 318,
    created_at: "2026-03-05T14:30:00Z",
    free: true,
    uploader: "admin",
  },
  {
    id: 2,
    name: "Arch Linux 2026.03.01",
    category_id: 1,
    size: 850_000_000,
    seeders: 28,
    leechers: 3,
    times_completed: 156,
    created_at: "2026-03-04T10:15:00Z",
    free: false,
    uploader: "linuxfan",
  },
  {
    id: 3,
    name: "Blender 4.2 Source Code",
    category_id: 2,
    size: 320_000_000,
    seeders: 12,
    leechers: 1,
    times_completed: 87,
    created_at: "2026-03-03T18:45:00Z",
    free: false,
    uploader: "opensrc",
  },
  {
    id: 4,
    name: "Creative Commons Music Pack Vol. 12",
    category_id: 3,
    size: 1_200_000_000,
    seeders: 8,
    leechers: 2,
    times_completed: 64,
    created_at: "2026-03-02T09:00:00Z",
    free: true,
    uploader: "musicbot",
  },
  {
    id: 5,
    name: "Fedora 41 Server",
    category_id: 1,
    size: 2_100_000_000,
    seeders: 0,
    leechers: 4,
    times_completed: 201,
    created_at: "2026-03-01T22:30:00Z",
    free: false,
    uploader: "fedorauser",
  },
  {
    id: 6,
    name: "Debian 13 Netinst",
    category_id: 1,
    size: 400_000_000,
    seeders: 3,
    leechers: 0,
    times_completed: 112,
    created_at: "2026-02-28T16:00:00Z",
    free: false,
    uploader: "debfan",
  },
  {
    id: 7,
    name: "GIMP 3.0 Portable",
    category_id: 2,
    size: 180_000_000,
    seeders: 15,
    leechers: 2,
    times_completed: 95,
    created_at: "2026-02-27T11:20:00Z",
    free: false,
    uploader: "opensrc",
  },
  {
    id: 8,
    name: "Public Domain E-Book Collection 2026",
    category_id: 4,
    size: 2_500_000_000,
    seeders: 6,
    leechers: 1,
    times_completed: 43,
    created_at: "2026-02-26T08:00:00Z",
    free: true,
    uploader: "bookworm",
  },
  {
    id: 9,
    name: "openSUSE Tumbleweed DVD",
    category_id: 1,
    size: 4_200_000_000,
    seeders: 1,
    leechers: 6,
    times_completed: 78,
    created_at: "2026-02-25T20:00:00Z",
    free: false,
    uploader: "susefan",
  },
  {
    id: 10,
    name: "LibreOffice 25.2 Source",
    category_id: 2,
    size: 750_000_000,
    seeders: 0,
    leechers: 0,
    times_completed: 33,
    created_at: "2026-02-24T14:00:00Z",
    free: false,
    uploader: "officefan",
  },
  {
    id: 11,
    name: "CC Licensed Ambient Sounds",
    category_id: 3,
    size: 600_000_000,
    seeders: 22,
    leechers: 1,
    times_completed: 55,
    created_at: "2026-02-23T12:00:00Z",
    free: false,
    uploader: "musicbot",
  },
  {
    id: 12,
    name: "Kali Linux 2026.1",
    category_id: 1,
    size: 3_800_000_000,
    seeders: 35,
    leechers: 8,
    times_completed: 267,
    created_at: "2026-02-22T19:00:00Z",
    free: false,
    uploader: "secadmin",
  },
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

  const filtered = useMemo(() => {
    let result = MOCK_TORRENTS;

    if (query) {
      const q = query.toLowerCase();
      result = result.filter((t) => t.name.toLowerCase().includes(q));
    }

    if (category) {
      const catId = Number(category);
      result = result.filter((t) => t.category_id === catId);
    }

    result = [...result].sort((a, b) => {
      const aVal = a[sortBy];
      const bVal = b[sortBy];
      if (typeof aVal === "string" && typeof bVal === "string") {
        return sortDir === "asc"
          ? aVal.localeCompare(bVal)
          : bVal.localeCompare(aVal);
      }
      return sortDir === "asc"
        ? (aVal as number) - (bVal as number)
        : (bVal as number) - (aVal as number);
    });

    return result;
  }, [query, category, sortBy, sortDir]);

  const totalPages = Math.max(1, Math.ceil(filtered.length / PER_PAGE));
  const currentPage = Math.min(page, totalPages);
  const paginated = filtered.slice(
    (currentPage - 1) * PER_PAGE,
    currentPage * PER_PAGE,
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
              value={query}
              onChange={(e) => setParam("q", e.target.value)}
            />
          </div>
          <div className="browse__filter">
            <Select
              label="Category"
              options={CATEGORIES}
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

      {paginated.length === 0 ? (
        <div className="browse__empty">No torrents found.</div>
      ) : (
        <table className="browse__table">
          <thead>
            <tr>
              <th onClick={() => handleSort("name")}>
                Name{sortIndicator("name")}
              </th>
              <th>Category</th>
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
            {paginated.map((t) => (
              <tr key={t.id}>
                <td>
                  <span
                    className={`browse__health ${healthClass(t.seeders)}`}
                  />
                  <Link
                    className="browse__torrent-name"
                    to={`/torrent/${t.id}`}
                  >
                    {t.name}
                  </Link>
                  {t.free && <span className="browse__free-badge">FREE</span>}
                </td>
                <td>
                  {CATEGORIES.find((c) => c.value === String(t.category_id))
                    ?.label ?? "Unknown"}
                </td>
                <td>{formatBytes(t.size)}</td>
                <td>{t.seeders}</td>
                <td>{t.leechers}</td>
                <td>{timeAgo(t.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <Pagination
        currentPage={currentPage}
        totalPages={totalPages}
        onPageChange={(p) => setParam("page", String(p))}
      />
    </div>
  );
}
