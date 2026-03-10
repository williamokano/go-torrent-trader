import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams, useNavigate } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { formatBytes, timeAgo } from "@/utils/format";
import { UsernameDisplay } from "@/components/UsernameDisplay";
import { ConfirmModal } from "@/components/modal/ConfirmModal";
import { Modal } from "@/components/modal/Modal";
import { Textarea, Input, Checkbox, Select } from "@/components/form";
import { BanUserModal } from "@/pages/admin/BanUserModal";
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

interface Restriction {
  id: number;
  user_id: number;
  restriction_type: string;
  reason: string;
  issued_by: number | null;
  issued_by_username: string;
  expires_at: string | null;
  lifted_at: string | null;
  lifted_by: number | null;
  lifted_by_username: string;
  created_at: string;
}

interface GroupOption {
  value: string;
  label: string;
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
  passkey: string | null;
  invites: number;
  can_download: boolean;
  can_upload: boolean;
  can_chat: boolean;
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
  const [banModalOpen, setBanModalOpen] = useState(false);
  const [banning, setBanning] = useState(false);

  // Edit form state
  const [editUsername, setEditUsername] = useState("");
  const [editEmail, setEditEmail] = useState("");
  const [editAvatar, setEditAvatar] = useState("");
  const [editTitle, setEditTitle] = useState("");
  const [editInfo, setEditInfo] = useState("");
  const [editGroupId, setEditGroupId] = useState("");
  const [editUploaded, setEditUploaded] = useState("");
  const [editDownloaded, setEditDownloaded] = useState("");
  const [editInvites, setEditInvites] = useState("");
  const [editEnabled, setEditEnabled] = useState(true);
  const [editWarned, setEditWarned] = useState(false);
  const [editDonor, setEditDonor] = useState(false);
  const [editParked, setEditParked] = useState(false);
  const [saving, setSaving] = useState(false);
  const [groups, setGroups] = useState<GroupOption[]>([]);

  // Password reset state
  const [showPasswordModal, setShowPasswordModal] = useState(false);
  const [newPassword, setNewPassword] = useState("");
  const [generatedPassword, setGeneratedPassword] = useState<string | null>(
    null,
  );
  const [resettingPassword, setResettingPassword] = useState(false);

  // Passkey reset state
  const [showPasskeyConfirm, setShowPasskeyConfirm] = useState(false);
  const [generatedPasskey, setGeneratedPasskey] = useState<string | null>(null);
  const [resettingPasskey, setResettingPasskey] = useState(false);

  // Restriction state
  const [restrictions, setRestrictions] = useState<Restriction[]>([]);
  const [restrictionReason, setRestrictionReason] = useState("");
  const [restrictionExpiry, setRestrictionExpiry] = useState("");
  const [restrictDownload, setRestrictDownload] = useState(false);
  const [restrictUpload, setRestrictUpload] = useState(false);
  const [restrictChat, setRestrictChat] = useState(false);
  const [applyingRestrictions, setApplyingRestrictions] = useState(false);
  const [liftingRestrictionId, setLiftingRestrictionId] = useState<
    number | null
  >(null);

  const populateEditForm = useCallback((u: UserDetail) => {
    setEditUsername(u.username);
    setEditEmail(u.email);
    setEditAvatar(u.avatar ?? "");
    setEditTitle(u.title ?? "");
    setEditInfo(u.info ?? "");
    setEditGroupId(String(u.group_id));
    setEditUploaded(String(u.uploaded));
    setEditDownloaded(String(u.downloaded));
    setEditInvites(String(u.invites));
    setEditEnabled(u.enabled);
    setEditWarned(u.warned);
    setEditDonor(u.donor);
    setEditParked(u.parked);
  }, []);

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
        populateEditForm(data.user);
      } else if (res.status === 404) {
        navigate("/admin/users");
        toastRef.current.error("User not found");
      }
    } finally {
      setLoading(false);
    }
  }, [id, navigate, populateEditForm]);

  const fetchRestrictions = useCallback(async () => {
    const token = getAccessToken();
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/users/${id}/restrictions`,
      {
        headers: { Authorization: `Bearer ${token}` },
      },
    );
    if (res.ok) {
      const data = await res.json();
      setRestrictions(data.restrictions || []);
    }
  }, [id]);

  const fetchGroups = useCallback(async () => {
    const token = getAccessToken();
    const res = await fetch(`${getConfig().API_URL}/api/v1/admin/groups`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (res.ok) {
      const data = await res.json();
      setGroups(
        (data.groups ?? []).map((g: { id: number; name: string }) => ({
          value: String(g.id),
          label: g.name,
        })),
      );
    }
  }, []);

  useEffect(() => {
    fetchUser();
    fetchRestrictions();
    fetchGroups();
  }, [fetchUser, fetchRestrictions, fetchGroups]);

  const handleSaveProfile = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/users/${id}`,
        {
          method: "PUT",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify({
            username: editUsername,
            email: editEmail,
            avatar: editAvatar || null,
            title: editTitle || null,
            info: editInfo || null,
            group_id: Number(editGroupId),
            uploaded: Number(editUploaded),
            downloaded: Number(editDownloaded),
            invites: Number(editInvites),
            enabled: editEnabled,
            warned: editWarned,
            donor: editDonor,
            parked: editParked,
          }),
        },
      );
      if (res.ok) {
        toast.success("User updated successfully");
        fetchUser();
      } else {
        const err = await res.json().catch(() => null);
        toast.error(err?.error?.message ?? "Failed to update user");
      }
    } finally {
      setSaving(false);
    }
  };

  const handleResetPassword = async () => {
    setResettingPassword(true);
    try {
      const token = getAccessToken();
      const body: Record<string, string> = {};
      if (newPassword.trim()) {
        body.new_password = newPassword.trim();
      }

      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/users/${id}/reset-password`,
        {
          method: "PUT",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify(body),
        },
      );

      if (res.ok) {
        const data = await res.json();
        setGeneratedPassword(data.new_password);
        setShowPasswordModal(false);
        toast.success("Password reset successfully");
      } else {
        const err = await res.json().catch(() => null);
        toast.error(err?.error?.message ?? "Failed to reset password");
      }
    } finally {
      setResettingPassword(false);
    }
  };

  const handleResetPasskey = async () => {
    setResettingPasskey(true);
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/users/${id}/reset-passkey`,
        {
          method: "PUT",
          headers: {
            Authorization: `Bearer ${token}`,
          },
        },
      );

      if (res.ok) {
        const data = await res.json();
        setGeneratedPasskey(data.new_passkey);
        setShowPasskeyConfirm(false);
        toast.success("Passkey reset successfully");
      } else {
        const err = await res.json().catch(() => null);
        toast.error(err?.error?.message ?? "Failed to reset passkey");
      }
    } finally {
      setResettingPasskey(false);
    }
  };

  const handleCopy = (text: string) => {
    navigator.clipboard.writeText(text).then(
      () => toast.success("Copied to clipboard"),
      () => toast.error("Failed to copy"),
    );
  };

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

  const handleApplyRestrictions = async () => {
    if (!restrictionReason.trim()) {
      toast.error("Reason is required");
      return;
    }
    if (!restrictDownload && !restrictUpload && !restrictChat) {
      toast.error("Select at least one privilege to restrict");
      return;
    }

    setApplyingRestrictions(true);
    const token = getAccessToken();
    try {
      const body: Record<string, unknown> = {
        reason: restrictionReason,
      };
      if (restrictDownload) body.can_download = false;
      if (restrictUpload) body.can_upload = false;
      if (restrictChat) body.can_chat = false;
      if (restrictionExpiry) body.expires_at = restrictionExpiry;

      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/users/${id}/restrictions`,
        {
          method: "PUT",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify(body),
        },
      );
      if (res.ok) {
        toast.success("Restrictions applied");
        setRestrictionReason("");
        setRestrictionExpiry("");
        setRestrictDownload(false);
        setRestrictUpload(false);
        setRestrictChat(false);
        fetchUser();
        fetchRestrictions();
      } else {
        const err = await res.json().catch(() => null);
        toast.error(err?.error?.message ?? "Failed to apply restrictions");
      }
    } finally {
      setApplyingRestrictions(false);
    }
  };

  const handleRestorePrivilege = async (type: string) => {
    const token = getAccessToken();
    const body: Record<string, unknown> = {
      reason: "Privilege restored by admin",
    };
    if (type === "download") body.can_download = true;
    if (type === "upload") body.can_upload = true;
    if (type === "chat") body.can_chat = true;

    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/users/${id}/restrictions`,
      {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(body),
      },
    );
    if (res.ok) {
      toast.success(`${type} privilege restored`);
      fetchUser();
      fetchRestrictions();
    } else {
      toast.error(`Failed to restore ${type} privilege`);
    }
  };

  const handleLiftRestriction = async () => {
    if (!liftingRestrictionId) return;
    const token = getAccessToken();
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/restrictions/${liftingRestrictionId}`,
      {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      },
    );
    if (res.ok) {
      toast.success("Restriction lifted");
      fetchUser();
      fetchRestrictions();
    } else {
      toast.error("Failed to lift restriction");
    }
    setLiftingRestrictionId(null);
  };

  const handleBan = async (data: {
    reason: string;
    ban_ip: boolean;
    ban_email: boolean;
    duration_days: number | null;
  }) => {
    setBanning(true);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/users/${id}/ban`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify(data),
        },
      );
      if (res.ok) {
        toast.success("User banned successfully");
        setBanModalOpen(false);
        fetchUser();
      } else {
        const err = await res.json().catch(() => null);
        toast.error(err?.error || "Failed to ban user");
      }
    } finally {
      setBanning(false);
    }
  };

  const formatRatio = (ratio: number) => {
    if (ratio === -1) return "Inf";
    if (ratio === 0) return "0.00";
    return ratio.toFixed(2);
  };

  if (loading) return <p>Loading...</p>;
  if (!user) return <p>User not found.</p>;

  const displayPasskey = generatedPasskey ?? user.passkey;

  return (
    <div className="admin-user-detail">
      <div className="admin-user-detail__header">
        <Link to="/admin/users" className="admin-user-detail__back">
          &larr; Back to Users
        </Link>
        <h1>
          <UsernameDisplay
            userId={user.id}
            username={user.username}
            warned={user.warned}
            noLink
          />
          <span className="admin-user-detail__header-meta">
            {user.group_name} &middot; Joined {timeAgo(user.created_at)}{" "}
            &middot; Last active{" "}
            {user.last_access ? timeAgo(user.last_access) : "Never"}
          </span>
        </h1>
        {user.enabled && (
          <button
            className="admin-user-detail__ban-btn"
            onClick={() => setBanModalOpen(true)}
          >
            Ban User
          </button>
        )}
      </div>

      {/* Edit Profile Form */}
      <div className="admin-user-detail__card">
        <h2>Edit Profile</h2>
        <form onSubmit={handleSaveProfile}>
          <div className="admin-user-detail__form-row">
            <div className="admin-user-detail__form-field">
              <Input
                label="Username"
                value={editUsername}
                onChange={(e) => setEditUsername(e.target.value)}
              />
            </div>
            <div className="admin-user-detail__form-field">
              <Input
                label="Email"
                type="email"
                value={editEmail}
                onChange={(e) => setEditEmail(e.target.value)}
              />
            </div>
            <div className="admin-user-detail__form-field">
              <Select
                label="Group"
                options={groups}
                value={editGroupId}
                onChange={(e) => setEditGroupId(e.target.value)}
              />
            </div>
          </div>

          <Input
            label="Avatar URL"
            value={editAvatar}
            onChange={(e) => setEditAvatar(e.target.value)}
            placeholder="https://..."
          />
          <Input
            label="Title"
            value={editTitle}
            onChange={(e) => setEditTitle(e.target.value)}
          />
          <Textarea
            label="Info / Bio"
            value={editInfo}
            onChange={(e) => setEditInfo(e.target.value)}
          />

          <div className="admin-user-detail__form-row">
            <div className="admin-user-detail__form-field">
              <Input
                label="Uploaded (bytes)"
                type="number"
                value={editUploaded}
                onChange={(e) => setEditUploaded(e.target.value)}
              />
            </div>
            <div className="admin-user-detail__form-field">
              <Input
                label="Downloaded (bytes)"
                type="number"
                value={editDownloaded}
                onChange={(e) => setEditDownloaded(e.target.value)}
              />
            </div>
            <div className="admin-user-detail__form-field">
              <Input
                label="Invites"
                type="number"
                value={editInvites}
                onChange={(e) => setEditInvites(e.target.value)}
              />
            </div>
          </div>

          <div className="admin-user-detail__form-flags">
            <Checkbox
              label="Enabled"
              checked={editEnabled}
              onChange={(e) => setEditEnabled(e.target.checked)}
            />
            <Checkbox
              label="Warned"
              checked={editWarned}
              onChange={(e) => setEditWarned(e.target.checked)}
            />
            <Checkbox
              label="Donor"
              checked={editDonor}
              onChange={(e) => setEditDonor(e.target.checked)}
            />
            <Checkbox
              label="Parked"
              checked={editParked}
              onChange={(e) => setEditParked(e.target.checked)}
            />
          </div>

          {/* Passkey display */}
          {displayPasskey && (
            <div className="admin-user-detail__passkey">
              <label>Passkey</label>
              <code>{displayPasskey}</code>
            </div>
          )}

          {/* Generated password display */}
          {generatedPassword && (
            <div className="admin-user-detail__generated-value">
              <label>New Password:</label>
              <div className="admin-user-detail__copyable">
                <code>{generatedPassword}</code>
                <button
                  type="button"
                  onClick={() => handleCopy(generatedPassword)}
                >
                  Copy
                </button>
              </div>
            </div>
          )}

          {/* Stats summary (read-only) */}
          <div className="admin-user-detail__stats-summary">
            <span>
              Ratio: <strong>{formatRatio(user.ratio)}</strong>
            </span>
            <span>
              Active Warnings: <strong>{user.warnings_count}</strong>
            </span>
          </div>

          <div className="admin-user-detail__form-actions">
            <button
              type="submit"
              className="admin-user-detail__save-btn"
              disabled={saving}
            >
              {saving ? "Saving..." : "Save Changes"}
            </button>
            <button
              type="button"
              className="admin-user-detail__reset-btn"
              onClick={() => {
                setNewPassword("");
                setGeneratedPassword(null);
                setShowPasswordModal(true);
              }}
            >
              Reset Password
            </button>
            <button
              type="button"
              className="admin-user-detail__reset-btn admin-user-detail__reset-btn--danger"
              onClick={() => {
                setGeneratedPasskey(null);
                setShowPasskeyConfirm(true);
              }}
            >
              Reset Passkey
            </button>
          </div>
        </form>
      </div>

      {/* Privilege Restrictions */}
      <div className="admin-user-detail__card">
        <h2>Privilege Restrictions</h2>

        {/* Current status */}
        <div className="admin-user-detail__restrictions-status">
          <div className="admin-user-detail__restriction-item">
            <span>Download:</span>
            {user.can_download ? (
              <span className="admin-user-detail__badge admin-user-detail__badge--enabled">
                Allowed
              </span>
            ) : (
              <>
                <span className="admin-user-detail__badge admin-user-detail__badge--disabled">
                  Suspended
                </span>
                <button
                  className="admin-user-detail__restore-btn"
                  onClick={() => handleRestorePrivilege("download")}
                >
                  Restore
                </button>
              </>
            )}
          </div>
          <div className="admin-user-detail__restriction-item">
            <span>Upload:</span>
            {user.can_upload ? (
              <span className="admin-user-detail__badge admin-user-detail__badge--enabled">
                Allowed
              </span>
            ) : (
              <>
                <span className="admin-user-detail__badge admin-user-detail__badge--disabled">
                  Suspended
                </span>
                <button
                  className="admin-user-detail__restore-btn"
                  onClick={() => handleRestorePrivilege("upload")}
                >
                  Restore
                </button>
              </>
            )}
          </div>
          <div className="admin-user-detail__restriction-item">
            <span>Chat:</span>
            {user.can_chat ? (
              <span className="admin-user-detail__badge admin-user-detail__badge--enabled">
                Allowed
              </span>
            ) : (
              <>
                <span className="admin-user-detail__badge admin-user-detail__badge--disabled">
                  Suspended
                </span>
                <button
                  className="admin-user-detail__restore-btn"
                  onClick={() => handleRestorePrivilege("chat")}
                >
                  Restore
                </button>
              </>
            )}
          </div>
        </div>

        {/* Apply new restrictions */}
        <div className="admin-user-detail__restriction-form">
          <h3>Apply Restriction</h3>
          <div
            style={{
              display: "flex",
              gap: "var(--space-lg)",
              flexWrap: "wrap",
            }}
          >
            <Checkbox
              label="Suspend Download"
              checked={restrictDownload}
              onChange={(e) => setRestrictDownload(e.target.checked)}
            />
            <Checkbox
              label="Suspend Upload"
              checked={restrictUpload}
              onChange={(e) => setRestrictUpload(e.target.checked)}
            />
            <Checkbox
              label="Suspend Chat"
              checked={restrictChat}
              onChange={(e) => setRestrictChat(e.target.checked)}
            />
          </div>
          <Textarea
            label="Reason"
            placeholder="Reason for restriction..."
            value={restrictionReason}
            onChange={(e) => setRestrictionReason(e.target.value)}
          />
          <div className="admin-user-detail__field-group">
            <label htmlFor="restriction-expiry" className="admin-user-detail__field-label">
              Expires At (optional)
            </label>
            <input
              id="restriction-expiry"
              type="datetime-local"
              value={restrictionExpiry}
              onChange={(e) => setRestrictionExpiry(e.target.value)}
              className="admin-user-detail__date-input"
            />
          </div>
          <button
            className="admin-user-detail__add-note-btn"
            onClick={handleApplyRestrictions}
            disabled={
              applyingRestrictions ||
              !restrictionReason.trim() ||
              (!restrictDownload && !restrictUpload && !restrictChat)
            }
          >
            {applyingRestrictions ? "Applying..." : "Apply Restrictions"}
          </button>
        </div>

        {/* Restriction history */}
        {restrictions.length > 0 && (
          <>
            <h3 style={{ marginTop: "var(--space-lg)" }}>
              Restriction History
            </h3>
            <table className="admin-user-detail__table">
              <thead>
                <tr>
                  <th>Type</th>
                  <th>Reason</th>
                  <th>Issued By</th>
                  <th>Created</th>
                  <th>Expires</th>
                  <th>Status</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {restrictions.map((r) => (
                  <tr key={r.id}>
                    <td>{r.restriction_type}</td>
                    <td>{r.reason}</td>
                    <td>
                      {r.issued_by_username ||
                        (r.issued_by ? `#${r.issued_by}` : "System")}
                    </td>
                    <td>{timeAgo(r.created_at)}</td>
                    <td>
                      {r.expires_at ? timeAgo(r.expires_at) : "Permanent"}
                    </td>
                    <td>
                      {r.lifted_at ? (
                        <span className="admin-user-detail__badge admin-user-detail__badge--enabled">
                          Lifted
                          {r.lifted_by_username
                            ? ` by ${r.lifted_by_username}`
                            : ""}
                        </span>
                      ) : (
                        <span className="admin-user-detail__badge admin-user-detail__badge--disabled">
                          Active
                        </span>
                      )}
                    </td>
                    <td>
                      {!r.lifted_at && (
                        <button
                          className="admin-user-detail__note-delete"
                          onClick={() => setLiftingRestrictionId(r.id)}
                        >
                          Lift
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}
        {restrictions.length === 0 && (
          <p className="admin-user-detail__empty">No restrictions.</p>
        )}
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
                    <UsernameDisplay
                      userId={note.author_id}
                      username={note.author_username}
                    />
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

      {/* Reset Password Modal */}
      <Modal
        isOpen={showPasswordModal}
        onClose={() => setShowPasswordModal(false)}
        title={`Reset Password for ${user.username}`}
      >
        <div className="admin-user-detail__modal-form">
          <p style={{ color: "var(--color-text-muted)", margin: 0 }}>
            Leave empty to generate a random password. The user will be logged
            out of all sessions.
          </p>
          <Input
            label="New Password (optional)"
            type="text"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            placeholder="Leave blank to auto-generate"
          />
          <div className="admin-user-detail__modal-actions">
            <button
              type="button"
              className="admin-user-detail__cancel-btn"
              onClick={() => setShowPasswordModal(false)}
            >
              Cancel
            </button>
            <button
              type="button"
              className="admin-user-detail__save-btn"
              disabled={resettingPassword}
              onClick={handleResetPassword}
            >
              {resettingPassword ? "Resetting..." : "Reset Password"}
            </button>
          </div>
        </div>
      </Modal>

      {/* Reset Passkey Confirm Modal */}
      <ConfirmModal
        isOpen={showPasskeyConfirm}
        title={`Reset Passkey for ${user.username}`}
        message="This will invalidate all existing .torrent files for this user. They will need to re-download all their torrent files. Continue?"
        confirmLabel={resettingPasskey ? "Resetting..." : "Reset Passkey"}
        danger
        onConfirm={handleResetPasskey}
        onCancel={() => setShowPasskeyConfirm(false)}
      />

      <ConfirmModal
        isOpen={deletingNoteId !== null}
        title="Delete Note"
        message="Are you sure you want to delete this staff note?"
        confirmLabel="Delete"
        danger
        onConfirm={handleDeleteNote}
        onCancel={() => setDeletingNoteId(null)}
      />

      <ConfirmModal
        isOpen={liftingRestrictionId !== null}
        title="Lift Restriction"
        message="Are you sure you want to lift this restriction?"
        confirmLabel="Lift"
        danger
        onConfirm={handleLiftRestriction}
        onCancel={() => setLiftingRestrictionId(null)}
      />

      <BanUserModal
        isOpen={banModalOpen}
        username={user.username}
        onConfirm={handleBan}
        onCancel={() => setBanModalOpen(false)}
        loading={banning}
      />
    </div>
  );
}
