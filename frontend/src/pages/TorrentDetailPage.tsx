import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { api } from "@/api";
import { getAccessToken } from "@/features/auth/token";
import { formatBytes, formatNumber, timeAgo } from "@/utils/format";
import type { Torrent } from "@/types/torrent";
import { getConfig } from "@/config";
import "./torrent-detail.css";

const CATEGORIES: Record<number, string> = {
  1: "Linux ISOs",
  2: "Software",
  3: "Music",
  4: "E-Books",
  5: "Other",
};

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

  const [torrent, setTorrent] = useState<Torrent | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [downloading, setDownloading] = useState(false);

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
      setLoading(false);
    }

    fetchTorrent();
    return () => {
      cancelled = true;
    };
  }, [id]);

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
          {CATEGORIES[torrent.category_id ?? 0] ?? "Unknown"}
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

      <button
        className="torrent-detail__download"
        onClick={handleDownload}
        disabled={downloading}
      >
        {downloading ? "Downloading..." : "Download .torrent"}
      </button>

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
    </div>
  );
}
