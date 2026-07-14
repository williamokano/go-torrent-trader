import { useCallback, useEffect, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { ConfirmModal } from "@/components/modal/ConfirmModal";
import { formatBytes, timeAgo } from "@/utils/format";
import "./admin-backups.css";

interface Backup {
  name: string;
  size: number;
  created_at: string;
}

export function AdminBackupsPage() {
  const toast = useToast();
  const [backups, setBackups] = useState<Backup[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [downloading, setDownloading] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Backup | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);

  const fetchBackups = useCallback(async () => {
    setLoading(true);
    try {
      const token = getAccessToken();
      const res = await fetch(`${getConfig().API_URL}/api/v1/admin/backups`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (!res.ok) {
        toast.error("Failed to load backups");
        return;
      }
      const data = await res.json();
      setBackups(data.backups ?? []);
    } catch {
      toast.error("Failed to load backups");
    } finally {
      setLoading(false);
    }
    // `toast` is intentionally not a dependency: the toast context value is a
    // fresh object on every provider render, so depending on it would re-run the
    // effect below on every toast — including the error toast this raises, which
    // would spin into a refetch loop. Same as the other admin pages.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    fetchBackups();
  }, [fetchBackups]);

  const handleCreate = async () => {
    setCreating(true);
    try {
      const token = getAccessToken();
      const res = await fetch(`${getConfig().API_URL}/api/v1/admin/backups`, {
        method: "POST",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        toast.error(body?.error?.message ?? "Failed to create backup");
        return;
      }
      toast.success("Backup created");
      await fetchBackups();
    } catch {
      toast.error("Failed to create backup");
    } finally {
      setCreating(false);
    }
  };

  const handleDownload = async (backup: Backup) => {
    setDownloading(backup.name);
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/backups/${encodeURIComponent(backup.name)}/download`,
        { headers: token ? { Authorization: `Bearer ${token}` } : {} },
      );
      if (!res.ok) {
        toast.error("Failed to download backup");
        return;
      }
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = backup.name;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch {
      toast.error("Failed to download backup");
    } finally {
      setDownloading(null);
    }
  };

  const handleDeleteConfirm = async () => {
    if (!deleteTarget) return;
    setDeleteLoading(true);
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/backups/${encodeURIComponent(deleteTarget.name)}`,
        {
          method: "DELETE",
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        },
      );
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        toast.error(body?.error?.message ?? "Failed to delete backup");
        return;
      }
      toast.success("Backup deleted");
      setDeleteTarget(null);
      await fetchBackups();
    } catch {
      toast.error("Failed to delete backup");
    } finally {
      setDeleteLoading(false);
    }
  };

  return (
    <div>
      <div className="admin-backups__header">
        <h1>Database Backups</h1>
        <button
          className="admin-backups__create-btn"
          onClick={handleCreate}
          disabled={creating}
        >
          {creating ? "Creating backup..." : "Create Backup"}
        </button>
      </div>

      <p className="admin-backups__hint">
        Backups are compressed <code>pg_dump</code> archives (custom format).
        Restore with <code>pg_restore</code>. Creating a backup can take a while
        on a large database — this page waits until it finishes.
      </p>

      {loading ? (
        <p>Loading backups...</p>
      ) : backups.length === 0 ? (
        <p className="admin-backups__empty">No backups yet.</p>
      ) : (
        <table className="admin-backups__table">
          <thead>
            <tr>
              <th>File</th>
              <th>Size</th>
              <th>Created</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {backups.map((backup) => (
              <tr key={backup.name}>
                <td className="admin-backups__name">{backup.name}</td>
                <td>{formatBytes(backup.size)}</td>
                <td title={new Date(backup.created_at).toLocaleString()}>
                  {timeAgo(backup.created_at)}
                </td>
                <td>
                  <div className="admin-backups__actions">
                    <button
                      className="admin-backups__action-btn"
                      onClick={() => handleDownload(backup)}
                      disabled={downloading === backup.name}
                    >
                      {downloading === backup.name
                        ? "Downloading..."
                        : "Download"}
                    </button>
                    <button
                      className="admin-backups__action-btn admin-backups__action-btn--danger"
                      onClick={() => setDeleteTarget(backup)}
                    >
                      Delete
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <ConfirmModal
        isOpen={deleteTarget !== null}
        title="Delete Backup"
        message={
          deleteTarget
            ? `Delete backup '${deleteTarget.name}'? This cannot be undone.`
            : ""
        }
        confirmLabel="Delete"
        danger
        loading={deleteLoading}
        onConfirm={handleDeleteConfirm}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
}
