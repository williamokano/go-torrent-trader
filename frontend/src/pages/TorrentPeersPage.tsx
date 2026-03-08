import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { formatBytes, timeAgo } from "@/utils/format";
import "./torrent-peers.css";

interface Peer {
  user_id: number;
  uploaded: number;
  downloaded: number;
  left_bytes: number;
  seeder: boolean;
  agent: string | null;
  last_announce: string;
}

export function TorrentPeersPage() {
  const { id } = useParams<{ id: string }>();
  const [peers, setPeers] = useState<Peer[]>([]);
  const [torrentName, setTorrentName] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const torrentId = Number(id);
    if (!id || isNaN(torrentId)) {
      setError("Invalid torrent ID");
      setLoading(false);
      return;
    }

    async function fetchData() {
      try {
        const token = getAccessToken();
        const headers: Record<string, string> = token
          ? { Authorization: `Bearer ${token}` }
          : {};

        const res = await fetch(
          `${getConfig().API_URL}/api/v1/torrents/${torrentId}`,
          { headers },
        );

        if (!res.ok) {
          setError("Failed to load torrent");
          return;
        }

        const data = await res.json();
        setTorrentName(data?.torrent?.name ?? "");
        setPeers(data?.peers ?? []);
      } catch {
        setError("Failed to load torrent");
      } finally {
        setLoading(false);
      }
    }

    fetchData();
  }, [id]);

  if (loading) {
    return <div className="torrent-peers__loading">Loading peers...</div>;
  }

  if (error) {
    return <div className="torrent-peers__error">{error}</div>;
  }

  const seeders = peers.filter((p) => p.seeder);
  const leechers = peers.filter((p) => !p.seeder);

  return (
    <div className="torrent-peers">
      <div className="torrent-peers__header">
        <h1 className="torrent-peers__title">Peers</h1>
        <Link to={`/torrent/${id}`} className="torrent-peers__back">
          {torrentName || "Back to torrent"}
        </Link>
      </div>

      <div className="torrent-peers__summary">
        <span className="torrent-peers__count torrent-peers__count--seed">
          {seeders.length} {seeders.length === 1 ? "seeder" : "seeders"}
        </span>
        <span className="torrent-peers__count torrent-peers__count--leech">
          {leechers.length} {leechers.length === 1 ? "leecher" : "leechers"}
        </span>
      </div>

      {peers.length === 0 ? (
        <div className="torrent-peers__empty">No active peers.</div>
      ) : (
        <table className="torrent-peers__table">
          <thead>
            <tr>
              <th>Type</th>
              <th>Uploaded</th>
              <th>Downloaded</th>
              <th>Left</th>
              <th>Client</th>
              <th>Last Seen</th>
            </tr>
          </thead>
          <tbody>
            {peers.map((p, i) => (
              <tr key={i}>
                <td>
                  <span
                    className={`torrent-peers__type ${p.seeder ? "torrent-peers__type--seed" : "torrent-peers__type--leech"}`}
                  >
                    {p.seeder ? "Seed" : "Leech"}
                  </span>
                </td>
                <td>{formatBytes(p.uploaded)}</td>
                <td>{formatBytes(p.downloaded)}</td>
                <td>{p.left_bytes === 0 ? "-" : formatBytes(p.left_bytes)}</td>
                <td>{p.agent || "Unknown"}</td>
                <td>{timeAgo(p.last_announce)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
