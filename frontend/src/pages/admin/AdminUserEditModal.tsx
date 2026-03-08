import { useState } from "react";
import { Modal } from "@/components/modal/Modal";
import { Input } from "@/components/form";
import { Select } from "@/components/form";
import { Checkbox } from "@/components/form";

interface AdminUser {
  id: number;
  username: string;
  group_id: number;
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
  onSave: (
    userId: number,
    data: {
      group_id?: number;
      enabled?: boolean;
      warned?: boolean;
      invites?: number;
    },
  ) => Promise<void>;
}

export function AdminUserEditModal({
  user,
  groups,
  isOpen,
  onClose,
  onSave,
}: AdminUserEditModalProps) {
  const [groupId, setGroupId] = useState(String(user.group_id));
  const [enabled, setEnabled] = useState(user.enabled);
  const [warned, setWarned] = useState(user.warned);
  const [invites, setInvites] = useState(user.invites);
  const [saving, setSaving] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    try {
      await onSave(user.id, {
        group_id: Number(groupId),
        enabled,
        warned,
        invites,
      });
      onClose();
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={`Edit ${user.username}`}>
      <form className="admin-users__modal-form" onSubmit={handleSubmit}>
        <Select
          label="Group"
          options={groups}
          value={groupId}
          onChange={(e) => setGroupId(e.target.value)}
        />
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
        <Input
          label="Invites"
          type="number"
          value={String(invites)}
          onChange={(e) => setInvites(Number(e.target.value))}
        />
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
