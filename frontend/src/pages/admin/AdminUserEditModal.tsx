import { useState } from "react";
import { Modal } from "@/components/modal/Modal";
import { ConfirmModal } from "@/components/modal/ConfirmModal";
import { Input } from "@/components/form";
import { Select } from "@/components/form";
import { Checkbox } from "@/components/form";
import { Textarea } from "@/components/form";
import { useToast } from "@/components/toast";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";

interface AdminUser {
  id: number;
  username: string;
  email: string;
  group_id: number;
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
}

interface GroupOption {
  value: string;
  label: string;
}

interface AdminUserEditModalProps {
  user: AdminUser;
  groups: GroupOption[];
  isOpen: boolean;
  onClose: () => void;
  onSave: (userId: number, data: Record<string, unknown>) => Promise<void>;
}

export function AdminUserEditModal({
  user,
  groups,
  isOpen,
  onClose,
  onSave,
}: AdminUserEditModalProps) {
  const toast = useToast();
  const [username, setUsername] = useState(user.username);
  const [email, setEmail] = useState(user.email);
  const [avatar, setAvatar] = useState(user.avatar ?? "");
  const [title, setTitle] = useState(user.title ?? "");
  const [info, setInfo] = useState(user.info ?? "");
  const [groupId, setGroupId] = useState(String(user.group_id));
  const [uploaded, setUploaded] = useState(String(user.uploaded));
  const [downloaded, setDownloaded] = useState(String(user.downloaded));
  const [invites, setInvites] = useState(String(user.invites));
  const [enabled, setEnabled] = useState(user.enabled);
  const [warned, setWarned] = useState(user.warned);
  const [donor, setDonor] = useState(user.donor);
  const [parked, setParked] = useState(user.parked);
  const [saving, setSaving] = useState(false);

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

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    try {
      await onSave(user.id, {
        username,
        email,
        avatar: avatar || null,
        title: title || null,
        info: info || null,
        group_id: Number(groupId),
        uploaded: Number(uploaded),
        downloaded: Number(downloaded),
        invites: Number(invites),
        enabled,
        warned,
        donor,
        parked,
      });
      onClose();
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
        `${getConfig().API_URL}/api/v1/admin/users/${user.id}/reset-password`,
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
        toast.success(`Password reset for ${user.username}`);
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
        `${getConfig().API_URL}/api/v1/admin/users/${user.id}/reset-passkey`,
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
        toast.success(`Passkey reset for ${user.username}`);
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

  return (
    <>
      <Modal isOpen={isOpen} onClose={onClose} title={`Edit ${user.username}`}>
        <form className="admin-users__modal-form" onSubmit={handleSubmit}>
          <div style={{ display: "flex", gap: "var(--space-md)" }}>
            <div style={{ flex: 1 }}>
              <Input
                label="Username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
            </div>
            <div style={{ flex: 1 }}>
              <Input
                label="Email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </div>
          </div>
          <Select
            label="Group"
            options={groups}
            value={groupId}
            onChange={(e) => setGroupId(e.target.value)}
          />
          <Input
            label="Avatar URL"
            value={avatar}
            onChange={(e) => setAvatar(e.target.value)}
            placeholder="https://..."
          />
          <Input
            label="Title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
          <Textarea
            label="Info / Bio"
            value={info}
            onChange={(e) => setInfo(e.target.value)}
          />
          <div style={{ display: "flex", gap: "var(--space-md)" }}>
            <div style={{ flex: 1 }}>
              <Input
                label="Uploaded (bytes)"
                type="number"
                value={uploaded}
                onChange={(e) => setUploaded(e.target.value)}
              />
            </div>
            <div style={{ flex: 1 }}>
              <Input
                label="Downloaded (bytes)"
                type="number"
                value={downloaded}
                onChange={(e) => setDownloaded(e.target.value)}
              />
            </div>
            <div style={{ flex: 1 }}>
              <Input
                label="Invites"
                type="number"
                value={invites}
                onChange={(e) => setInvites(e.target.value)}
              />
            </div>
          </div>
          <div
            style={{
              display: "flex",
              gap: "var(--space-lg)",
              flexWrap: "wrap",
            }}
          >
            <Checkbox
              label="Enabled"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
            />
            <Checkbox
              label="Warned"
              checked={warned}
              onChange={(e) => setWarned(e.target.checked)}
            />
            <Checkbox
              label="Donor"
              checked={donor}
              onChange={(e) => setDonor(e.target.checked)}
            />
            <Checkbox
              label="Parked"
              checked={parked}
              onChange={(e) => setParked(e.target.checked)}
            />
          </div>

          {/* Reset actions */}
          <div className="admin-users__reset-actions">
            <button
              type="button"
              className="admin-users__reset-btn"
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
              className="admin-users__reset-btn admin-users__reset-btn--danger"
              onClick={() => {
                setGeneratedPasskey(null);
                setShowPasskeyConfirm(true);
              }}
            >
              Reset Passkey
            </button>
          </div>

          {/* Show generated password */}
          {generatedPassword && (
            <div className="admin-users__generated-value">
              <label>New Password:</label>
              <div className="admin-users__copyable">
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

          {/* Show generated passkey */}
          {generatedPasskey && (
            <div className="admin-users__generated-value">
              <label>New Passkey:</label>
              <div className="admin-users__copyable">
                <code>{generatedPasskey}</code>
                <button
                  type="button"
                  onClick={() => handleCopy(generatedPasskey)}
                >
                  Copy
                </button>
              </div>
            </div>
          )}

          <div className="admin-users__modal-actions">
            <button
              type="button"
              className="admin-users__cancel-btn"
              onClick={onClose}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="admin-users__save-btn"
              disabled={saving}
            >
              {saving ? "Saving..." : "Save"}
            </button>
          </div>
        </form>
      </Modal>

      {/* Reset Password Modal */}
      <Modal
        isOpen={showPasswordModal}
        onClose={() => setShowPasswordModal(false)}
        title={`Reset Password for ${user.username}`}
      >
        <div className="admin-users__modal-form">
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
          <div className="admin-users__modal-actions">
            <button
              type="button"
              className="admin-users__cancel-btn"
              onClick={() => setShowPasswordModal(false)}
            >
              Cancel
            </button>
            <button
              type="button"
              className="admin-users__save-btn"
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
    </>
  );
}
