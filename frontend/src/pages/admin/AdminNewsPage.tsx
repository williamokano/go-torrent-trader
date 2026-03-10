import { useCallback, useEffect, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { timeAgo } from "@/utils/format";
import { Pagination } from "@/components/Pagination";
import type { AdminNewsArticle } from "@/types/news";
import "./admin-news.css";

const PER_PAGE = 25;

function StatusBadge({ published }: { published: boolean }) {
  return (
    <span
      className={`admin-news__status admin-news__status--${published ? "published" : "draft"}`}
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
      <h1>News</h1>

      <div className="admin-news__controls">
        <button className="admin-news__create-btn" onClick={openCreate}>
          Create Article
        </button>
      </div>

      {loading ? (
        <p>Loading...</p>
      ) : articles.length === 0 ? (
        <p className="admin-news__empty">No news articles found.</p>
      ) : (
        <>
          <table className="admin-news__table">
            <thead>
              <tr>
                <th>Title</th>
                <th>Author</th>
                <th>Status</th>
                <th>Date</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {articles.map((a) => (
                <tr key={a.id}>
                  <td>{a.title}</td>
                  <td>{a.author_name ?? "Unknown"}</td>
                  <td>
                    <StatusBadge published={a.published} />
                  </td>
                  <td
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
                  <td className="admin-news__actions">
                    <button
                      className="admin-news__edit-btn"
                      onClick={() => openEdit(a)}
                    >
                      Edit
                    </button>
                    <button
                      className="admin-news__delete-btn"
                      onClick={() => setDeletingId(a.id)}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {totalPages > 1 && (
            <Pagination
              currentPage={page}
              totalPages={totalPages}
              onPageChange={setPage}
            />
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
                <label htmlFor="news-body">Body</label>
                <textarea
                  id="news-body"
                  value={formBody}
                  onChange={(e) => setFormBody(e.target.value)}
                  required
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
                  className="admin-news__modal-cancel"
                  onClick={closeModal}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="admin-news__modal-submit"
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
                className="admin-news__modal-cancel"
                onClick={() => setDeletingId(null)}
              >
                Cancel
              </button>
              <button
                className="admin-news__modal-delete"
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
