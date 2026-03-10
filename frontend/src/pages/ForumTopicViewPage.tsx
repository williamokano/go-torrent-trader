import { useCallback, useEffect, useState } from "react";
import {
  Link,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useAuth } from "@/features/auth";
import { timeAgo } from "@/utils/format";
import { UsernameDisplay } from "@/components/UsernameDisplay";
import { MarkdownRenderer } from "@/components/MarkdownRenderer";
import { Pagination } from "@/components/Pagination";
import { Modal, ConfirmModal } from "@/components/modal";
import "./forums.css";

interface TopicData {
  id: number;
  forum_id: number;
  user_id: number;
  username: string;
  title: string;
  pinned: boolean;
  locked: boolean;
  post_count: number;
  view_count: number;
  forum_name: string;
  created_at: string;
}

interface ForumOption {
  id: number;
  name: string;
}

interface PostData {
  id: number;
  topic_id: number;
  user_id: number;
  username: string;
  avatar?: string;
  group_name: string;
  body: string;
  reply_to_post_id?: number;
  edited_at?: string;
  edited_by?: number;
  created_at: string;
  user_created_at: string;
  user_post_count: number;
}

const PER_PAGE = 25;

export function ForumTopicViewPage() {
  const { id } = useParams<{ id: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();
  const { user } = useAuth();

  const page = Math.max(1, Number(searchParams.get("page")) || 1);

  const [topic, setTopic] = useState<TopicData | null>(null);
  const [posts, setPosts] = useState<PostData[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [replyBody, setReplyBody] = useState("");
  const [replyToPostId, setReplyToPostId] = useState<number | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const [editingPostId, setEditingPostId] = useState<number | null>(null);
  const [editBody, setEditBody] = useState("");
  const [editError, setEditError] = useState<string | null>(null);
  const [editSaving, setEditSaving] = useState(false);

  const [deletePostId, setDeletePostId] = useState<number | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  // Moderation state
  const [showRenameModal, setShowRenameModal] = useState(false);
  const [renameTitle, setRenameTitle] = useState("");
  const [showMoveModal, setShowMoveModal] = useState(false);
  const [forums, setForums] = useState<ForumOption[]>([]);
  const [selectedForumId, setSelectedForumId] = useState<number | null>(null);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [modLoading, setModLoading] = useState(false);

  const fetchTopic = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const token = getAccessToken();
      const params = new URLSearchParams();
      params.set("page", String(page));
      params.set("per_page", String(PER_PAGE));

      const res = await fetch(
        `${getConfig().API_URL}/api/v1/forums/topics/${id}?${params.toString()}`,
        { headers: token ? { Authorization: `Bearer ${token}` } : {} },
      );
      if (res.status === 403) {
        setError("You do not have access to this topic.");
        return;
      }
      if (!res.ok) throw new Error("Failed to load topic");
      const data = await res.json();
      setTopic(data.topic ?? null);
      setPosts(data.posts ?? []);
      setTotal(data.total ?? 0);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, [id, page]);

  useEffect(() => {
    fetchTopic();
  }, [fetchTopic]);

  const handlePageChange = (newPage: number) => {
    const params = new URLSearchParams(searchParams);
    params.set("page", String(newPage));
    setSearchParams(params);
  };

  const handleQuote = (post: PostData) => {
    const quotedText = `> **${post.username}** wrote:\n> ${post.body.split("\n").join("\n> ")}\n\n`;
    setReplyBody((prev) => prev + quotedText);
    setReplyToPostId(post.id);

    // Scroll to reply form
    const replyForm = document.querySelector(".forum-reply");
    if (replyForm) replyForm.scrollIntoView({ behavior: "smooth" });
  };

  const handleSubmitReply = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!replyBody.trim()) return;

    setSubmitting(true);
    setSubmitError(null);

    try {
      const token = getAccessToken();
      const payload: Record<string, unknown> = { body: replyBody };
      if (replyToPostId) payload.reply_to_post_id = replyToPostId;

      const res = await fetch(
        `${getConfig().API_URL}/api/v1/forums/topics/${id}/posts`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
          body: JSON.stringify(payload),
        },
      );

      if (!res.ok) {
        const data = await res.json().catch(() => null);
        throw new Error(data?.error?.message ?? "Failed to post reply");
      }

      setReplyBody("");
      setReplyToPostId(null);

      // Navigate to last page to see the new post
      const newTotal = total + 1;
      const lastPage = Math.ceil(newTotal / PER_PAGE);
      if (lastPage !== page) {
        const params = new URLSearchParams(searchParams);
        params.set("page", String(lastPage));
        setSearchParams(params);
      } else {
        await fetchTopic();
      }
    } catch (err) {
      setSubmitError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  const canModifyPost = (post: PostData) =>
    !!user && (user.id === post.user_id || user.isAdmin || user.isStaff);

  const handleStartEdit = (post: PostData) => {
    setEditingPostId(post.id);
    setEditBody(post.body);
    setEditError(null);
  };

  const handleCancelEdit = () => {
    setEditingPostId(null);
    setEditBody("");
    setEditError(null);
  };

  const handleSaveEdit = async (postId: number) => {
    if (!editBody.trim()) return;
    setEditSaving(true);
    setEditError(null);

    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/forums/posts/${postId}`,
        {
          method: "PUT",
          headers: {
            "Content-Type": "application/json",
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
          body: JSON.stringify({ body: editBody }),
        },
      );

      if (!res.ok) {
        const data = await res.json().catch(() => null);
        throw new Error(data?.error?.message ?? "Failed to save edit");
      }

      const data = await res.json();
      setPosts((prev) =>
        prev.map((p) => (p.id === postId ? { ...p, ...data.post } : p)),
      );
      setEditingPostId(null);
      setEditBody("");
    } catch (err) {
      setEditError((err as Error).message);
    } finally {
      setEditSaving(false);
    }
  };

  const handleConfirmDelete = async () => {
    if (!deletePostId) return;
    setDeleteError(null);
    setDeleting(true);

    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/forums/posts/${deletePostId}`,
        {
          method: "DELETE",
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        },
      );

      if (!res.ok) {
        const data = await res.json().catch(() => null);
        const msg =
          res.status === 400
            ? (data?.error?.message ??
              "Cannot delete the first post. Delete the topic instead.")
            : (data?.error?.message ?? "Failed to delete post");
        throw new Error(msg);
      }

      setPosts((prev) => prev.filter((p) => p.id !== deletePostId));
      setTotal((prev) => prev - 1);
      setDeletePostId(null);
    } catch (err) {
      setDeleteError((err as Error).message);
      setDeletePostId(null);
    } finally {
      setDeleting(false);
    }
  };

  const isMod = !!(user?.isAdmin || user?.isStaff);

  const modAction = async (url: string, method: string, body?: object) => {
    setModLoading(true);
    setError(null);
    try {
      const token = getAccessToken();
      const res = await fetch(`${getConfig().API_URL}${url}`, {
        method,
        headers: {
          "Content-Type": "application/json",
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        ...(body ? { body: JSON.stringify(body) } : {}),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => null);
        throw new Error(data?.error?.message ?? "Action failed");
      }
      return true;
    } catch (err) {
      setError((err as Error).message);
      return false;
    } finally {
      setModLoading(false);
    }
  };

  const handleToggleLock = async () => {
    if (!topic) return;
    const action = topic.locked ? "unlock" : "lock";
    const ok = await modAction(
      `/api/v1/forums/topics/${topic.id}/${action}`,
      "POST",
    );
    if (ok) {
      setTopic({ ...topic, locked: !topic.locked });
    }
  };

  const handleTogglePin = async () => {
    if (!topic) return;
    const action = topic.pinned ? "unpin" : "pin";
    const ok = await modAction(
      `/api/v1/forums/topics/${topic.id}/${action}`,
      "POST",
    );
    if (ok) {
      setTopic({ ...topic, pinned: !topic.pinned });
    }
  };

  const handleRename = async () => {
    if (!topic || !renameTitle.trim()) return;
    const ok = await modAction(
      `/api/v1/forums/topics/${topic.id}/title`,
      "PUT",
      {
        title: renameTitle.trim(),
      },
    );
    if (ok) {
      setTopic({ ...topic, title: renameTitle.trim() });
      setShowRenameModal(false);
    }
  };

  const handleOpenMoveModal = async () => {
    setShowMoveModal(true);
    try {
      const token = getAccessToken();
      const res = await fetch(`${getConfig().API_URL}/api/v1/forums`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (res.ok) {
        const data = await res.json();
        const forumList: ForumOption[] = [];
        for (const cat of data.categories ?? []) {
          for (const f of cat.forums ?? []) {
            forumList.push({ id: f.id, name: f.name });
          }
        }
        setForums(forumList);
        if (topic && !selectedForumId) {
          setSelectedForumId(topic.forum_id);
        }
      }
    } catch {
      // forum list fetch failed — user can cancel
    }
  };

  const handleMove = async () => {
    if (!topic || !selectedForumId || selectedForumId === topic.forum_id)
      return;
    const ok = await modAction(
      `/api/v1/forums/topics/${topic.id}/move`,
      "POST",
      {
        forum_id: selectedForumId,
      },
    );
    if (ok) {
      setShowMoveModal(false);
      await fetchTopic();
    }
  };

  const handleDelete = async () => {
    if (!topic) return;
    const ok = await modAction(`/api/v1/forums/topics/${topic.id}`, "DELETE");
    if (ok) {
      setShowDeleteConfirm(false);
      navigate(`/forums/${topic.forum_id}`);
    }
  };

  if (loading) return <div className="topic-view-page">Loading topic...</div>;
  if (error) return <div className="topic-view-page">Error: {error}</div>;
  if (!topic) return <div className="topic-view-page">Topic not found.</div>;

  const totalPages = Math.ceil(total / PER_PAGE);
  const canReply = !!user && !topic.locked;

  return (
    <div className="topic-view-page">
      <div className="topic-view-page__breadcrumb">
        <Link to="/forums">Forums</Link> &rsaquo;{" "}
        <Link to={`/forums/${topic.forum_id}`}>{topic.forum_name}</Link>{" "}
        &rsaquo; {topic.title}
      </div>

      <h1>{topic.title}</h1>

      {isMod && (
        <div className="forum-mod-toolbar">
          <button onClick={handleToggleLock} disabled={modLoading}>
            {topic.locked ? "Unlock" : "Lock"}
          </button>
          <button onClick={handleTogglePin} disabled={modLoading}>
            {topic.pinned ? "Unpin" : "Pin"}
          </button>
          <button
            onClick={() => {
              setRenameTitle(topic.title);
              setShowRenameModal(true);
            }}
            disabled={modLoading}
          >
            Rename
          </button>
          <button onClick={handleOpenMoveModal} disabled={modLoading}>
            Move
          </button>
          <button
            onClick={() => setShowDeleteConfirm(true)}
            className="forum-mod-toolbar__danger"
            disabled={modLoading}
          >
            Delete
          </button>
        </div>
      )}

      {topic.locked && (
        <div className="topic-view-page__locked">
          This topic is locked. No new replies can be posted.
        </div>
      )}

      <div className="forum-posts">
        {posts.map((post) => (
          <div key={post.id} className="forum-post">
            <div className="forum-post__sidebar">
              {post.avatar ? (
                <img
                  src={post.avatar}
                  alt={post.username}
                  className="forum-post__avatar"
                />
              ) : (
                <div className="forum-post__avatar-placeholder">
                  {post.username.charAt(0).toUpperCase()}
                </div>
              )}
              <UsernameDisplay userId={post.user_id} username={post.username} />
              <div className="forum-post__group">{post.group_name}</div>
              <div className="forum-post__stats">
                Posts: {post.user_post_count}
                <br />
                Joined: {timeAgo(post.user_created_at)}
              </div>
            </div>
            <div className="forum-post__content">
              <div className="forum-post__header">
                <span>{timeAgo(post.created_at)}</span>
                <span>#{post.id}</span>
              </div>
              {post.reply_to_post_id && (
                <div className="forum-post__reply-ref">
                  In reply to post #{post.reply_to_post_id}
                </div>
              )}
              {editingPostId === post.id ? (
                <div className="forum-post__edit-form">
                  <textarea
                    value={editBody}
                    onChange={(e) => setEditBody(e.target.value)}
                    placeholder="Edit your post... (Markdown supported)"
                  />
                  {editError && (
                    <div className="forum-post__error">{editError}</div>
                  )}
                  <div className="forum-post__actions">
                    <button
                      className="btn btn--primary"
                      onClick={() => handleSaveEdit(post.id)}
                      disabled={editSaving || !editBody.trim()}
                    >
                      {editSaving ? "Saving..." : "Save"}
                    </button>
                    <button
                      className="btn btn--secondary"
                      onClick={handleCancelEdit}
                      disabled={editSaving}
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              ) : (
                <>
                  <div className="forum-post__body">
                    <MarkdownRenderer content={post.body} />
                  </div>
                  {post.edited_at && (
                    <div className="forum-post__edited">
                      Edited {timeAgo(post.edited_at)}
                    </div>
                  )}
                  <div className="forum-post__actions">
                    {canReply && (
                      <button
                        className="forum-post__quote-btn"
                        onClick={() => handleQuote(post)}
                      >
                        Quote
                      </button>
                    )}
                    {canModifyPost(post) && (
                      <>
                        <button
                          className="forum-post__edit-btn"
                          onClick={() => handleStartEdit(post)}
                        >
                          Edit
                        </button>
                        <button
                          className="forum-post__delete-btn"
                          onClick={() => setDeletePostId(post.id)}
                        >
                          Delete
                        </button>
                      </>
                    )}
                  </div>
                </>
              )}
            </div>
          </div>
        ))}
      </div>

      <Pagination
        currentPage={page}
        totalPages={totalPages}
        onPageChange={handlePageChange}
      />

      {deleteError && (
        <div className="forum-post__error" style={{ margin: "1rem 0" }}>
          {deleteError}
        </div>
      )}

      <ConfirmModal
        isOpen={deletePostId !== null}
        title="Delete Post"
        message="Are you sure you want to delete this post? This action cannot be undone."
        confirmLabel="Delete"
        danger
        loading={deleting}
        onConfirm={handleConfirmDelete}
        onCancel={() => setDeletePostId(null)}
      />

      {canReply && (
        <form className="forum-reply" onSubmit={handleSubmitReply}>
          <h3>Post a Reply</h3>
          {replyToPostId && (
            <div className="forum-post__reply-ref">
              Replying to post #{replyToPostId}{" "}
              <button
                type="button"
                className="forum-post__quote-btn"
                onClick={() => setReplyToPostId(null)}
              >
                Cancel
              </button>
            </div>
          )}
          <textarea
            value={replyBody}
            onChange={(e) => setReplyBody(e.target.value)}
            placeholder="Write your reply... (Markdown supported)"
            required
          />
          {submitError && (
            <div className="forum-post__error">{submitError}</div>
          )}
          <div className="forum-reply__actions">
            <button
              type="submit"
              className="btn btn--primary"
              disabled={submitting || !replyBody.trim()}
            >
              {submitting ? "Posting..." : "Post Reply"}
            </button>
          </div>
        </form>
      )}

      {/* Rename Modal */}
      <Modal
        isOpen={showRenameModal}
        onClose={() => setShowRenameModal(false)}
        title="Rename Topic"
      >
        <div className="modal-body">
          <input
            type="text"
            value={renameTitle}
            onChange={(e) => setRenameTitle(e.target.value)}
            style={{ width: "100%", padding: "0.5rem", fontSize: "0.95rem" }}
            aria-label="New topic title"
          />
        </div>
        <div className="modal-footer">
          <button
            className="modal-btn modal-btn--secondary"
            onClick={() => setShowRenameModal(false)}
          >
            Cancel
          </button>
          <button
            className="modal-btn modal-btn--primary"
            onClick={handleRename}
            disabled={modLoading || !renameTitle.trim()}
          >
            Rename
          </button>
        </div>
      </Modal>

      {/* Move Modal */}
      <Modal
        isOpen={showMoveModal}
        onClose={() => setShowMoveModal(false)}
        title="Move Topic"
      >
        <div className="modal-body">
          <select
            value={selectedForumId ?? ""}
            onChange={(e) => setSelectedForumId(Number(e.target.value))}
            style={{ width: "100%", padding: "0.5rem", fontSize: "0.95rem" }}
            aria-label="Select destination forum"
          >
            <option value="" disabled>
              Select a forum...
            </option>
            {forums.map((f) => (
              <option key={f.id} value={f.id}>
                {f.name}
              </option>
            ))}
          </select>
        </div>
        <div className="modal-footer">
          <button
            className="modal-btn modal-btn--secondary"
            onClick={() => setShowMoveModal(false)}
          >
            Cancel
          </button>
          <button
            className="modal-btn modal-btn--primary"
            onClick={handleMove}
            disabled={
              modLoading ||
              !selectedForumId ||
              selectedForumId === topic.forum_id
            }
          >
            Move
          </button>
        </div>
      </Modal>

      {/* Delete Confirm */}
      <ConfirmModal
        isOpen={showDeleteConfirm}
        title="Delete Topic"
        message="Are you sure you want to delete this topic? This action cannot be undone."
        confirmLabel="Delete"
        cancelLabel="Cancel"
        danger={true}
        onConfirm={handleDelete}
        onCancel={() => setShowDeleteConfirm(false)}
      />
    </div>
  );
}
