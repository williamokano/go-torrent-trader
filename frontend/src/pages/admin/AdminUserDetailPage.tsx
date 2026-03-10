import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams, useNavigate } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { formatBytes, timeAgo } from "@/utils/format";
import { WarningBadge } from "@/components/WarningBadge";
import { ConfirmModal } from "@/components/modal/ConfirmModal";
import { Textarea } from "@/components/form";
import "./admin-user-detail.css";

interface ModNote {
  id: number;
  user_id: number;
  author_id: number;
  author_username: string;
  note: string;
  created_at: string;
}

interface TorrentSummary {
  id: number;
  name: string;
  size: number;
  created_at: string;
}

interface UserDetail {
  id: number;
  username: string;
  email: string;
  group_id: number;
  group_name: string;
  avatar: string | null;
  title: string | null;
  info: string | null;
  uploaded: number;
  downloaded: number;
  enabled: boolean;
  warned: boolean;
  donor: boolean;
  parked: boolean;
  invites: number;
  created_at: string;
  last_access: string | null;
  ratio: number;
  recent_uploads: TorrentSummary[];
  warnings_count: number;
  mod_notes: ModNote[];
}

export function AdminUserDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const toast = useToast();
  const toastRef = useRef(toast);
  toastRef.current = toast;

  const [user, setUser] = useState<UserDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [newNote, setNewNote] = useState("");
  const [addingNote, setAddingNote] = useState(false);
  const [deletingNoteId, setDeletingNoteId] = useState<number | null>(null);

  const fetchUser = useCallback(async () => {
    setLoading(true);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/users/${id}`,
        {
          headers: { Authorization: `Bearer ${token}` },
        },
      );
      if (res.ok) {
        const data = await res.json();
        setUser(data.user);
      } else if (res.status === 404) {
        navigate("/admin/users");
        toastRef.current.error("User not found");
      }
    } finally {
      setLoading(false);
    }
  }, [id, navigate]);

  useEffect(() => {
    fetchUser();
  }, [fetchUser]);

  const handleAddNote = async () => {
    if (!newNote.trim()) return;
    setAddingNote(true);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/users/${id}/notes`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify({ note: newNote }),
        },
      );
      if (res.ok) {
        toast.success("Note added");
        setNewNote("");
        fetchUser();
      } else {
        toast.error("Failed to add note");
      }
    } finally {
      setAddingNote(false);
    }
  };

  const handleDeleteNote = async () => {
    if (!deletingNoteId) return;
    const token = getAccessToken();
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/notes/${deletingNoteId}`,
      {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      },
    );
    if (res.ok) {
      toast.success("Note deleted");
      fetchUser();
    } else {
      toast.error("Failed to delete note");
    }
    setDeletingNoteId(null);
  };

  const formatRatio = (ratio: number) => {
    if (ratio === -1) return "Inf";
    if (ratio === 0) return "0.00";
    return ratio.toFixed(2);
  };

  if (loading) return <p>Loading...</p>;
  if (!user) return <p>User not found.</p>;

  return (
    <div className="admin-user-detail">
      <div className="admin-user-detail__header">
        <Link to="/admin/users" className="admin-user-detail__back">
          &larr; Back to Users
        </Link>
        <h1>
          {user.username}
          <WarningBadge warned={user.warned} />
        </h1>
      </div>

      {/* Profile Info */}
      <div className="admin-user-detail__grid">
        <div className="admin-user-detail__card">
          <h2>Profile</h2>
          <dl className="admin-user-detail__dl">
            <dt>Email</dt>
            <dd>{user.email}</dd>
            <dt>Group</dt>
            <dd>{user.group_name}</dd>
            <dt>Title</dt>
            <dd>{user.title || "None"}</dd>
            <dt>Bio</dt>
            <dd>{user.info || "None"}</dd>
            <dt>Status</dt>
            <dd>
              {!user.enabled && (
                <span className="admin-user-detail__badge admin-user-detail__badge--disabled">
                  Disabled
                </span>
              )}
              {user.enabled && !user.warned && (
                <span className="admin-user-detail__badge admin-user-detail__badge--enabled">
                  Active
                </span>
              )}
              {user.warned && (
                <span className="admin-user-detail__badge admin-user-detail__badge--warned">
                  Warned
                </span>
              )}
              {user.donor && (
                <span className="admin-user-detail__badge admin-user-detail__badge--donor">
                  Donor
                </span>
              )}
              {user.parked && (
                <span className="admin-user-detail__badge admin-user-detail__badge--parked">
                  Parked
                </span>
              )}
            </dd>
            <dt>Joined</dt>
            <dd>{timeAgo(user.created_at)}</dd>
            <dt>Last Active</dt>
            <dd>{user.last_access ? timeAgo(user.last_access) : "Never"}</dd>
          </dl>
        </div>

        <div className="admin-user-detail__card">
          <h2>Stats</h2>
          <dl className="admin-user-detail__dl">
            <dt>Uploaded</dt>
            <dd>{formatBytes(user.uploaded)}</dd>
            <dt>Downloaded</dt>
            <dd>{formatBytes(user.downloaded)}</dd>
            <dt>Ratio</dt>
            <dd>{formatRatio(user.ratio)}</dd>
            <dt>Invites</dt>
            <dd>{user.invites}</dd>
            <dt>Active Warnings</dt>
            <dd>{user.warnings_count}</dd>
          </dl>
        </div>
      </div>

      {/* Recent Uploads */}
      <div className="admin-user-detail__card">
        <h2>Recent Uploads</h2>
        {user.recent_uploads.length === 0 ? (
          <p className="admin-user-detail__empty">No uploads.</p>
        ) : (
          <table className="admin-user-detail__table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Size</th>
                <th>Date</th>
              </tr>
            </thead>
            <tbody>
              {user.recent_uploads.map((t) => (
                <tr key={t.id}>
                  <td>
                    <Link to={`/torrent/${t.id}`}>{t.name}</Link>
                  </td>
                  <td>{formatBytes(t.size)}</td>
                  <td>{timeAgo(t.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Mod Notes */}
      <div className="admin-user-detail__card">
        <h2>Staff Notes</h2>
        <div className="admin-user-detail__note-form">
          <Textarea
            label=""
            placeholder="Add a private staff note..."
            value={newNote}
            onChange={(e) => setNewNote(e.target.value)}
          />
          <button
            className="admin-user-detail__add-note-btn"
            onClick={handleAddNote}
            disabled={addingNote || !newNote.trim()}
          >
            {addingNote ? "Adding..." : "Add Note"}
          </button>
        </div>
        {user.mod_notes.length === 0 ? (
          <p className="admin-user-detail__empty">No staff notes.</p>
        ) : (
          <div className="admin-user-detail__notes">
            {user.mod_notes.map((note) => (
              <div key={note.id} className="admin-user-detail__note">
                <div className="admin-user-detail__note-header">
                  <span className="admin-user-detail__note-author">
                    {note.author_username}
                  </span>
                  <span className="admin-user-detail__note-date">
                    {timeAgo(note.created_at)}
                  </span>
                  <button
                    className="admin-user-detail__note-delete"
                    onClick={() => setDeletingNoteId(note.id)}
                  >
                    Delete
                  </button>
                </div>
                <p className="admin-user-detail__note-body">{note.note}</p>
              </div>
            ))}
          </div>
        )}
      </div>

      <ConfirmModal
        isOpen={deletingNoteId !== null}
        title="Delete Note"
        message="Are you sure you want to delete this staff note?"
        confirmLabel="Delete"
        danger
        onConfirm={handleDeleteNote}
        onCancel={() => setDeletingNoteId(null)}
      />
    </div>
  );
}
