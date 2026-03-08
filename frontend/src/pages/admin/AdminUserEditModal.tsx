import { useState } from "react";
import { Modal } from "@/components/modal/Modal";
import { Input } from "@/components/form";
import { Select } from "@/components/form";
import { Checkbox } from "@/components/form";

interface AdminUser {
  id: number;
  username: string;
  email: string;
  group_id: number;
  uploaded: number;
  downloaded: number;
  enabled: boolean;
  warned: boolean;
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
  const [username, setUsername] = useState(user.username);
  const [email, setEmail] = useState(user.email);
  const [groupId, setGroupId] = useState(String(user.group_id));
  const [uploaded, setUploaded] = useState(String(user.uploaded));
  const [downloaded, setDownloaded] = useState(String(user.downloaded));
  const [enabled, setEnabled] = useState(user.enabled);
  const [warned, setWarned] = useState(user.warned);
  const [invites, setInvites] = useState(String(user.invites));
  const [saving, setSaving] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    try {
      await onSave(user.id, {
        username,
        email,
        group_id: Number(groupId),
        uploaded: Number(uploaded),
        downloaded: Number(downloaded),
        enabled,
        warned,
        invites: Number(invites),
      });
      onClose();
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={`Edit ${user.username}`}>
      <form className="admin-users__modal-form" onSubmit={handleSubmit}>
        <Input
          label="Username"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
        />
        <Input
          label="Email"
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        <Select
          label="Group"
          options={groups}
          value={groupId}
          onChange={(e) => setGroupId(e.target.value)}
        />
        <div style={{ display: "flex", gap: "var(--space-md)" }}>
          <Input
            label="Uploaded (bytes)"
            type="number"
            value={uploaded}
            onChange={(e) => setUploaded(e.target.value)}
          />
          <Input
            label="Downloaded (bytes)"
            type="number"
            value={downloaded}
            onChange={(e) => setDownloaded(e.target.value)}
          />
        </div>
        <Input
          label="Invites"
          type="number"
          value={invites}
          onChange={(e) => setInvites(e.target.value)}
        />
        <div
          style={{ display: "flex", gap: "var(--space-lg)", flexWrap: "wrap" }}
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
        </div>
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
  );
}
