import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/api";
import { getAccessToken } from "@/features/auth/token";
import { useAuth } from "@/features/auth";
import { formatBytes, formatNumber, timeAgo } from "@/utils/format";
import { Shoutbox } from "@/components/Shoutbox";
import type { Torrent } from "@/types/torrent";
import "./home.css";

interface SiteStats {
  users: number;
  torrents: number;
  peers: number;
}

export function HomePage() {
  const { user, isAuthenticated } = useAuth();

  const [stats, setStats] = useState<SiteStats | null>(null);
  const [statsLoading, setStatsLoading] = useState(true);
  const [latestTorrents, setLatestTorrents] = useState<Torrent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Fetch site stats (public endpoint)
  useEffect(() => {
    let cancelled = false;

    async function fetchStats() {
      setStatsLoading(true);
      const { data, error: apiError } = await api.GET("/api/v1/stats");

      if (cancelled) return;

      if (apiError) {
        // Stats are non-critical; silently fall back to null
        setStatsLoading(false);
        return;
      }

      setStats((data?.stats as SiteStats) ?? null);
      setStatsLoading(false);
    }

    fetchStats();
    return () => {
      cancelled = true;
    };
  }, []);

  // Fetch latest torrents
  useEffect(() => {
    let cancelled = false;

    async function fetchLatest() {
      setLoading(true);
      setError(null);

      const token = getAccessToken();
      const { data, error: apiError } = await api.GET("/api/v1/torrents", {
        params: {
          query: {
            per_page: 5,
            sort: "created_at",
            order: "desc",
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

      setLatestTorrents(data?.torrents ?? []);
      setLoading(false);
    }

    fetchLatest();
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="home">
      <section className="home__welcome">
        <h1 className="home__title">Welcome to TorrentTrader</h1>
        {isAuthenticated && user ? (
          <p className="home__subtitle">
            Welcome back,{" "}
            <span className="home__user-greeting">{user.username}</span>
          </p>
        ) : (
          <p className="home__subtitle">
            Your private BitTorrent tracker community.
          </p>
        )}
      </section>

      <section aria-label="Site statistics">
        {statsLoading ? (
          <div className="home__loading">Loading stats...</div>
        ) : stats ? (
          <div className="home__stats">
            <div className="home__stat-card">
              <div className="home__stat-value">
                {formatNumber(stats.users)}
              </div>
              <div className="home__stat-label">Users</div>
            </div>
            <div className="home__stat-card">
              <div className="home__stat-value">
                {formatNumber(stats.torrents)}
              </div>
              <div className="home__stat-label">Torrents</div>
            </div>
            <div className="home__stat-card">
              <div className="home__stat-value">
                {formatNumber(stats.peers)}
              </div>
              <div className="home__stat-label">Peers</div>
            </div>
          </div>
        ) : null}
      </section>

      <section aria-label="Latest torrents">
        <h2 className="home__section-title">Latest Torrents</h2>
        {loading ? (
          <div className="home__loading">Loading...</div>
        ) : error ? (
          <div className="home__error">{error}</div>
        ) : latestTorrents.length === 0 ? (
          <p className="home__empty">No torrents yet.</p>
        ) : (
          <table className="home__latest-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Size</th>
                <th>S</th>
                <th>L</th>
                <th>Uploaded</th>
              </tr>
            </thead>
            <tbody>
              {latestTorrents.map((t) => (
                <tr key={t.id}>
                  <td>
                    <Link
                      className="home__torrent-link"
                      to={`/torrent/${t.id}`}
                    >
                      {t.name}
                    </Link>
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
      </section>

      {isAuthenticated && (
        <section aria-label="Shoutbox">
          <Shoutbox />
        </section>
      )}
    </div>
  );
}
