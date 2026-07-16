import { useCallback, useEffect, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { timeAgo } from "@/utils/format";
import { Pagination } from "@/components/Pagination";
import { MarkdownEditor } from "@/components/MarkdownEditor";
import type { AdminNewsArticle } from "@/types/news";
import "./admin-ui.css";
import "./admin-news.css";

const PER_PAGE = 25;

function StatusBadge({ published }: { published: boolean }) {
  return (
    <span
      className={`admin-badge ${published ? "admin-badge--ok" : "admin-badge--muted"}`}
    >
      {published ? "Published" : "Draft"}
    </span>
  );
}

export function AdminNewsPage() {
  const toast = useToast();

  const [articles, setArticles] = useState<AdminNewsArticle[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);

  // Create/Edit modal
  const [showModal, setShowModal] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [formTitle, setFormTitle] = useState("");
  const [formBody, setFormBody] = useState("");
  const [formPublished, setFormPublished] = useState(false);
  const [saving, setSaving] = useState(false);

  // Delete confirmation
  const [deletingId, setDeletingId] = useState<number | null>(null);
  const [deleting, setDeleting] = useState(false);

  const fetchArticles = useCallback(async () => {
    setLoading(true);
    const token = getAccessToken();
    const params = new URLSearchParams();
    params.set("page", String(page));
    params.set("per_page", String(PER_PAGE));

    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/news?${params}`,
        { headers: { Authorization: `Bearer ${token}` } },
      );
      if (res.ok) {
        const data = await res.json();
        setArticles(data.articles ?? []);
        setTotal(data.total ?? 0);
      }
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    fetchArticles();
  }, [fetchArticles]);

  const openCreate = () => {
    setEditingId(null);
    setFormTitle("");
    setFormBody("");
    setFormPublished(false);
    setShowModal(true);
  };

  const openEdit = (article: AdminNewsArticle) => {
    setEditingId(article.id);
    setFormTitle(article.title);
    setFormBody(article.body);
    setFormPublished(article.published);
    setShowModal(true);
  };

  const closeModal = () => {
    setShowModal(false);
    setEditingId(null);
    setFormTitle("");
    setFormBody("");
    setFormPublished(false);
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!formTitle.trim() || !formBody.trim()) return;

    setSaving(true);
    const token = getAccessToken();
    const body = JSON.stringify({
      title: formTitle.trim(),
      body: formBody.trim(),
      published: formPublished,
    });

    try {
      const url = editingId
        ? `${getConfig().API_URL}/api/v1/admin/news/${editingId}`
        : `${getConfig().API_URL}/api/v1/admin/news`;
      const method = editingId ? "PUT" : "POST";

      const res = await fetch(url, {
        method,
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body,
      });

      if (res.ok) {
        toast.success(editingId ? "Article updated" : "Article created");
        closeModal();
        fetchArticles();
      } else {
        const data = await res.json();
        toast.error(data?.error?.message ?? "Failed to save article");
      }
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (deletingId === null) return;

    setDeleting(true);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/news/${deletingId}`,
        {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        },
      );

      if (res.ok) {
        toast.success("Article deleted");
        setDeletingId(null);
        fetchArticles();
      } else {
        const data = await res.json();
        toast.error(data?.error?.message ?? "Failed to delete article");
      }
    } finally {
      setDeleting(false);
    }
  };

  const totalPages = Math.ceil(total / PER_PAGE);

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title">News</h1>
          <p className="admin-page-header__desc">
            Write and publish announcements for the home page.
          </p>
        </div>
        <div className="admin-page-header__actions">
          <button className="admin-btn admin-btn--primary" onClick={openCreate}>
            Create Article
          </button>
        </div>
      </div>

      {loading ? (
        <p>Loading...</p>
      ) : articles.length === 0 ? (
        <div className="admin-panel">
          <p className="admin-empty">No news articles found.</p>
        </div>
      ) : (
        <>
          <div className="admin-panel">
            <div className="admin-table-scroll">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>Title</th>
                    <th>Author</th>
                    <th>Status</th>
                    <th>Date</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {articles.map((a) => (
                    <tr key={a.id}>
                      <td className="admin-table__name">{a.title}</td>
                      <td>{a.author_name ?? "Unknown"}</td>
                      <td>
                        <StatusBadge published={a.published} />
                      </td>
                      <td
                        className="admin-muted"
                        title={
                          a.updated_at !== a.created_at
                            ? `Updated ${timeAgo(a.updated_at)}`
                            : undefined
                        }
                      >
                        {timeAgo(a.created_at)}
                        {a.updated_at !== a.created_at && (
                          <span className="admin-news__edited"> (edited)</span>
                        )}
                      </td>
                      <td className="admin-table__actions">
                        <button
                          className="admin-btn admin-btn--ghost admin-btn--sm"
                          onClick={() => openEdit(a)}
                        >
                          Edit
                        </button>{" "}
                        <button
                          className="admin-btn admin-btn--danger admin-btn--sm"
                          onClick={() => setDeletingId(a.id)}
                        >
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {totalPages > 1 && (
            <div style={{ marginTop: "var(--space-md)" }}>
              <Pagination
                currentPage={page}
                totalPages={totalPages}
                onPageChange={setPage}
              />
            </div>
          )}
        </>
      )}

      {/* Create/Edit Modal */}
      {showModal && (
        <div className="admin-news__modal-overlay" onClick={closeModal}>
          <div
            className="admin-news__modal"
            onClick={(e) => e.stopPropagation()}
          >
            <h3>{editingId ? "Edit Article" : "Create Article"}</h3>
            <form onSubmit={handleSave}>
              <div className="admin-news__modal-field">
                <label htmlFor="news-title">Title</label>
                <input
                  id="news-title"
                  type="text"
                  value={formTitle}
                  onChange={(e) => setFormTitle(e.target.value)}
                  required
                  placeholder="Article title"
                />
              </div>
              <div className="admin-news__modal-field">
                <MarkdownEditor
                  id="news-body"
                  label="Body"
                  value={formBody}
                  onChange={setFormBody}
                  placeholder="Article content..."
                  rows={8}
                />
              </div>
              <div className="admin-news__modal-checkbox">
                <label>
                  <input
                    type="checkbox"
                    checked={formPublished}
                    onChange={(e) => setFormPublished(e.target.checked)}
                  />
                  Published
                </label>
              </div>
              <div className="admin-news__modal-actions">
                <button
                  type="button"
                  className="admin-btn admin-btn--ghost"
                  onClick={closeModal}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="admin-btn admin-btn--primary"
                  disabled={saving || !formTitle.trim() || !formBody.trim()}
                >
                  {saving ? "Saving..." : editingId ? "Update" : "Create"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {deletingId !== null && (
        <div
          className="admin-news__modal-overlay"
          onClick={() => setDeletingId(null)}
        >
          <div
            className="admin-news__modal"
            onClick={(e) => e.stopPropagation()}
          >
            <h3>Delete Article</h3>
            <p>
              Are you sure you want to delete this article? This cannot be
              undone.
            </p>
            <div className="admin-news__modal-actions">
              <button
                type="button"
                className="admin-btn admin-btn--ghost"
                onClick={() => setDeletingId(null)}
              >
                Cancel
              </button>
              <button
                className="admin-btn admin-btn--danger"
                onClick={handleDelete}
                disabled={deleting}
              >
                {deleting ? "Deleting..." : "Delete"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
