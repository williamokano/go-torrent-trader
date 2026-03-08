import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "@/api";
import { Textarea } from "@/components/form";
import { Modal } from "@/components/modal/Modal";
import { ReportModal } from "@/components/ReportModal";
import { CommentsSection } from "@/components/CommentsSection";
import { RatingWidget } from "@/components/RatingWidget";
import { useToast } from "@/components/toast";
import { useAuth } from "@/features/auth";
import { getAccessToken } from "@/features/auth/token";
import { formatBytes, formatNumber, timeAgo } from "@/utils/format";
import type { Torrent } from "@/types/torrent";
import { NfoViewer } from "@/components/NfoViewer";
import { getConfig } from "@/config";
import "./torrent-detail.css";

function healthClass(seeders: number): string {
  if (seeders > 5) return "torrent-detail__health--good";
  if (seeders >= 1) return "torrent-detail__health--warning";
  return "torrent-detail__health--dead";
}

function healthLabel(seeders: number): string {
  if (seeders > 5) return "Healthy";
  if (seeders >= 1) return "Low";
  return "Dead";
}

export function TorrentDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const toast = useToast();
  const { user } = useAuth();

  const [torrent, setTorrent] = useState<Torrent | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [downloading, setDownloading] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [deleteReason, setDeleteReason] = useState("");
  const [isDeleting, setIsDeleting] = useState(false);
  const [showReportModal, setShowReportModal] = useState(false);
  const [reseedCount, setReseedCount] = useState(0);
  const [reseedRequested, setReseedRequested] = useState(false);
  const [reseedLoading, setReseedLoading] = useState(false);
  const [peers, setPeers] = useState<
    Array<{
      user_id: number;
      uploaded: number;
      downloaded: number;
      left_bytes: number;
      seeder: boolean;
      agent: string | null;
      last_announce: string;
    }>
  >([]);

  useEffect(() => {
    let cancelled = false;

    async function fetchTorrent() {
      setLoading(true);
      setError(null);

      const torrentId = Number(id);
      if (!id || isNaN(torrentId)) {
        setError("Invalid torrent ID");
        setLoading(false);
        return;
      }

      const token = getAccessToken();
      const { data, error: apiError } = await api.GET("/api/v1/torrents/{id}", {
        params: { path: { id: torrentId } },
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });

      if (cancelled) return;

      if (apiError) {
        const msg =
          (apiError as { error?: { message?: string } }).error?.message ??
          "Failed to load torrent";
        setError(msg);
        setLoading(false);
        return;
      }

      setTorrent(data?.torrent ?? null);
      setPeers(
        ((data as Record<string, unknown>)?.peers as typeof peers) ?? [],
      );
      setLoading(false);
    }

    fetchTorrent();
    return () => {
      cancelled = true;
    };
  }, [id]);

  useEffect(() => {
    if (!torrent || !id) return;
    const seeders = torrent.seeders ?? 0;
    if (seeders > 0) return;

    const token = getAccessToken();
    if (!token) return;

    const baseUrl = getConfig().API_URL;
    fetch(`${baseUrl}/api/v1/torrents/${encodeURIComponent(id)}/reseed`, {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then((res) => res.json())
      .then((data) => {
        setReseedCount(data.count ?? 0);
      })
      .catch(() => {
        /* ignore */
      });
  }, [torrent, id]);

  async function handleRequestReseed() {
    if (!id || reseedLoading || reseedRequested) return;

    setReseedLoading(true);
    try {
      const token = getAccessToken();
      if (!token) {
        toast.error("You must be logged in");
        return;
      }

      const baseUrl = getConfig().API_URL;
      const response = await fetch(
        `${baseUrl}/api/v1/torrents/${encodeURIComponent(id)}/reseed`,
        {
          method: "POST",
          headers: {
            Authorization: `Bearer ${token}`,
          },
        },
      );

      if (response.status === 409) {
        setReseedRequested(true);
        toast.error("You have already requested a reseed for this torrent");
        return;
      }

      if (!response.ok) {
        const data = await response.json().catch(() => null);
        const message =
          data?.error?.message ?? `Reseed request failed (${response.status})`;
        throw new Error(message);
      }

      setReseedCount((prev) => prev + 1);
      setReseedRequested(true);
      toast.success("Reseed request submitted");
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
          : "Reseed request failed. Please try again.",
      );
    } finally {
      setReseedLoading(false);
    }
  }

  async function handleDownload() {
    if (!id || downloading) return;

    setDownloading(true);
    try {
      const token = getAccessToken();
      const baseUrl = getConfig().API_URL;
      const response = await fetch(
        `${baseUrl}/api/v1/torrents/${encodeURIComponent(id)}/download`,
        {
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        },
      );

      if (!response.ok) {
        throw new Error("Download failed");
      }

      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${torrent?.name ?? "torrent"}.torrent`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch {
      // Download error — could add toast here in the future
    } finally {
      setDownloading(false);
    }
  }

  const canManage =
    user && torrent && (user.isAdmin || torrent.uploader_id === user.id);

  async function handleDelete() {
    if (!id || isDeleting) return;

    setIsDeleting(true);
    try {
      const token = getAccessToken();
      if (!token) {
        toast.error("You must be logged in");
        return;
      }

      const response = await fetch(
        `${getConfig().API_URL}/api/v1/torrents/${encodeURIComponent(id)}`,
        {
          method: "DELETE",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify({ reason: deleteReason }),
        },
      );

      if (!response.ok) {
        const data = await response.json().catch(() => null);
        const message =
          data?.error?.message ?? `Delete failed (${response.status})`;
        throw new Error(message);
      }

      toast.success("Torrent deleted");
      navigate("/browse");
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Delete failed. Please try again.",
      );
    } finally {
      setIsDeleting(false);
      setShowDeleteModal(false);
    }
  }

  async function handleReport(torrentId: number, reason: string) {
    const token = getAccessToken();
    if (!token) {
      toast.error("You must be logged in to report a torrent");
      throw new Error("You must be logged in to report a torrent");
    }

    const response = await fetch(`${getConfig().API_URL}/api/v1/reports`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ torrent_id: torrentId, reason }),
    });

    if (!response.ok) {
      const data = await response.json().catch(() => null);
      const message =
        data?.error?.message ?? `Report failed (${response.status})`;
      throw new Error(message);
    }

    toast.success("Report submitted. Staff will review it.");
  }

  if (loading) {
    return <div className="torrent-detail__loading">Loading torrent...</div>;
  }

  if (error) {
    return <div className="torrent-detail__error">{error}</div>;
  }

  if (!torrent) {
    return <div className="torrent-detail__error">Torrent not found</div>;
  }

  const seeders = torrent.seeders ?? 0;
  const leechers = torrent.leechers ?? 0;

  return (
    <div className="torrent-detail">
      <div className="torrent-detail__header">
        <h1 className="torrent-detail__name">
          <span
            className={`torrent-detail__health ${healthClass(seeders)}`}
            title={healthLabel(seeders)}
          />
          {torrent.name}
        </h1>
        <span className="torrent-detail__category">
          {torrent.category_name ?? "Unknown"}
        </span>
      </div>

      <div className="torrent-detail__stats">
        <div className="torrent-detail__stat">
          <span className="torrent-detail__stat-value">
            {formatNumber(seeders)}
          </span>
          <span className="torrent-detail__stat-label">Seeders</span>
        </div>
        <div className="torrent-detail__stat">
          <span className="torrent-detail__stat-value">
            {formatNumber(leechers)}
          </span>
          <span className="torrent-detail__stat-label">Leechers</span>
        </div>
        <div className="torrent-detail__stat">
          <span className="torrent-detail__stat-value">
            {formatNumber(torrent.times_completed ?? 0)}
          </span>
          <span className="torrent-detail__stat-label">Completed</span>
        </div>
        <div className="torrent-detail__stat">
          <span className="torrent-detail__stat-value">
            {formatBytes(torrent.size ?? 0)}
          </span>
          <span className="torrent-detail__stat-label">Size</span>
        </div>
      </div>

      <div className="torrent-detail__actions">
        <button
          className="torrent-detail__download"
          onClick={handleDownload}
          disabled={downloading}
        >
          {downloading ? "Downloading..." : "Download .torrent"}
        </button>

        {canManage && (
          <>
            <Link
              to={`/torrent/${id}/edit`}
              className="torrent-detail__action-btn torrent-detail__edit-btn"
            >
              Edit
            </Link>
            <button
              className="torrent-detail__action-btn torrent-detail__delete-btn"
              onClick={() => setShowDeleteModal(true)}
            >
              Delete
            </button>
          </>
        )}

        {user && seeders === 0 && (
          <button
            className="torrent-detail__action-btn torrent-detail__reseed-btn"
            onClick={handleRequestReseed}
            disabled={reseedLoading || reseedRequested}
          >
            {reseedLoading
              ? "Requesting..."
              : reseedRequested
                ? "Reseed Requested"
                : "Request Reseed"}
          </button>
        )}

        {seeders === 0 && reseedCount > 0 && (
          <span className="torrent-detail__reseed-count">
            {reseedCount} {reseedCount === 1 ? "user" : "users"} requested
            reseed
          </span>
        )}

        {user && (
          <button
            className="torrent-detail__action-btn torrent-detail__report-btn"
            onClick={() => setShowReportModal(true)}
          >
            Report
          </button>
        )}
      </div>

      <div className="torrent-detail__info">
        <div className="torrent-detail__info-row">
          <span className="torrent-detail__info-label">Info Hash</span>
          <span className="torrent-detail__info-value torrent-detail__info-hash">
            {torrent.info_hash}
          </span>
        </div>
        <div className="torrent-detail__info-row">
          <span className="torrent-detail__info-label">Files</span>
          <span className="torrent-detail__info-value">
            {formatNumber(torrent.file_count ?? 0)}
          </span>
        </div>
        <div className="torrent-detail__info-row">
          <span className="torrent-detail__info-label">Uploaded</span>
          <span className="torrent-detail__info-value">
            {torrent.created_at ? timeAgo(torrent.created_at) : "Unknown"}
          </span>
        </div>
      </div>

      {torrent.description && (
        <div className="torrent-detail__description">
          <h2 className="torrent-detail__description-title">Description</h2>
          <div className="torrent-detail__description-body">
            {torrent.description}
          </div>
        </div>
      )}

      {(torrent as unknown as { nfo?: string }).nfo && (
        <NfoViewer content={(torrent as unknown as { nfo: string }).nfo} />
      )}

      {peers.length > 0 && (
        <div className="torrent-detail__peers">
          <h2 className="torrent-detail__peers-title">Peers</h2>
          <table className="torrent-detail__peers-table">
            <thead>
              <tr>
                <th>Type</th>
                <th>Uploaded</th>
                <th>Downloaded</th>
                <th>Left</th>
                <th>Client</th>
              </tr>
            </thead>
            <tbody>
              {peers.map((p, i) => (
                <tr key={i}>
                  <td>
                    <span
                      className={`torrent-detail__peer-type ${p.seeder ? "torrent-detail__peer-type--seed" : "torrent-detail__peer-type--leech"}`}
                    >
                      {p.seeder ? "Seed" : "Leech"}
                    </span>
                  </td>
                  <td>{formatBytes(p.uploaded)}</td>
                  <td>{formatBytes(p.downloaded)}</td>
                  <td>
                    {p.left_bytes === 0 ? "-" : formatBytes(p.left_bytes)}
                  </td>
                  <td>{p.agent || "Unknown"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <RatingWidget torrentId={id!} />

      <CommentsSection torrentId={id!} />

      <Modal
        isOpen={showDeleteModal}
        onClose={() => setShowDeleteModal(false)}
        title="Delete Torrent"
      >
        <div className="torrent-detail__delete-modal">
          <p className="torrent-detail__delete-warning">
            Are you sure you want to delete this torrent? This action cannot be
            undone.
          </p>
          <Textarea
            label="Reason for deletion"
            value={deleteReason}
            onChange={(e) => setDeleteReason(e.target.value)}
            rows={3}
            placeholder="Provide a reason..."
          />
          <div className="torrent-detail__delete-modal-actions">
            <button
              className="torrent-detail__delete-modal-cancel"
              onClick={() => setShowDeleteModal(false)}
            >
              Cancel
            </button>
            <button
              className="torrent-detail__delete-modal-confirm"
              onClick={handleDelete}
              disabled={isDeleting}
            >
              {isDeleting ? "Deleting..." : "Delete"}
            </button>
          </div>
        </div>
      </Modal>

      <ReportModal
        isOpen={showReportModal}
        onClose={() => setShowReportModal(false)}
        torrentId={torrent.id!}
        onSubmit={handleReport}
      />
    </div>
  );
}
