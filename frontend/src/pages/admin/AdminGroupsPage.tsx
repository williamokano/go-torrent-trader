import { useCallback, useEffect, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { Input, Checkbox } from "@/components/form";
import { Modal } from "@/components/modal/Modal";

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

const cellStyle: React.CSSProperties = {
  padding: "var(--space-xs) var(--space-sm)",
  borderBottom: "1px solid var(--color-border)",
};

const headStyle: React.CSSProperties = {
  ...cellStyle,
  textAlign: "left",
  color: "var(--color-text-muted)",
};

export function AdminGroupsPage() {
  const toast = useToast();
  const [groups, setGroups] = useState<Group[]>([]);
  const [loading, setLoading] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form, setForm] = useState<GroupFormData>(emptyForm);
  const [saving, setSaving] = useState(false);

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

  const handleDelete = async (group: Group) => {
    if (
      !window.confirm(
        `Delete the "${group.name}" group? This cannot be undone.`,
      )
    ) {
      return;
    }
    const token = getAccessToken();
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/groups/${group.id}`,
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
  };

  if (loading) return <p>Loading...</p>;

  return (
    <div>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          marginBottom: "var(--space-lg)",
        }}
      >
        <h1 style={{ fontSize: "var(--text-xl)", margin: 0 }}>Groups</h1>
        <button
          onClick={openCreateModal}
          style={{
            padding: "var(--space-xs) var(--space-md)",
            backgroundColor: "var(--color-accent)",
            color: "white",
            border: "none",
            borderRadius: "var(--radius-md)",
            cursor: "pointer",
            fontSize: "var(--text-sm)",
          }}
        >
          New Group
        </button>
      </div>

      <table
        style={{
          width: "100%",
          borderCollapse: "collapse",
          fontSize: "var(--text-sm)",
        }}
      >
        <thead>
          <tr>
            <th style={headStyle}>Name</th>
            <th style={headStyle}>Level</th>
            <th style={headStyle}>Color</th>
            {CAPABILITY_COLUMNS.map((col) => (
              <th
                key={col.key}
                style={{
                  ...headStyle,
                  textAlign: "center",
                  whiteSpace: "nowrap",
                }}
              >
                {col.label}
              </th>
            ))}
            <th style={{ ...headStyle, textAlign: "right" }}>Actions</th>
          </tr>
        </thead>
        <tbody>
          {groups.map((group) => (
            <tr key={group.id}>
              <td style={{ ...cellStyle, fontWeight: 600 }}>{group.name}</td>
              <td style={cellStyle}>{group.level}</td>
              <td style={cellStyle}>
                {group.color ? (
                  <span
                    style={{
                      display: "inline-block",
                      width: 16,
                      height: 16,
                      borderRadius: 4,
                      backgroundColor: group.color,
                      border: "1px solid var(--color-border)",
                      verticalAlign: "middle",
                    }}
                    title={group.color}
                  />
                ) : (
                  "-"
                )}
              </td>
              {CAPABILITY_COLUMNS.map((col) => (
                <td key={col.key} style={{ ...cellStyle, textAlign: "center" }}>
                  {group[col.key] ? "Y" : "N"}
                </td>
              ))}
              <td
                style={{
                  ...cellStyle,
                  textAlign: "right",
                  whiteSpace: "nowrap",
                }}
              >
                <button
                  onClick={() => openEditModal(group)}
                  style={{ marginRight: "var(--space-xs)", cursor: "pointer" }}
                >
                  Edit
                </button>
                <button
                  onClick={() => handleDelete(group)}
                  style={{
                    cursor: "pointer",
                    color: "var(--color-danger, #c0392b)",
                  }}
                >
                  Delete
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <Modal
        isOpen={modalOpen}
        onClose={closeModal}
        title={editingId ? "Edit Group" : "New Group"}
      >
        <div
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "var(--space-sm)",
          }}
        >
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
          <div
            style={{
              display: "flex",
              alignItems: "flex-end",
              gap: "var(--space-sm)",
            }}
          >
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
              value={
                /^#[0-9A-Fa-f]{6}$/.test(form.color) ? form.color : "#555555"
              }
              onChange={(e) => setForm({ ...form, color: e.target.value })}
              style={{
                width: 40,
                height: 34,
                padding: 0,
                border: "none",
                background: "none",
              }}
            />
          </div>

          <div
            style={{
              display: "grid",
              gridTemplateColumns: "1fr 1fr",
              gap: "var(--space-2xs, 4px) var(--space-md)",
              marginTop: "var(--space-xs)",
            }}
          >
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

          <div
            style={{
              display: "flex",
              justifyContent: "flex-end",
              gap: "var(--space-sm)",
              marginTop: "var(--space-md)",
            }}
          >
            <button onClick={closeModal} style={{ cursor: "pointer" }}>
              Cancel
            </button>
            <button
              onClick={handleSave}
              disabled={saving || !form.name.trim()}
              style={{
                padding: "var(--space-xs) var(--space-md)",
                backgroundColor: "var(--color-accent)",
                color: "white",
                border: "none",
                borderRadius: "var(--radius-md)",
                cursor: saving || !form.name.trim() ? "not-allowed" : "pointer",
                opacity: saving || !form.name.trim() ? 0.5 : 1,
              }}
            >
              {saving ? "Saving..." : "Save"}
            </button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
