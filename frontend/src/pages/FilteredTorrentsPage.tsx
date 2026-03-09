import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { Pagination } from "@/components/Pagination";
import { CategoryIcon } from "@/components/CategoryIcon";
import { formatBytes, timeAgo } from "@/utils/format";
import type { Torrent } from "@/types/torrent";
import "./browse.css";

const PER_PAGE = 25;

function healthClass(seeders: number): string {
  if (seeders > 5) return "browse__health--good";
  if (seeders >= 1) return "browse__health--warning";
  return "browse__health--dead";
}

interface FilteredTorrentsPageProps {
  title: string;
  extraParams: Record<string, string>;
  emptyMessage: string;
}

export function FilteredTorrentsPage({
  title,
  extraParams,
  emptyMessage,
}: FilteredTorrentsPageProps) {
  const [searchParams, setSearchParams] = useSearchParams();
  const page = Math.max(1, Number(searchParams.get("page")) || 1);

  const [torrents, setTorrents] = useState<Torrent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function fetchTorrents() {
      setLoading(true);
      setError(null);

      try {
        const token = getAccessToken();
        const params = new URLSearchParams({
          page: String(page),
          per_page: String(PER_PAGE),
          ...extraParams,
        });

        const res = await fetch(
          `${getConfig().API_URL}/api/v1/torrents?${params.toString()}`,
          {
            headers: token ? { Authorization: `Bearer ${token}` } : {},
          },
        );

        if (cancelled) return;

        if (!res.ok) {
          setError("Failed to load torrents");
          setLoading(false);
          return;
        }

        const data = await res.json();
        setTorrents(data?.torrents ?? []);
        setTotal(data?.total ?? 0);
      } catch {
        if (!cancelled) setError("Failed to load torrents");
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    fetchTorrents();
    return () => {
      cancelled = true;
    };
  }, [page, extraParams]);

  const totalPages = Math.max(1, Math.ceil(total / PER_PAGE));

  return (
    <div className="browse">
      <div className="browse__header">
        <h1 className="browse__title">{title}</h1>
      </div>

      {loading ? (
        <div className="browse__loading">Loading torrents...</div>
      ) : error ? (
        <div className="browse__error">{error}</div>
      ) : torrents.length === 0 ? (
        <div className="browse__empty">{emptyMessage}</div>
      ) : (
        <table className="browse__table">
          <thead>
            <tr>
              <th title="Category">Cat.</th>
              <th>Name</th>
              <th>Uploader</th>
              <th>Size</th>
              <th>S</th>
              <th>L</th>
              <th>Uploaded</th>
            </tr>
          </thead>
          <tbody>
            {torrents.map((t) => (
              <tr key={t.id}>
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
                  <CategoryIcon
                    name={t.category_name ?? "?"}
                    imageUrl={t.category_image_url}
                  />
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
          onPageChange={(p) =>
            setSearchParams((prev) => {
              const next = new URLSearchParams(prev);
              next.set("page", String(p));
              return next;
            })
          }
        />
      )}
    </div>
  );
}
