import { Link } from "react-router-dom";
import { useAuth } from "@/features/auth";
import { formatBytes, formatNumber, timeAgo } from "@/utils/format";
import "./home.css";

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
];

const MOCK_STATS = {
  users: 1_247,
  torrents: 8_432,
  peers: 3_891,
  traffic: 142_000_000_000_000,
};

export function HomePage() {
  const { user, isAuthenticated } = useAuth();

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
        <div className="home__stats">
          <div className="home__stat-card">
            <div className="home__stat-value">
              {formatNumber(MOCK_STATS.users)}
            </div>
            <div className="home__stat-label">Users</div>
          </div>
          <div className="home__stat-card">
            <div className="home__stat-value">
              {formatNumber(MOCK_STATS.torrents)}
            </div>
            <div className="home__stat-label">Torrents</div>
          </div>
          <div className="home__stat-card">
            <div className="home__stat-value">
              {formatNumber(MOCK_STATS.peers)}
            </div>
            <div className="home__stat-label">Peers</div>
          </div>
          <div className="home__stat-card">
            <div className="home__stat-value">
              {formatBytes(MOCK_STATS.traffic, 1)}
            </div>
            <div className="home__stat-label">Traffic</div>
          </div>
        </div>
      </section>

      <section aria-label="Latest torrents">
        <h2 className="home__section-title">Latest Torrents</h2>
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
            {MOCK_TORRENTS.map((t) => (
              <tr key={t.id}>
                <td>
                  <Link className="home__torrent-link" to={`/torrent/${t.id}`}>
                    {t.name}
                  </Link>
                  {t.free && <span className="home__free-badge">FREE</span>}
                </td>
                <td>{formatBytes(t.size)}</td>
                <td>{t.seeders}</td>
                <td>{t.leechers}</td>
                <td>{timeAgo(t.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </div>
  );
}
