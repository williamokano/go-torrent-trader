import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useAuth } from "@/features/auth/useAuth";
import "@/pages/admin/admin-ui.css";
import "./metadata-issues.css";

interface MissingField {
  key: string;
  label: string;
}

interface MetadataIssue {
  torrent_id: number;
  torrent_name: string;
  category_id: number;
  category_name: string;
  uploader_id: number;
  uploader_name: string;
  anonymous: boolean;
  missing_fields: MissingField[];
}

type Scope = "mine" | "all";

/**
 * Reports torrents whose stored metadata is missing a field their category now
 * requires (e.g. a required field added after upload). Any user sees their own
 * uploads; admins can toggle to the site-wide report.
 */
export function MetadataIssuesPage() {
  const { user } = useAuth();
  const isAdmin = user?.isAdmin ?? false;
  const [scope, setScope] = useState<Scope>(isAdmin ? "all" : "mine");
  const [issues, setIssues] = useState<MetadataIssue[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (s: Scope) => {
    setLoading(true);
    setError(null);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/torrents/metadata-issues?scope=${s}`,
        { headers: { Authorization: `Bearer ${token}` } },
      );
      if (!res.ok) {
        const data = await res.json().catch(() => null);
        setError(data?.error?.message ?? "Failed to load the report");
        setIssues([]);
      } else {
        const data = await res.json();
        setIssues(data.issues ?? []);
      }
    } catch {
      setError("Failed to load the report");
      setIssues([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load(scope);
  }, [load, scope]);

  const showUploader = isAdmin && scope === "all";

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title">Metadata Issues</h1>
          <p className="admin-page-header__desc">
            Torrents missing a metadata field their category now requires —
            usually a field added after the torrent was uploaded. Edit a torrent
            to fill it in.
          </p>
        </div>
        {isAdmin && (
          <div className="admin-page-header__actions">
            <div
              className="metadata-issues__scope"
              role="group"
              aria-label="Scope"
            >
              <button
                type="button"
                className={`admin-btn admin-btn--sm ${scope === "all" ? "admin-btn--primary" : "admin-btn--ghost"}`}
                aria-pressed={scope === "all"}
                onClick={() => setScope("all")}
              >
                All uploaders
              </button>{" "}
              <button
                type="button"
                className={`admin-btn admin-btn--sm ${scope === "mine" ? "admin-btn--primary" : "admin-btn--ghost"}`}
                aria-pressed={scope === "mine"}
                onClick={() => setScope("mine")}
              >
                Only mine
              </button>
            </div>
          </div>
        )}
      </div>

      {loading ? (
        <p>Loading...</p>
      ) : error ? (
        <p className="metadata-issues__error">{error}</p>
      ) : issues.length === 0 ? (
        <div className="admin-panel">
          <p className="admin-empty">
            🎉 No torrents are missing required metadata.
          </p>
        </div>
      ) : (
        <div className="admin-panel">
          <div className="admin-table-scroll">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>Torrent</th>
                  <th>Category</th>
                  {showUploader && <th>Uploader</th>}
                  <th>Missing fields</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {issues.map((issue) => (
                  <tr key={issue.torrent_id}>
                    <td className="admin-table__name">
                      <Link to={`/torrent/${issue.torrent_id}`}>
                        {issue.torrent_name}
                      </Link>
                    </td>
                    <td className="admin-muted">{issue.category_name}</td>
                    {showUploader && (
                      <td className="admin-muted">{issue.uploader_name}</td>
                    )}
                    <td>
                      {issue.missing_fields.map((f) => (
                        <span key={f.key} className="metadata-issues__badge">
                          {f.label}
                        </span>
                      ))}
                    </td>
                    <td className="admin-table__actions">
                      <Link
                        className="admin-btn admin-btn--ghost admin-btn--sm"
                        to={`/torrent/${issue.torrent_id}/edit`}
                      >
                        Fix
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
