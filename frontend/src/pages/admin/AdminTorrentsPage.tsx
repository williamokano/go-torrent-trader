import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { Input } from "@/components/form";
import { Pagination } from "@/components/Pagination";
import { ConfirmModal } from "@/components/modal/ConfirmModal";
import { formatBytes, timeAgo } from "@/utils/format";
import "./admin-ui.css";

interface AdminTorrent {
  id: number;
  name: string;
  size: number;
  seeders: number;
  leechers: number;
  uploader_id: number;
  uploader: string;
  banned: boolean;
  created_at: string;
}

const PER_PAGE = 25;

export function AdminTorrentsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const toast = useToast();

  const query = searchParams.get("q") ?? "";
  const page = Math.max(1, Number(searchParams.get("page")) || 1);

  const [searchInput, setSearchInput] = useState(query);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const [torrents, setTorrents] = useState<AdminTorrent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [deletingId, setDeletingId] = useState<number | null>(null);

  const fetchTorrents = useCallback(async () => {
    setLoading(true);
    const token = getAccessToken();
    const params = new URLSearchParams();
    if (query) params.set("search", query);
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
      }
    } finally {
      setLoading(false);
    }
  }, [query, page]);

  useEffect(() => {
    fetchTorrents();
  }, [fetchTorrents]);

  const handleSearchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    setSearchInput(val);
    clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      const next = new URLSearchParams(searchParams);
      if (val) {
        next.set("q", val);
      } else {
        next.delete("q");
      }
      next.delete("page");
      setSearchParams(next);
    }, 250);
  };

  const handlePageChange = (newPage: number) => {
    const next = new URLSearchParams(searchParams);
    next.set("page", String(newPage));
    setSearchParams(next);
  };

  const handleDelete = async () => {
    if (!deletingId) return;
    const token = getAccessToken();
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/torrents/${deletingId}`,
      {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      },
    );
    if (res.ok) {
      toast.success("Torrent deleted");
      fetchTorrents();
    } else {
      toast.error("Failed to delete torrent");
    }
    setDeletingId(null);
  };

  const totalPages = Math.ceil(total / PER_PAGE);

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
            placeholder="Torrent name or uploader..."
            value={searchInput}
            onChange={handleSearchChange}
          />
        </div>
      </div>

      {loading ? (
        <p>Loading...</p>
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
                    <th>Name</th>
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
                      <td className="admin-table__name">
                        <Link to={`/torrent/${torrent.id}`}>
                          {torrent.name}
                        </Link>
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
                      <td>
                        {torrent.banned ? (
                          <span className="admin-badge admin-badge--danger">
                            Banned
                          </span>
                        ) : (
                          <span className="admin-badge admin-badge--ok">
                            Active
                          </span>
                        )}
                      </td>
                      <td className="admin-muted">
                        {timeAgo(torrent.created_at)}
                      </td>
                      <td className="admin-table__actions">
                        <button
                          className="admin-btn admin-btn--danger admin-btn--sm"
                          onClick={() => setDeletingId(torrent.id)}
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
        isOpen={deletingId !== null}
        title="Delete Torrent"
        message="Are you sure you want to permanently delete this torrent and its files? This cannot be undone."
        confirmLabel="Delete"
        danger
        onConfirm={handleDelete}
        onCancel={() => setDeletingId(null)}
      />
    </div>
  );
}
