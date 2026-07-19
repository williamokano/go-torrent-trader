import { useCallback, useEffect, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { Input, Checkbox, Button } from "@/components/form";
import { Modal } from "@/components/modal/Modal";
import { ConfirmModal } from "@/components/modal/ConfirmModal";
import "./admin-ui.css";

interface Group {
  id: number;
  name: string;
  slug: string;
  level: number;
  color: string | null;
  can_upload: boolean;
  can_download: boolean;
  can_invite: boolean;
  can_comment: boolean;
  can_forum: boolean;
  is_admin: boolean;
  is_moderator: boolean;
  is_immune: boolean;
}

type CapabilityKey =
  | "can_upload"
  | "can_download"
  | "can_invite"
  | "can_comment"
  | "can_forum"
  | "is_admin"
  | "is_moderator"
  | "is_immune";

const CAPABILITY_COLUMNS: { key: CapabilityKey; label: string }[] = [
  { key: "can_upload", label: "Upload" },
  { key: "can_download", label: "Download" },
  { key: "can_invite", label: "Invite" },
  { key: "can_comment", label: "Comment" },
  { key: "can_forum", label: "Forum" },
  { key: "is_admin", label: "Admin" },
  { key: "is_moderator", label: "Moderator" },
  { key: "is_immune", label: "Immune" },
];

interface GroupFormData {
  name: string;
  slug: string;
  level: string;
  color: string;
  can_upload: boolean;
  can_download: boolean;
  can_invite: boolean;
  can_comment: boolean;
  can_forum: boolean;
  is_admin: boolean;
  is_moderator: boolean;
  is_immune: boolean;
}

const emptyForm: GroupFormData = {
  name: "",
  slug: "",
  level: "0",
  color: "",
  can_upload: true,
  can_download: true,
  can_invite: false,
  can_comment: true,
  can_forum: true,
  is_admin: false,
  is_moderator: false,
  is_immune: false,
};

export function AdminGroupsPage() {
  const toast = useToast();
  const [groups, setGroups] = useState<Group[]>([]);
  const [loading, setLoading] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form, setForm] = useState<GroupFormData>(emptyForm);
  const [saving, setSaving] = useState(false);
  const [deletingGroup, setDeletingGroup] = useState<Group | null>(null);
  const [deleting, setDeleting] = useState(false);

  const fetchGroups = useCallback(async () => {
    const token = getAccessToken();
    try {
      const res = await fetch(`${getConfig().API_URL}/api/v1/admin/groups`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setGroups(data.groups ?? []);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchGroups();
  }, [fetchGroups]);

  const openCreateModal = () => {
    setEditingId(null);
    setForm(emptyForm);
    setModalOpen(true);
  };

  const openEditModal = (group: Group) => {
    setEditingId(group.id);
    setForm({
      name: group.name,
      slug: group.slug,
      level: String(group.level),
      color: group.color ?? "",
      can_upload: group.can_upload,
      can_download: group.can_download,
      can_invite: group.can_invite,
      can_comment: group.can_comment,
      can_forum: group.can_forum,
      is_admin: group.is_admin,
      is_moderator: group.is_moderator,
      is_immune: group.is_immune,
    });
    setModalOpen(true);
  };

  const closeModal = () => {
    setModalOpen(false);
    setEditingId(null);
    setForm(emptyForm);
  };

  const handleSave = async () => {
    setSaving(true);
    const token = getAccessToken();
    const payload = {
      name: form.name,
      slug: form.slug,
      level: Number(form.level) || 0,
      color: form.color.trim() || null,
      can_upload: form.can_upload,
      can_download: form.can_download,
      can_invite: form.can_invite,
      can_comment: form.can_comment,
      can_forum: form.can_forum,
      is_admin: form.is_admin,
      is_moderator: form.is_moderator,
      is_immune: form.is_immune,
    };

    try {
      const url = editingId
        ? `${getConfig().API_URL}/api/v1/admin/groups/${editingId}`
        : `${getConfig().API_URL}/api/v1/admin/groups`;
      const res = await fetch(url, {
        method: editingId ? "PUT" : "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(payload),
      });

      if (res.ok) {
        toast.success(editingId ? "Group updated" : "Group created");
        closeModal();
        fetchGroups();
      } else {
        const data = await res.json().catch(() => null);
        toast.error(data?.error?.message ?? "Failed to save group");
      }
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!deletingGroup) return;
    setDeleting(true);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/groups/${deletingGroup.id}`,
        {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        },
      );
      if (res.ok) {
        toast.success("Group deleted");
        fetchGroups();
      } else {
        const data = await res.json().catch(() => null);
        toast.error(data?.error?.message ?? "Failed to delete group");
      }
    } finally {
      setDeleting(false);
      setDeletingGroup(null);
    }
  };

  if (loading) return <p>Loading…</p>;

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title">Groups</h1>
          <p className="admin-page-header__desc">
            Permission classes and their capabilities. Level orders classes and
            drives the promotion ladder.
          </p>
        </div>
        <div className="admin-page-header__actions">
          <Button variant="primary" onClick={openCreateModal}>
            New group
          </Button>
        </div>
      </div>

      <div className="admin-panel">
        <div className="admin-table-scroll">
          <table className="admin-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Level</th>
                <th>Color</th>
                {CAPABILITY_COLUMNS.map((col) => (
                  <th key={col.key} className="admin-table__center">
                    {col.label}
                  </th>
                ))}
                <th className="admin-table__actions">Actions</th>
              </tr>
            </thead>
            <tbody>
              {groups.map((group) => (
                <tr key={group.id}>
                  <td className="admin-table__name">{group.name}</td>
                  <td className="admin-num">{group.level}</td>
                  <td>
                    {group.color ? (
                      <span
                        className="admin-swatch"
                        style={{ backgroundColor: group.color }}
                        title={group.color}
                      />
                    ) : (
                      <span className="admin-muted">—</span>
                    )}
                  </td>
                  {CAPABILITY_COLUMNS.map((col) => (
                    <td key={col.key} className="admin-table__center">
                      {group[col.key] ? (
                        <span className="admin-yes" aria-label="yes">
                          ✓
                        </span>
                      ) : (
                        <span className="admin-muted" aria-label="no">
                          —
                        </span>
                      )}
                    </td>
                  ))}
                  <td className="admin-table__actions">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => openEditModal(group)}
                    >
                      Edit
                    </Button>{" "}
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => setDeletingGroup(group)}
                    >
                      Delete
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <Modal
        isOpen={modalOpen}
        onClose={closeModal}
        title={editingId ? "Edit group" : "New group"}
        closeOnEscape={false}
      >
        <div className="admin-modal-form">
          <Input
            label="Name"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
          />
          <Input
            label="Slug"
            value={form.slug}
            placeholder="Auto-generated from name if empty"
            onChange={(e) => setForm({ ...form, slug: e.target.value })}
          />
          <Input
            label="Level"
            type="number"
            value={form.level}
            onChange={(e) => setForm({ ...form, level: e.target.value })}
          />
          <div className="admin-color-row">
            <div style={{ flex: 1 }}>
              <Input
                label="Color (hex)"
                value={form.color}
                placeholder="#55AA88"
                onChange={(e) => setForm({ ...form, color: e.target.value })}
              />
            </div>
            <input
              type="color"
              aria-label="Color picker"
              className="admin-color-row__picker"
              value={
                /^#[0-9A-Fa-f]{6}$/.test(form.color) ? form.color : "#555555"
              }
              onChange={(e) => setForm({ ...form, color: e.target.value })}
            />
          </div>

          <div className="admin-modal-form__caps">
            {CAPABILITY_COLUMNS.map((col) => (
              <Checkbox
                key={col.key}
                label={col.label}
                checked={form[col.key]}
                onChange={(e) =>
                  setForm({ ...form, [col.key]: e.target.checked })
                }
              />
            ))}
          </div>

          <div className="admin-modal-form__actions">
            <Button variant="ghost" onClick={closeModal}>
              Cancel
            </Button>
            <Button
              variant="primary"
              onClick={handleSave}
              loading={saving}
              disabled={!form.name.trim()}
            >
              Save
            </Button>
          </div>
        </div>
      </Modal>

      <ConfirmModal
        isOpen={deletingGroup !== null}
        title="Delete Group"
        message={`Delete the "${deletingGroup?.name}" group? This cannot be undone.`}
        confirmLabel="Delete"
        danger
        loading={deleting}
        onConfirm={handleDelete}
        onCancel={() => setDeletingGroup(null)}
      />
    </div>
  );
}
