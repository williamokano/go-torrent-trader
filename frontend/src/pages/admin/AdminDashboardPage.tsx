import { useCallback, useEffect, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { timeAgo } from "@/utils/format";
import "./admin-dashboard.css";

interface DashboardData {
  users: { total: number; today: number; week: number };
  torrents: { total: number; today: number };
  peers: { total: number; seeders: number; leechers: number };
  pending_reports: number;
  active_warnings: number;
  active_mutes: number;
  recent_activity: ActivityEntry[];
}

interface ActivityEntry {
  id: number;
  event_type: string;
  actor_id: number | null;
  message: string;
  created_at: string;
}

export function AdminDashboardPage() {
  const [data, setData] = useState<DashboardData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchDashboard = useCallback(async () => {
    setLoading(true);
    setError(null);
    const token = getAccessToken();
    try {
      const res = await fetch(`${getConfig().API_URL}/api/v1/admin/dashboard`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const json = await res.json();
        setData(json);
      } else {
        setError("Failed to load dashboard data");
      }
    } catch {
      setError("Failed to load dashboard data");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchDashboard();
  }, [fetchDashboard]);

  if (loading) {
    return <div className="admin-dashboard__loading">Loading dashboard...</div>;
  }

  if (error || !data) {
    return <div className="admin-dashboard__empty">{error ?? "No data"}</div>;
  }

  return (
    <div>
      <h1>Dashboard</h1>

      <div className="admin-dashboard__stats-grid">
        <div className="admin-dashboard__card">
          <span className="admin-dashboard__card-label">Users</span>
          <span className="admin-dashboard__card-value">
            {data.users.total.toLocaleString()}
          </span>
          <span className="admin-dashboard__card-sub">
            <span>{data.users.today}</span> today &middot;{" "}
            <span>{data.users.week}</span> this week
          </span>
        </div>

        <div className="admin-dashboard__card">
          <span className="admin-dashboard__card-label">Torrents</span>
          <span className="admin-dashboard__card-value">
            {data.torrents.total.toLocaleString()}
          </span>
          <span className="admin-dashboard__card-sub">
            <span>{data.torrents.today}</span> today
          </span>
        </div>

        <div className="admin-dashboard__card">
          <span className="admin-dashboard__card-label">Peers</span>
          <span className="admin-dashboard__card-value">
            {data.peers.total.toLocaleString()}
          </span>
          <span className="admin-dashboard__card-sub">
            <span>{data.peers.seeders}</span> seeders &middot;{" "}
            <span>{data.peers.leechers}</span> leechers
          </span>
        </div>

        <div
          className={`admin-dashboard__card${data.pending_reports > 0 ? " admin-dashboard__card--warning" : ""}`}
        >
          <span className="admin-dashboard__card-label">Pending Reports</span>
          <span
            className={`admin-dashboard__card-value${data.pending_reports > 0 ? " admin-dashboard__card-value--warning" : ""}`}
          >
            {data.pending_reports}
          </span>
        </div>

        <div className="admin-dashboard__card">
          <span className="admin-dashboard__card-label">Active Warnings</span>
          <span className="admin-dashboard__card-value">
            {data.active_warnings}
          </span>
        </div>

        <div
          className={`admin-dashboard__card${data.active_mutes > 0 ? " admin-dashboard__card--danger" : ""}`}
        >
          <span className="admin-dashboard__card-label">Active Mutes</span>
          <span
            className={`admin-dashboard__card-value${data.active_mutes > 0 ? " admin-dashboard__card-value--danger" : ""}`}
          >
            {data.active_mutes}
          </span>
        </div>
      </div>

      <h2 className="admin-dashboard__section-title">Recent Activity</h2>

      {data.recent_activity.length === 0 ? (
        <p className="admin-dashboard__empty">No recent activity.</p>
      ) : (
        <table className="admin-dashboard__activity-table">
          <thead>
            <tr>
              <th>Event</th>
              <th>Message</th>
              <th>Actor</th>
              <th>Time</th>
            </tr>
          </thead>
          <tbody>
            {data.recent_activity.map((entry) => (
              <tr key={entry.id}>
                <td>
                  <span className="admin-dashboard__event-type">
                    {entry.event_type}
                  </span>
                </td>
                <td>{entry.message}</td>
                <td>{entry.actor_id ? `#${entry.actor_id}` : "System"}</td>
                <td>{timeAgo(entry.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
