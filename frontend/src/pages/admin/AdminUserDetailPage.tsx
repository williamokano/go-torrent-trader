import { useCallback, useEffect, useState } from "react";
import { Link, useParams, useNavigate } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { formatBytes, timeAgo } from "@/utils/format";
import { UsernameDisplay } from "@/components/UsernameDisplay";
import { ConfirmModal } from "@/components/modal/ConfirmModal";
import { Modal } from "@/components/modal/Modal";
import {
  Textarea,
  Input,
  Checkbox,
  Select,
  ByteSizeInput,
} from "@/components/form";
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

interface UserInvite {
  id: number;
  token: string;
  status: "pending" | "redeemed" | "expired" | "voided";
  expires_at: string;
  created_at: string;
  invitee_name?: string;
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

interface EditHistoryEntry {
  id: number;
  user_id: number;
  changed_by: number | null;
  changed_by_username: string;
  field: string;
  old_value: string;
  new_value: string;
  created_at: string;
}

const HISTORY_PAGE_SIZE = 50;

const HISTORY_FIELD_LABELS: Record<string, string> = {
  username: "Username",
  email: "Email",
  avatar: "Avatar URL",
  title: "Title",
  info: "Info / Bio",
  group: "Group",
  uploaded: "Uploaded",
  downloaded: "Downloaded",
  invites: "Invites",
  bonus_points: "Bonus points",
  enabled: "Enabled",
  warned: "Warned",
  donor: "Donor",
  parked: "Parked",
};

const HISTORY_BYTE_FIELDS = new Set(["uploaded", "downloaded"]);
const HISTORY_BOOLEAN_FIELDS = new Set([
  "enabled",
  "warned",
  "donor",
  "parked",
]);

function formatHistoryValue(field: string, value: string): string {
  if (value === "") return "—";
  if (HISTORY_BYTE_FIELDS.has(field)) {
    const n = Number(value);
    if (Number.isFinite(n)) return formatBytes(n);
  }
  if (HISTORY_BOOLEAN_FIELDS.has(field)) {
    if (value === "true") return "Yes";
    if (value === "false") return "No";
  }
  return value;
}

// From/To as a pair: when a byte change is smaller than the rounded display
// (a 5 GB shave at 50 TB scale renders "50.00 TB → 50.00 TB"), fall back to
// exact byte counts so a real change never looks like a no-op.
function formatHistoryPair(entry: EditHistoryEntry): [string, string] {
  let from = formatHistoryValue(entry.field, entry.old_value);
  let to = formatHistoryValue(entry.field, entry.new_value);
  if (
    HISTORY_BYTE_FIELDS.has(entry.field) &&
    from === to &&
    entry.old_value !== entry.new_value
  ) {
    from = `${Number(entry.old_value).toLocaleString()} B`;
    to = `${Number(entry.new_value).toLocaleString()} B`;
  }
  return [from, to];
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
  const [editUploaded, setEditUploaded] = useState(0);
  const [editDownloaded, setEditDownloaded] = useState(0);
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

  // Outstanding invites state
  const [invites, setInvites] = useState<UserInvite[]>([]);
  const [revokingInviteId, setRevokingInviteId] = useState<number | null>(null);
  const [revokingInvite, setRevokingInvite] = useState(false);

  // Edit history state
  const [history, setHistory] = useState<EditHistoryEntry[]>([]);
  const [historyTotal, setHistoryTotal] = useState(0);
  const [historyError, setHistoryError] = useState(false);
  const [loadingMoreHistory, setLoadingMoreHistory] = useState(false);

  const populateEditForm = useCallback((u: UserDetail) => {
    setEditUsername(u.username);
    setEditEmail(u.email);
    setEditAvatar(u.avatar ?? "");
    setEditTitle(u.title ?? "");
    setEditInfo(u.info ?? "");
    setEditGroupId(String(u.group_id));
    setEditUploaded(u.uploaded);
    setEditDownloaded(u.downloaded);
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

  const fetchInvites = useCallback(async () => {
    const token = getAccessToken();
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/users/${id}/invites`,
      {
        headers: { Authorization: `Bearer ${token}` },
      },
    );
    if (res.ok) {
      const data = await res.json();
      setInvites(data.invites || []);
    }
  }, [id]);

  const fetchEditHistory = useCallback(
    async (offset = 0) => {
      const token = getAccessToken();
      try {
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/admin/users/${id}/edit-history?limit=${HISTORY_PAGE_SIZE}&offset=${offset}`,
          {
            headers: { Authorization: `Bearer ${token}` },
          },
        );
        if (!res.ok) {
          setHistoryError(true);
          return;
        }
        const data = await res.json();
        const entries: EditHistoryEntry[] = data.entries || [];
        // Dedupe on append: a save between fetches shifts offsets on this
        // newest-first list, so an older page can re-serve rows already shown.
        setHistory((prev) =>
          offset === 0
            ? entries
            : [
                ...prev,
                ...entries.filter((e) => !prev.some((p) => p.id === e.id)),
              ],
        );
        setHistoryTotal(data.total ?? entries.length);
        setHistoryError(false);
      } catch {
        setHistoryError(true);
      }
    },
    [id],
  );

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
    fetchInvites();
    fetchGroups();
    fetchEditHistory();
  }, [
    fetchUser,
    fetchRestrictions,
    fetchInvites,
    fetchGroups,
    fetchEditHistory,
  ]);

  const handleSaveProfile = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!user) return;

    // Send only the fields the admin actually changed. Counters like
    // uploaded/downloaded/invites/bonus points accrue concurrently (announces,
    // auto-grants, seeding awards) — re-sending an untouched absolute value
    // would silently revert that accrual and pollute the edit history with
    // phantom admin edits.
    const body: Record<string, unknown> = {};
    if (editUsername !== user.username) body.username = editUsername;
    if (editEmail !== user.email) body.email = editEmail;
    if (editAvatar !== (user.avatar ?? "")) body.avatar = editAvatar || null;
    if (editTitle !== (user.title ?? "")) body.title = editTitle || null;
    if (editInfo !== (user.info ?? "")) body.info = editInfo || null;
    if (Number(editGroupId) !== user.group_id)
      body.group_id = Number(editGroupId);
    if (editUploaded !== user.uploaded) body.uploaded = editUploaded;
    if (editDownloaded !== user.downloaded) body.downloaded = editDownloaded;
    if (Number(editInvites) !== user.invites)
      body.invites = Number(editInvites);
    if (Number(editBonusPoints) !== (user.bonus_points ?? 0))
      body.bonus_points = Number(editBonusPoints);
    if (editEnabled !== user.enabled) body.enabled = editEnabled;
    if (editWarned !== user.warned) body.warned = editWarned;
    if (editDonor !== user.donor) body.donor = editDonor;
    if (editParked !== user.parked) body.parked = editParked;

    if (Object.keys(body).length === 0) {
      toast.success("No changes to save");
      return;
    }

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
          body: JSON.stringify(body),
        },
      );
      if (res.ok) {
        toast.success("User updated successfully");
        fetchUser();
        fetchEditHistory();
      } else {
        const err = await res.json().catch(() => null);
        toast.error(err?.error?.message ?? "Failed to update user");
      }
    } finally {
      setSaving(false);
    }
  };

  const handleLoadMoreHistory = async () => {
    setLoadingMoreHistory(true);
    try {
      await fetchEditHistory(history.length);
    } finally {
      setLoadingMoreHistory(false);
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

  const handleRevokeInvite = async () => {
    if (!revokingInviteId) return;
    setRevokingInvite(true);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/invites/${revokingInviteId}`,
        {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        },
      );
      if (res.ok) {
        toast.success("Invite revoked");
        fetchInvites();
      } else {
        const err = await res.json().catch(() => null);
        toast.error(err?.error?.message ?? "Failed to revoke invite");
      }
    } finally {
      setRevokingInvite(false);
      setRevokingInviteId(null);
    }
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
            <form onSubmit={handleSaveProfile} className="form-stack">
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
                  <ByteSizeInput
                    label="Uploaded"
                    value={editUploaded}
                    onChange={setEditUploaded}
                  />
                </div>
                <div className="admin-user-detail__form-field">
                  <ByteSizeInput
                    label="Downloaded"
                    value={editDownloaded}
                    onChange={setEditDownloaded}
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
            <div className="form-stack">
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

        {/* Outstanding Invites */}
        <div className="admin-panel">
          <div className="admin-panel__section">
            <h2 className="admin-panel__title admin-panel__title--tight">
              Outstanding invites
            </h2>
          </div>
          {invites.length === 0 ? (
            <p className="admin-empty">No invites created by this user.</p>
          ) : (
            <div className="admin-table-scroll">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>Token</th>
                    <th>Status</th>
                    <th>Invitee</th>
                    <th>Created</th>
                    <th>Expires</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {invites.map((inv) => (
                    <tr key={inv.id}>
                      <td className="admin-num">{inv.token}</td>
                      <td>
                        {inv.status === "pending" && (
                          <span className="admin-badge admin-badge--warn">
                            Pending
                          </span>
                        )}
                        {inv.status === "redeemed" && (
                          <span className="admin-badge admin-badge--ok">
                            Redeemed
                          </span>
                        )}
                        {inv.status === "expired" && (
                          <span className="admin-badge admin-badge--muted">
                            Expired
                          </span>
                        )}
                        {inv.status === "voided" && (
                          <span
                            className="admin-badge admin-badge--muted"
                            title="Blocked because this user's invite privilege is currently suspended"
                          >
                            Voided
                          </span>
                        )}
                      </td>
                      <td>{inv.invitee_name ?? "—"}</td>
                      <td className="admin-muted">{timeAgo(inv.created_at)}</td>
                      <td className="admin-muted">{timeAgo(inv.expires_at)}</td>
                      <td className="admin-table__actions">
                        {(inv.status === "pending" ||
                          inv.status === "voided") && (
                          <button
                            className="admin-btn admin-btn--danger admin-btn--sm"
                            onClick={() => setRevokingInviteId(inv.id)}
                          >
                            Revoke
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        {/* Recent Uploads */}
        <div className="admin-panel">
          <div className="admin-panel__section">
            <h2 className="admin-panel__title admin-panel__title--tight">
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
        {/* Edit History */}
        <div className="admin-panel">
          <div className="admin-panel__section">
            <h2 className="admin-panel__title admin-panel__title--tight">
              Edit history
            </h2>
            <p className="admin-user-detail__history-desc">
              Every change staff make to this profile, with the value before and
              after.
            </p>
          </div>
          {historyError ? (
            <p className="admin-empty">
              Couldn't load the edit history.{" "}
              <button
                className="admin-btn admin-btn--ghost admin-btn--sm"
                onClick={() => fetchEditHistory()}
              >
                Retry
              </button>
            </p>
          ) : history.length === 0 ? (
            <p className="admin-empty">No edits recorded yet.</p>
          ) : (
            <>
              <div className="admin-table-scroll">
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>Field</th>
                      <th>From</th>
                      <th></th>
                      <th>To</th>
                      <th>Changed by</th>
                      <th>When</th>
                    </tr>
                  </thead>
                  <tbody>
                    {history.map((entry) => {
                      const [from, to] = formatHistoryPair(entry);
                      return (
                        <tr key={entry.id}>
                          <td className="admin-table__name">
                            {HISTORY_FIELD_LABELS[entry.field] ?? entry.field}
                          </td>
                          <td
                            className="admin-user-detail__history-value admin-user-detail__history-value--old"
                            title={entry.old_value}
                          >
                            {from}
                          </td>
                          <td
                            className="admin-user-detail__history-arrow"
                            aria-hidden="true"
                          >
                            →
                          </td>
                          <td
                            className="admin-user-detail__history-value admin-user-detail__history-value--new"
                            title={entry.new_value}
                          >
                            {to}
                          </td>
                          <td>
                            {entry.changed_by !== null ? (
                              <UsernameDisplay
                                userId={entry.changed_by}
                                username={
                                  entry.changed_by_username ||
                                  `#${entry.changed_by}`
                                }
                                noLink={!entry.changed_by_username}
                              />
                            ) : (
                              <span className="admin-muted">
                                {entry.changed_by_username || "deleted user"}
                              </span>
                            )}
                          </td>
                          <td className="admin-muted">
                            {timeAgo(entry.created_at)}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
              {history.length < historyTotal && (
                <div className="admin-user-detail__history-more">
                  <button
                    className="admin-btn admin-btn--ghost admin-btn--sm"
                    onClick={handleLoadMoreHistory}
                    disabled={loadingMoreHistory}
                  >
                    {loadingMoreHistory
                      ? "Loading…"
                      : `Show older edits (${historyTotal - history.length} more)`}
                  </button>
                </div>
              )}
            </>
          )}
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

      <ConfirmModal
        isOpen={revokingInviteId !== null}
        title="Revoke Invite"
        message="This permanently deletes the invite. The token will no longer work, even if it hasn't expired."
        confirmLabel="Revoke"
        danger
        loading={revokingInvite}
        onConfirm={handleRevokeInvite}
        onCancel={() => setRevokingInviteId(null)}
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
