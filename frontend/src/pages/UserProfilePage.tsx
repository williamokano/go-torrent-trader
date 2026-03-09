import { useEffect, useState, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useAuth } from "@/features/auth";
import { formatBytes, formatRatio, formatDate, timeAgo } from "@/utils/format";
import "./profile.css";

interface PublicUser {
  id: number;
  username: string;
  group_id: number;
  group_name: string;
  avatar: string;
  title: string;
  info: string;
  uploaded: number;
  downloaded: number;
  ratio: number;
  donor: boolean;
  created_at: string;
  invited_by_id?: number;
  invited_by_name?: string;
  seeding_count: number;
  leeching_count: number;
  recent_uploads?: Array<{
    id: number;
    name: string;
    created_at: string;
  }>;
}

interface TorrentUpload {
  id: number;
  name: string;
  size: number;
  seeders: number;
  leechers: number;
  times_completed: number;
  category_name: string;
  created_at: string;
  anonymous?: boolean;
}

interface ActivityItem {
  torrent_id: number;
  torrent_name: string;
  uploaded: number;
  downloaded: number;
  ratio: number;
  seeder: boolean;
  ip?: string;
  port?: number;
  last_announce?: string;
  completed_at?: string;
}

type ActivityTab = "uploads" | "seeding" | "leeching" | "history";

export function UserProfilePage() {
  const { id } = useParams<{ id: string }>();
  const { user: currentUser } = useAuth();

  const [profile, setProfile] = useState<PublicUser | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [activeTab, setActiveTab] = useState<ActivityTab>("uploads");
  const [uploads, setUploads] = useState<TorrentUpload[]>([]);
  const [uploadsTotal, setUploadsTotal] = useState(0);
  const [uploadsPage, setUploadsPage] = useState(1);
  const [activity, setActivity] = useState<ActivityItem[]>([]);
  const [activityTotal, setActivityTotal] = useState(0);
  const [activityPage, setActivityPage] = useState(1);
  const [tabLoading, setTabLoading] = useState(false);
  const [tabError, setTabError] = useState<string | null>(null);

  const perPage = 25;

  // Reset tab and stale data when navigating to a different profile
  useEffect(() => {
    setActiveTab("uploads");
    setUploads([]);
    setUploadsTotal(0);
    setUploadsPage(1);
    setActivity([]);
    setActivityTotal(0);
    setActivityPage(1);
  }, [id]);

  useEffect(() => {
    const numericId = Number(id);
    if (!id || isNaN(numericId)) {
      setError("Invalid user ID");
      setLoading(false);
      return;
    }

    async function fetchProfile() {
      try {
        const token = getAccessToken();
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/users/${numericId}`,
          {
            headers: token ? { Authorization: `Bearer ${token}` } : {},
          },
        );

        const body = await res.json();

        if (!res.ok) {
          setError(body?.error?.message ?? "Failed to load profile");
          return;
        }

        if (body?.user) {
          setProfile(body.user as PublicUser);
        } else {
          setError("User not found");
        }
      } catch {
        setError("Failed to load profile");
      } finally {
        setLoading(false);
      }
    }

    fetchProfile();
  }, [id]);

  const canViewPrivateActivity =
    profile &&
    currentUser &&
    (currentUser.id === profile.id || currentUser.isStaff);

  const fetchUploads = useCallback(
    async (page: number) => {
      if (!id) return;
      setTabLoading(true);
      setTabError(null);
      try {
        const token = getAccessToken();
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/users/${id}/torrents?page=${page}&per_page=${perPage}`,
          { headers: token ? { Authorization: `Bearer ${token}` } : {} },
        );
        if (res.ok) {
          const body = await res.json();
          setUploads(body.torrents ?? []);
          setUploadsTotal(body.total ?? 0);
        } else {
          setTabError("Failed to load uploads");
        }
      } catch {
        setTabError("Failed to load uploads");
      } finally {
        setTabLoading(false);
      }
    },
    [id],
  );

  const fetchActivity = useCallback(
    async (tab: "seeding" | "leeching" | "history", page: number) => {
      if (!id) return;
      setTabLoading(true);
      setTabError(null);
      try {
        const token = getAccessToken();
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/users/${id}/activity?tab=${tab}&page=${page}&per_page=${perPage}`,
          { headers: token ? { Authorization: `Bearer ${token}` } : {} },
        );
        if (res.ok) {
          const body = await res.json();
          setActivity(body.activity ?? []);
          setActivityTotal(body.total ?? 0);
        } else {
          setTabError("Failed to load activity");
        }
      } catch {
        setTabError("Failed to load activity");
      } finally {
        setTabLoading(false);
      }
    },
    [id],
  );

  // Fetch data when tab or page changes
  useEffect(() => {
    if (!profile) return;
    if (activeTab === "uploads") {
      fetchUploads(uploadsPage);
    } else if (canViewPrivateActivity) {
      fetchActivity(activeTab, activityPage);
    }
  }, [
    profile,
    activeTab,
    uploadsPage,
    activityPage,
    canViewPrivateActivity,
    fetchUploads,
    fetchActivity,
  ]);

  function handleTabChange(tab: ActivityTab) {
    setActiveTab(tab);
    if (tab === "uploads") {
      setUploadsPage(1);
    } else {
      setActivityPage(1);
    }
  }

  if (loading) {
    return (
      <div className="profile-page">
        <p className="profile-page__loading">Loading profile...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="profile-page">
        <p className="profile-page__error">{error}</p>
      </div>
    );
  }

  if (!profile) {
    return (
      <div className="profile-page">
        <p className="profile-page__error">User not found</p>
      </div>
    );
  }

  const initials = profile.username.charAt(0).toUpperCase();
  const isOwnProfile = currentUser?.id === profile.id;

  const uploadsTotalPages = Math.ceil(uploadsTotal / perPage);
  const activityTotalPages = Math.ceil(activityTotal / perPage);

  return (
    <div className="profile-page">
      <div className="profile-header">
        {profile.avatar ? (
          <img
            className="profile-avatar"
            src={profile.avatar}
            alt={`${profile.username}'s avatar`}
          />
        ) : (
          <div className="profile-avatar--initials">{initials}</div>
        )}
        <div className="profile-info">
          <h1 className="profile-info__username">{profile.username}</h1>
          {profile.title && (
            <p className="profile-info__title">{profile.title}</p>
          )}
          <div className="profile-info__meta">
            {profile.donor && <span className="profile-badge">Donor</span>}
            {profile.group_name && (
              <span className="profile-badge profile-badge--group">
                {profile.group_name}
              </span>
            )}
            <span className="profile-info__joined">
              Joined {formatDate(profile.created_at)}
            </span>
            {profile.invited_by_name && (
              <span className="profile-info__invited-by">
                Invited by{" "}
                <Link to={`/user/${profile.invited_by_id}`}>
                  {profile.invited_by_name}
                </Link>
              </span>
            )}
            {isOwnProfile && (
              <Link to="/settings" className="profile-info__settings-link">
                Edit Profile
              </Link>
            )}
            {!isOwnProfile && currentUser && (
              <Link
                to={`/messages?tab=compose&to=${encodeURIComponent(profile.username)}&to_id=${profile.id}`}
                className="profile-info__settings-link"
              >
                Send Message
              </Link>
            )}
            {!isOwnProfile && currentUser?.isStaff && (
              <Link
                to={`/admin/users?q=${encodeURIComponent(profile.username)}`}
                className="profile-info__settings-link"
              >
                Manage User
              </Link>
            )}
          </div>
        </div>
      </div>

      <div className="profile-stats">
        <div className="profile-stat">
          <div className="profile-stat__label">Uploaded</div>
          <div className="profile-stat__value">
            {formatBytes(profile.uploaded)}
          </div>
        </div>
        <div className="profile-stat">
          <div className="profile-stat__label">Downloaded</div>
          <div className="profile-stat__value">
            {formatBytes(profile.downloaded)}
          </div>
        </div>
        <div className="profile-stat">
          <div className="profile-stat__label">Ratio</div>
          <div
            className={`profile-stat__value ${
              profile.ratio >= 1
                ? "profile-stat__value--good"
                : "profile-stat__value--bad"
            }`}
          >
            {formatRatio(profile.ratio)}
          </div>
        </div>
        <div className="profile-stat">
          <div className="profile-stat__label">Seeding</div>
          <div className="profile-stat__value profile-stat__value--good">
            {profile.seeding_count}
          </div>
        </div>
        <div className="profile-stat">
          <div className="profile-stat__label">Leeching</div>
          <div className="profile-stat__value">{profile.leeching_count}</div>
        </div>
      </div>

      {profile.info && (
        <div className="profile-bio">
          <h2 className="profile-bio__title">About</h2>
          <p className="profile-bio__content">{profile.info}</p>
        </div>
      )}

      {/* Activity Tabs */}
      <div className="profile-activity">
        <div className="profile-activity__tabs">
          <button
            className={`profile-activity__tab ${activeTab === "uploads" ? "profile-activity__tab--active" : ""}`}
            onClick={() => handleTabChange("uploads")}
          >
            Uploads
          </button>
          {canViewPrivateActivity && (
            <>
              <button
                className={`profile-activity__tab ${activeTab === "seeding" ? "profile-activity__tab--active" : ""}`}
                onClick={() => handleTabChange("seeding")}
              >
                Seeding
              </button>
              <button
                className={`profile-activity__tab ${activeTab === "leeching" ? "profile-activity__tab--active" : ""}`}
                onClick={() => handleTabChange("leeching")}
              >
                Leeching
              </button>
              <button
                className={`profile-activity__tab ${activeTab === "history" ? "profile-activity__tab--active" : ""}`}
                onClick={() => handleTabChange("history")}
              >
                History
              </button>
            </>
          )}
        </div>

        <div className="profile-activity__content">
          {tabError ? (
            <p className="profile-activity__error">{tabError}</p>
          ) : tabLoading ? (
            <p className="profile-activity__loading">Loading...</p>
          ) : activeTab === "uploads" ? (
            <UploadsTable
              uploads={uploads}
              page={uploadsPage}
              totalPages={uploadsTotalPages}
              onPageChange={setUploadsPage}
            />
          ) : (
            <ActivityTable
              items={activity}
              showPort={activeTab === "seeding" || activeTab === "leeching"}
              showCompletedAt={activeTab === "history"}
              page={activityPage}
              totalPages={activityTotalPages}
              onPageChange={setActivityPage}
            />
          )}
        </div>
      </div>
    </div>
  );
}

function UploadsTable({
  uploads,
  page,
  totalPages,
  onPageChange,
}: {
  uploads: TorrentUpload[];
  page: number;
  totalPages: number;
  onPageChange: (p: number) => void;
}) {
  if (uploads.length === 0) {
    return <p className="profile-activity__empty">No uploads found.</p>;
  }

  return (
    <>
      <table className="profile-activity__table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Size</th>
            <th>SE</th>
            <th>LE</th>
            <th>Completed</th>
            <th>Added</th>
          </tr>
        </thead>
        <tbody>
          {uploads.map((t) => (
            <tr key={t.id}>
              <td>
                <Link to={`/torrent/${t.id}`}>{t.name}</Link>
              </td>
              <td>{formatBytes(t.size)}</td>
              <td className="profile-activity__seeders">{t.seeders}</td>
              <td className="profile-activity__leechers">{t.leechers}</td>
              <td>{t.times_completed}</td>
              <td>{timeAgo(t.created_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <Pagination page={page} totalPages={totalPages} onChange={onPageChange} />
    </>
  );
}

function ActivityTable({
  items,
  showPort,
  showCompletedAt,
  page,
  totalPages,
  onPageChange,
}: {
  items: ActivityItem[];
  showPort: boolean;
  showCompletedAt: boolean;
  page: number;
  totalPages: number;
  onPageChange: (p: number) => void;
}) {
  if (items.length === 0) {
    return <p className="profile-activity__empty">No activity found.</p>;
  }

  return (
    <>
      <table className="profile-activity__table">
        <thead>
          <tr>
            <th>Torrent</th>
            <th>Uploaded</th>
            <th>Downloaded</th>
            <th>Ratio</th>
            {showPort && <th>IP</th>}
            {showPort && <th>Port</th>}
            {showCompletedAt && <th>Completed</th>}
            <th>Last Announce</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.torrent_id}>
              <td>
                <Link to={`/torrent/${item.torrent_id}`}>
                  {item.torrent_name}
                </Link>
              </td>
              <td>{formatBytes(item.uploaded)}</td>
              <td>{formatBytes(item.downloaded)}</td>
              <td>{formatRatio(item.ratio)}</td>
              {showPort && <td>{item.ip ?? "-"}</td>}
              {showPort && <td>{item.port ?? "-"}</td>}
              {showCompletedAt && (
                <td>{item.completed_at ? timeAgo(item.completed_at) : "-"}</td>
              )}
              <td>{item.last_announce ? timeAgo(item.last_announce) : "-"}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <Pagination page={page} totalPages={totalPages} onChange={onPageChange} />
    </>
  );
}

function Pagination({
  page,
  totalPages,
  onChange,
}: {
  page: number;
  totalPages: number;
  onChange: (p: number) => void;
}) {
  if (totalPages <= 1) return null;

  return (
    <div className="profile-activity__pagination">
      <button
        disabled={page <= 1}
        onClick={() => onChange(page - 1)}
        className="profile-activity__page-btn"
      >
        Previous
      </button>
      <span className="profile-activity__page-info">
        Page {page} of {totalPages}
      </span>
      <button
        disabled={page >= totalPages}
        onClick={() => onChange(page + 1)}
        className="profile-activity__page-btn"
      >
        Next
      </button>
    </div>
  );
}
