import { useCallback, useEffect, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { ConfirmModal } from "@/components/modal/ConfirmModal";
import { formatBytes, timeAgo } from "@/utils/format";
import "./admin-ui.css";
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
  }, [toast]);

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
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title">Database Backups</h1>
          <p className="admin-page-header__desc">
            Backups are compressed <code>pg_dump</code> archives (custom
            format). Restore with <code>pg_restore</code>. Creating a backup can
            take a while on a large database — this page waits until it
            finishes.
          </p>
        </div>
        <div className="admin-page-header__actions">
          <button
            className="admin-btn admin-btn--primary"
            onClick={handleCreate}
            disabled={creating}
          >
            {creating ? "Creating backup..." : "Create Backup"}
          </button>
        </div>
      </div>

      {loading ? (
        <p>Loading backups...</p>
      ) : backups.length === 0 ? (
        <div className="admin-panel">
          <p className="admin-empty">No backups yet.</p>
        </div>
      ) : (
        <div className="admin-panel">
          <div className="admin-table-scroll">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>File</th>
                  <th>Size</th>
                  <th>Created</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {backups.map((backup) => (
                  <tr key={backup.name}>
                    <td className="admin-num">{backup.name}</td>
                    <td className="admin-num">{formatBytes(backup.size)}</td>
                    <td
                      className="admin-muted"
                      title={new Date(backup.created_at).toLocaleString()}
                    >
                      {timeAgo(backup.created_at)}
                    </td>
                    <td className="admin-table__actions">
                      <div className="admin-backups__actions">
                        <button
                          className="admin-btn admin-btn--ghost admin-btn--sm"
                          onClick={() => handleDownload(backup)}
                          disabled={downloading === backup.name}
                        >
                          {downloading === backup.name
                            ? "Downloading..."
                            : "Download"}
                        </button>
                        <button
                          className="admin-btn admin-btn--danger admin-btn--sm"
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
          </div>
        </div>
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
