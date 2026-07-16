import { useCallback, useEffect, useState } from "react";
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
import "./admin-ui.css";
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
  bonus_points: number;
  can_download: boolean;
  can_upload: boolean;
  can_chat: boolean;
  can_invite: boolean;
  disabled_until: string | null;
  created_at: string;
  last_access: string | null;
  ratio: number;
  recent_uploads: TorrentSummary[];
  warnings_count: number;
  mod_notes: ModNote[];
}

type PrivilegeType = "download" | "upload" | "chat" | "invite";

const PRIVILEGES: {
  type: PrivilegeType;
  label: string;
  field: keyof Pick<
    UserDetail,
    "can_download" | "can_upload" | "can_chat" | "can_invite"
  >;
}[] = [
  { type: "download", label: "Download", field: "can_download" },
  { type: "upload", label: "Upload", field: "can_upload" },
  { type: "chat", label: "Chat", field: "can_chat" },
  { type: "invite", label: "Invite", field: "can_invite" },
];

export function AdminUserDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const toast = useToast();

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
  const [editBonusPoints, setEditBonusPoints] = useState("");
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
  const [restrictInvite, setRestrictInvite] = useState(false);
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
    setEditBonusPoints(String(u.bonus_points ?? 0));
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
        toast.error("User not found");
      }
    } finally {
      setLoading(false);
    }
  }, [id, navigate, populateEditForm, toast]);

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
            bonus_points: Number(editBonusPoints),
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
    if (
      !restrictDownload &&
      !restrictUpload &&
      !restrictChat &&
      !restrictInvite
    ) {
      toast.error("Select at least one privilege to suspend");
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
      if (restrictInvite) body.can_invite = false;
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
        setRestrictInvite(false);
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

  const handleRestorePrivilege = async (type: PrivilegeType) => {
    const token = getAccessToken();
    const body: Record<string, unknown> = {
      reason: "Privilege restored by admin",
    };
    if (type === "download") body.can_download = true;
    if (type === "upload") body.can_upload = true;
    if (type === "chat") body.can_chat = true;
    if (type === "invite") body.can_invite = true;

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
        const result = await res.json().catch(() => null);
        const parts: string[] = ["User banned"];
        if (result?.ip_banned) parts.push("IP banned");
        if (result?.email_banned)
          parts.push(`email domain banned (${result.email_pattern})`);
        toast.success(parts.join(", "));
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

  if (loading) return <p>Loading…</p>;
  if (!user) return <p>User not found.</p>;

  const displayPasskey = generatedPasskey ?? user.passkey;

  return (
    <div className="admin-user-detail">
      <Link to="/admin/users" className="admin-user-detail__back">
        &larr; Back to users
      </Link>

      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title admin-user-detail__title">
            <UsernameDisplay
              userId={user.id}
              username={user.username}
              warned={user.warned}
              noLink
            />
            {!user.enabled ? (
              <span className="admin-badge admin-badge--danger">Disabled</span>
            ) : user.warned ? (
              <span className="admin-badge admin-badge--warn">Warned</span>
            ) : (
              <span className="admin-badge admin-badge--ok">Active</span>
            )}
            {user.donor && (
              <span className="admin-badge admin-badge--accent">Donor</span>
            )}
            {user.parked && (
              <span className="admin-badge admin-badge--muted">Parked</span>
            )}
          </h1>
          <p className="admin-page-header__desc">
            {user.group_name} &middot; Joined {timeAgo(user.created_at)}{" "}
            &middot; Last active{" "}
            {user.last_access ? timeAgo(user.last_access) : "never"}
          </p>
        </div>
        <div className="admin-page-header__actions">
          {user.enabled ? (
            <button
              className="admin-btn admin-btn--danger"
              onClick={() => setBanModalOpen(true)}
            >
              Ban user
            </button>
          ) : (
            <span className="admin-badge admin-badge--danger">
              Banned
              {user.disabled_until
                ? ` (until ${timeAgo(user.disabled_until)})`
                : " (permanent)"}
            </span>
          )}
        </div>
      </div>

      <div className="admin-stack">
        {/* Key figures */}
        <div className="admin-panel">
          <div className="admin-stats">
            <div>
              <span className="admin-stat__label">Uploaded</span>
              <span className="admin-stat__value">
                {formatBytes(user.uploaded)}
              </span>
            </div>
            <div>
              <span className="admin-stat__label">Downloaded</span>
              <span className="admin-stat__value">
                {formatBytes(user.downloaded)}
              </span>
            </div>
            <div>
              <span className="admin-stat__label">Ratio</span>
              <span className="admin-stat__value">
                {formatRatio(user.ratio)}
              </span>
            </div>
            <div>
              <span className="admin-stat__label">Invites</span>
              <span className="admin-stat__value">{user.invites}</span>
            </div>
            <div>
              <span className="admin-stat__label">Active warnings</span>
              <span className="admin-stat__value">{user.warnings_count}</span>
            </div>
          </div>
        </div>

        {/* Edit Profile */}
        <div className="admin-panel">
          <div className="admin-panel__section">
            <h2 className="admin-panel__title">Edit profile</h2>
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
                <div className="admin-user-detail__form-field">
                  <Input
                    label="Bonus Points"
                    type="number"
                    min="0"
                    value={editBonusPoints}
                    onChange={(e) => setEditBonusPoints(e.target.value)}
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

              {displayPasskey && (
                <div className="admin-user-detail__passkey">
                  <label>Passkey</label>
                  <code>{displayPasskey}</code>
                </div>
              )}

              {generatedPassword && (
                <div className="admin-user-detail__generated-value">
                  <label>New password</label>
                  <div className="admin-user-detail__copyable">
                    <code>{generatedPassword}</code>
                    <button
                      type="button"
                      className="admin-btn admin-btn--primary admin-btn--sm"
                      onClick={() => handleCopy(generatedPassword)}
                    >
                      Copy
                    </button>
                  </div>
                </div>
              )}

              <div className="admin-user-detail__form-actions">
                <button
                  type="submit"
                  className="admin-btn admin-btn--primary"
                  disabled={saving}
                >
                  {saving ? "Saving…" : "Save changes"}
                </button>
                <button
                  type="button"
                  className="admin-btn admin-btn--ghost"
                  onClick={() => {
                    setNewPassword("");
                    setGeneratedPassword(null);
                    setShowPasswordModal(true);
                  }}
                >
                  Reset password
                </button>
                <button
                  type="button"
                  className="admin-btn admin-btn--danger"
                  onClick={() => {
                    setGeneratedPasskey(null);
                    setShowPasskeyConfirm(true);
                  }}
                >
                  Reset passkey
                </button>
              </div>
            </form>
          </div>
        </div>

        {/* Privileges */}
        <div className="admin-panel">
          <div className="admin-panel__section">
            <h2 className="admin-panel__title">Privileges</h2>
            <div className="admin-user-detail__privileges">
              {PRIVILEGES.map(({ type, label, field }) => (
                <div key={type} className="admin-user-detail__privilege">
                  <span className="admin-user-detail__privilege-name">
                    {label}
                  </span>
                  {user[field] ? (
                    <span className="admin-badge admin-badge--ok">Allowed</span>
                  ) : (
                    <>
                      <span className="admin-badge admin-badge--danger">
                        Suspended
                      </span>
                      <button
                        className="admin-btn admin-btn--ghost admin-btn--sm"
                        onClick={() => handleRestorePrivilege(type)}
                      >
                        Restore
                      </button>
                    </>
                  )}
                </div>
              ))}
            </div>
          </div>

          <div className="admin-panel__section">
            <h2 className="admin-panel__title">Suspend privileges</h2>
            <div className="admin-user-detail__restriction-checks">
              <Checkbox
                label="Suspend download"
                checked={restrictDownload}
                onChange={(e) => setRestrictDownload(e.target.checked)}
              />
              <Checkbox
                label="Suspend upload"
                checked={restrictUpload}
                onChange={(e) => setRestrictUpload(e.target.checked)}
              />
              <Checkbox
                label="Suspend chat"
                checked={restrictChat}
                onChange={(e) => setRestrictChat(e.target.checked)}
              />
              <Checkbox
                label="Suspend invite"
                checked={restrictInvite}
                onChange={(e) => setRestrictInvite(e.target.checked)}
              />
            </div>
            <Textarea
              label="Reason"
              placeholder="Reason for suspending these privileges…"
              value={restrictionReason}
              onChange={(e) => setRestrictionReason(e.target.value)}
            />
            <div className="admin-user-detail__field-group">
              <label
                htmlFor="restriction-expiry"
                className="admin-user-detail__field-label"
              >
                Expires at (optional)
              </label>
              <input
                id="restriction-expiry"
                type="datetime-local"
                value={restrictionExpiry}
                onChange={(e) => setRestrictionExpiry(e.target.value)}
                className="admin-user-detail__date-input"
              />
            </div>
            <div className="admin-user-detail__form-actions">
              <button
                className="admin-btn admin-btn--primary"
                onClick={handleApplyRestrictions}
                disabled={
                  applyingRestrictions ||
                  !restrictionReason.trim() ||
                  (!restrictDownload &&
                    !restrictUpload &&
                    !restrictChat &&
                    !restrictInvite)
                }
              >
                {applyingRestrictions ? "Applying…" : "Apply restrictions"}
              </button>
            </div>
          </div>

          {restrictions.length > 0 ? (
            <div className="admin-table-scroll">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>Type</th>
                    <th>Reason</th>
                    <th>Issued by</th>
                    <th>Created</th>
                    <th>Expires</th>
                    <th>Status</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {restrictions.map((r) => (
                    <tr key={r.id}>
                      <td className="admin-table__name">
                        {r.restriction_type}
                      </td>
                      <td>{r.reason}</td>
                      <td>
                        {r.issued_by_username ||
                          (r.issued_by ? `#${r.issued_by}` : "System")}
                      </td>
                      <td className="admin-muted">{timeAgo(r.created_at)}</td>
                      <td className="admin-muted">
                        {r.expires_at ? timeAgo(r.expires_at) : "Permanent"}
                      </td>
                      <td>
                        {r.lifted_at ? (
                          <span className="admin-badge admin-badge--muted">
                            Lifted
                            {r.lifted_by_username
                              ? ` by ${r.lifted_by_username}`
                              : ""}
                          </span>
                        ) : (
                          <span className="admin-badge admin-badge--danger">
                            Active
                          </span>
                        )}
                      </td>
                      <td className="admin-table__actions">
                        {!r.lifted_at && (
                          <button
                            className="admin-btn admin-btn--ghost admin-btn--sm"
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
            </div>
          ) : (
            <p className="admin-empty">
              No restrictions have been applied to this account.
            </p>
          )}
        </div>

        {/* Recent Uploads */}
        <div className="admin-panel">
          <div className="admin-panel__section">
            <h2 className="admin-panel__title" style={{ margin: 0 }}>
              Recent uploads
            </h2>
          </div>
          {user.recent_uploads.length === 0 ? (
            <p className="admin-empty">No uploads yet.</p>
          ) : (
            <div className="admin-table-scroll">
              <table className="admin-table">
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
                      <td className="admin-table__name">
                        <Link to={`/torrent/${t.id}`}>{t.name}</Link>
                      </td>
                      <td className="admin-num">{formatBytes(t.size)}</td>
                      <td className="admin-muted">{timeAgo(t.created_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        {/* Staff Notes */}
        <div className="admin-panel">
          <div className="admin-panel__section">
            <h2 className="admin-panel__title">Staff notes</h2>
            <div className="admin-user-detail__note-form">
              <Textarea
                label=""
                placeholder="Add a private staff note…"
                value={newNote}
                onChange={(e) => setNewNote(e.target.value)}
              />
              <button
                className="admin-btn admin-btn--primary"
                onClick={handleAddNote}
                disabled={addingNote || !newNote.trim()}
              >
                {addingNote ? "Adding…" : "Add note"}
              </button>
            </div>
            {user.mod_notes.length === 0 ? (
              <p className="admin-empty" style={{ padding: 0 }}>
                No staff notes yet.
              </p>
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
                        className="admin-btn admin-btn--danger admin-btn--sm"
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
        </div>
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
              className="admin-btn admin-btn--ghost"
              onClick={() => setShowPasswordModal(false)}
            >
              Cancel
            </button>
            <button
              type="button"
              className="admin-btn admin-btn--primary"
              disabled={resettingPassword}
              onClick={handleResetPassword}
            >
              {resettingPassword ? "Resetting…" : "Reset password"}
            </button>
          </div>
        </div>
      </Modal>

      {/* Reset Passkey Confirm Modal */}
      <ConfirmModal
        isOpen={showPasskeyConfirm}
        title={`Reset Passkey for ${user.username}`}
        message="This will invalidate all existing .torrent files for this user. They will need to re-download all their torrent files. Continue?"
        confirmLabel={resettingPasskey ? "Resetting…" : "Reset Passkey"}
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
        email={user.email}
        onConfirm={handleBan}
        onCancel={() => setBanModalOpen(false)}
        loading={banning}
      />
    </div>
  );
}
