import { useCallback, useEffect, useState } from "react";
import { Pagination } from "@/components/Pagination";
import { Textarea } from "@/components/form";
import { useToast } from "@/components/toast";
import { useAuth } from "@/features/auth";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { timeAgo } from "@/utils/format";

export interface Comment {
  id: number;
  user_id: number;
  username?: string;
  body: string;
  created_at: string;
  updated_at: string;
}

interface CommentsResponse {
  comments: Comment[];
  total: number;
  page: number;
  per_page: number;
}

interface CommentsSectionProps {
  torrentId: string;
}

const PER_PAGE = 10;

export function CommentsSection({ torrentId }: CommentsSectionProps) {
  const toast = useToast();
  const { user } = useAuth();

  const [comments, setComments] = useState<Comment[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [body, setBody] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const [editingId, setEditingId] = useState<number | null>(null);
  const [editBody, setEditBody] = useState("");
  const [editSubmitting, setEditSubmitting] = useState(false);

  const [deletingId, setDeletingId] = useState<number | null>(null);

  const totalPages = Math.max(1, Math.ceil(total / PER_PAGE));

  const fetchComments = useCallback(
    async (targetPage: number) => {
      setLoading(true);
      try {
        const token = getAccessToken();
        const baseUrl = getConfig().API_URL;
        const url = `${baseUrl}/api/v1/torrents/${encodeURIComponent(torrentId)}/comments?page=${targetPage}&per_page=${PER_PAGE}`;
        const response = await fetch(url, {
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        });

        if (!response.ok) {
          throw new Error("Failed to load comments");
        }

        const data: CommentsResponse = await response.json();
        setComments(data.comments ?? []);
        setTotal(data.total ?? 0);
      } catch {
        toast.error("Failed to load comments");
      } finally {
        setLoading(false);
      }
    },
    [torrentId, toast],
  );

  useEffect(() => {
    fetchComments(page);
  }, [page, fetchComments]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = body.trim();
    if (!trimmed || submitting) return;

    setSubmitting(true);
    try {
      const token = getAccessToken();
      if (!token) {
        toast.error("You must be logged in to comment");
        return;
      }

      const baseUrl = getConfig().API_URL;
      const response = await fetch(
        `${baseUrl}/api/v1/torrents/${encodeURIComponent(torrentId)}/comments`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify({ body: trimmed }),
        },
      );

      if (!response.ok) {
        const data = await response.json().catch(() => null);
        throw new Error(
          data?.error?.message ?? `Failed to post comment (${response.status})`,
        );
      }

      setBody("");
      toast.success("Comment posted");
      // Go to page 1 to see the newest comment
      if (page === 1) {
        await fetchComments(1);
      } else {
        setPage(1);
      }
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to post comment",
      );
    } finally {
      setSubmitting(false);
    }
  }

  async function handleEdit(commentId: number) {
    const trimmed = editBody.trim();
    if (!trimmed || editSubmitting) return;

    setEditSubmitting(true);
    try {
      const token = getAccessToken();
      if (!token) {
        toast.error("You must be logged in");
        return;
      }

      const baseUrl = getConfig().API_URL;
      const response = await fetch(`${baseUrl}/api/v1/comments/${commentId}`, {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ body: trimmed }),
      });

      if (!response.ok) {
        const data = await response.json().catch(() => null);
        throw new Error(
          data?.error?.message ?? `Failed to edit comment (${response.status})`,
        );
      }

      setEditingId(null);
      setEditBody("");
      toast.success("Comment updated");
      await fetchComments(page);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to edit comment",
      );
    } finally {
      setEditSubmitting(false);
    }
  }

  async function handleDelete(commentId: number) {
    setDeletingId(commentId);
    try {
      const token = getAccessToken();
      if (!token) {
        toast.error("You must be logged in");
        return;
      }

      const baseUrl = getConfig().API_URL;
      const response = await fetch(`${baseUrl}/api/v1/comments/${commentId}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });

      if (!response.ok) {
        const data = await response.json().catch(() => null);
        throw new Error(
          data?.error?.message ??
            `Failed to delete comment (${response.status})`,
        );
      }

      toast.success("Comment deleted");
      await fetchComments(page);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to delete comment",
      );
    } finally {
      setDeletingId(null);
    }
  }

  function startEditing(comment: Comment) {
    setEditingId(comment.id);
    setEditBody(comment.body);
  }

  function cancelEditing() {
    setEditingId(null);
    setEditBody("");
  }

  function canManageComment(comment: Comment): boolean {
    if (!user) return false;
    return user.isAdmin || user.id === comment.user_id;
  }

  return (
    <section className="comments-section" aria-label="Comments">
      <h2 className="comments-section__title">
        Comments
        {total > 0 && (
          <span className="comments-section__count">({total})</span>
        )}
      </h2>

      {user && (
        <form className="comments-section__form" onSubmit={handleSubmit}>
          <Textarea
            label="Add a comment"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={3}
            placeholder="Write your comment..."
          />
          <button
            type="submit"
            className="comments-section__submit"
            disabled={submitting || !body.trim()}
          >
            {submitting ? "Posting..." : "Post Comment"}
          </button>
        </form>
      )}

      {loading ? (
        <p className="comments-section__loading">Loading comments...</p>
      ) : comments.length === 0 ? (
        <p className="comments-section__empty">No comments yet.</p>
      ) : (
        <>
          <ul className="comments-section__list">
            {comments.map((comment) => (
              <li key={comment.id} className="comments-section__item">
                <div className="comments-section__meta">
                  <span className="comments-section__author">
                    {comment.username ?? `User #${comment.user_id}`}
                  </span>
                  <time
                    className="comments-section__time"
                    dateTime={comment.created_at}
                  >
                    {timeAgo(comment.created_at)}
                  </time>
                  {comment.updated_at !== comment.created_at && (
                    <span className="comments-section__edited">(edited)</span>
                  )}
                </div>

                {editingId === comment.id ? (
                  <div className="comments-section__edit-form">
                    <Textarea
                      label="Edit comment"
                      value={editBody}
                      onChange={(e) => setEditBody(e.target.value)}
                      rows={3}
                    />
                    <div className="comments-section__edit-actions">
                      <button
                        type="button"
                        className="comments-section__cancel-btn"
                        onClick={cancelEditing}
                      >
                        Cancel
                      </button>
                      <button
                        type="button"
                        className="comments-section__save-btn"
                        onClick={() => handleEdit(comment.id)}
                        disabled={editSubmitting || !editBody.trim()}
                      >
                        {editSubmitting ? "Saving..." : "Save"}
                      </button>
                    </div>
                  </div>
                ) : (
                  <p className="comments-section__body">{comment.body}</p>
                )}

                {canManageComment(comment) && editingId !== comment.id && (
                  <div className="comments-section__actions">
                    <button
                      type="button"
                      className="comments-section__edit-btn"
                      onClick={() => startEditing(comment)}
                    >
                      Edit
                    </button>
                    {user?.isAdmin && (
                      <button
                        type="button"
                        className="comments-section__delete-btn"
                        onClick={() => handleDelete(comment.id)}
                        disabled={deletingId === comment.id}
                      >
                        {deletingId === comment.id ? "Deleting..." : "Delete"}
                      </button>
                    )}
                  </div>
                )}
              </li>
            ))}
          </ul>

          <Pagination
            currentPage={page}
            totalPages={totalPages}
            onPageChange={setPage}
          />
        </>
      )}
    </section>
  );
}
