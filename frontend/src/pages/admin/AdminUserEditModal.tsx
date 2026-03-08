import { useState } from "react";
import { Modal } from "@/components/modal/Modal";
import { Input } from "@/components/form";
import { Select } from "@/components/form";
import { Checkbox } from "@/components/form";
import { Textarea } from "@/components/form";

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

  return (
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
