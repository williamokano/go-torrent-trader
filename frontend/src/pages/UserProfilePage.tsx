import { useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useAuth } from "@/features/auth";
import { formatBytes, formatRatio, formatDate } from "@/utils/format";
import "./profile.css";

interface PublicUser {
  id: number;
  username: string;
  group_id: number;
  avatar: string;
  title: string;
  info: string;
  uploaded: number;
  downloaded: number;
  ratio: number;
  donor: boolean;
  created_at: string;
}

export function UserProfilePage() {
  const { id } = useParams<{ id: string }>();
  const { user: currentUser } = useAuth();

  const [profile, setProfile] = useState<PublicUser | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

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
            <span className="profile-info__joined">
              Joined {formatDate(profile.created_at)}
            </span>
            {isOwnProfile && (
              <Link to="/settings" className="profile-info__settings-link">
                Edit Profile
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
      </div>

      {profile.info && (
        <div className="profile-bio">
          <h2 className="profile-bio__title">About</h2>
          <p className="profile-bio__content">{profile.info}</p>
        </div>
      )}
    </div>
  );
}
