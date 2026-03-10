import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { Input } from "@/components/form";
import { Pagination } from "@/components/Pagination";
import { ConfirmModal } from "@/components/modal/ConfirmModal";
import { formatBytes, timeAgo } from "@/utils/format";
import "./admin-torrents.css";

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
      <div className="admin-torrents__header">
        <h1>Torrents</h1>
        <div className="admin-torrents__search">
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
        <p className="admin-torrents__empty">No torrents found.</p>
      ) : (
        <>
          <table className="admin-torrents__table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Size</th>
                <th>S/L</th>
                <th>Uploader</th>
                <th>Status</th>
                <th>Created</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {torrents.map((torrent) => (
                <tr key={torrent.id}>
                  <td>
                    <Link to={`/torrent/${torrent.id}`}>{torrent.name}</Link>
                  </td>
                  <td>{formatBytes(torrent.size)}</td>
                  <td>
                    {torrent.seeders}/{torrent.leechers}
                  </td>
                  <td>
                    <Link to={`/admin/users/${torrent.uploader_id}`}>
                      {torrent.uploader || `User #${torrent.uploader_id}`}
                    </Link>
                  </td>
                  <td>
                    {torrent.banned && (
                      <span className="admin-torrents__badge admin-torrents__badge--banned">
                        Banned
                      </span>
                    )}
                    {!torrent.banned && (
                      <span className="admin-torrents__badge admin-torrents__badge--active">
                        Active
                      </span>
                    )}
                  </td>
                  <td>{timeAgo(torrent.created_at)}</td>
                  <td className="admin-torrents__actions">
                    <button
                      className="admin-torrents__delete-btn"
                      onClick={() => setDeletingId(torrent.id)}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          <Pagination
            currentPage={page}
            totalPages={totalPages}
            onPageChange={handlePageChange}
          />
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
