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
import { useToast } from "@/components/toast";
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
  is_deleted?: boolean;
  deleted_at?: string;
  deleted_by?: number;
  is_first_post?: boolean;
  created_at: string;
  user_created_at: string;
  user_post_count: number;
}

interface PostEditData {
  id: number;
  post_id: number;
  edited_by: number;
  old_body: string;
  new_body: string;
  created_at: string;
  username?: string;
}

const PER_PAGE = 25;

export function ForumTopicViewPage() {
  const { id } = useParams<{ id: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();
  const { user } = useAuth();
  const toast = useToast();

  const page = Math.max(1, Number(searchParams.get("page")) || 1);

  const [topic, setTopic] = useState<TopicData | null>(null);
  const [posts, setPosts] = useState<PostData[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [canModerate, setCanModerate] = useState(false);

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

  // Subscription state
  const [subscribed, setSubscribed] = useState(false);
  const [subLoading, setSubLoading] = useState(false);

  // Moderation state
  const [showRenameModal, setShowRenameModal] = useState(false);
  const [renameTitle, setRenameTitle] = useState("");
  const [showMoveModal, setShowMoveModal] = useState(false);
  const [forums, setForums] = useState<ForumOption[]>([]);
  const [selectedForumId, setSelectedForumId] = useState<number | null>(null);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [modLoading, setModLoading] = useState(false);
  const [modReason, setModReason] = useState("");
  const [showLockConfirm, setShowLockConfirm] = useState(false);
  const [showPinConfirm, setShowPinConfirm] = useState(false);

  // Soft-delete & edit history state
  const [expandedDeletedPosts, setExpandedDeletedPosts] = useState<Set<number>>(
    new Set(),
  );
  const [editHistoryPostId, setEditHistoryPostId] = useState<number | null>(
    null,
  );
  const [editHistory, setEditHistory] = useState<PostEditData[]>([]);
  const [editHistoryLoading, setEditHistoryLoading] = useState(false);

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
      setCanModerate(data.can_moderate ?? false);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, [id, page]);

  useEffect(() => {
    fetchTopic();
  }, [fetchTopic]);

  // Fetch subscription status
  useEffect(() => {
    if (!id) return;
    const token = getAccessToken();
    if (!token) return;
    fetch(`${getConfig().API_URL}/api/v1/forums/topics/${id}/subscription`, {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then((r) => r.json())
      .then((d) => setSubscribed(d?.subscribed ?? false))
      .catch(() => {});
  }, [id]);

  // Scroll to post anchor if URL has a hash like #post-123
  useEffect(() => {
    const hash = window.location.hash;
    if (hash && hash.startsWith("#post-")) {
      const el = document.getElementById(hash.slice(1));
      if (el) {
        el.scrollIntoView({ behavior: "smooth", block: "start" });
      }
    }
  }, [posts]);

  const handleToggleSubscription = async () => {
    if (!id) return;
    setSubLoading(true);
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/forums/topics/${id}/subscribe`,
        {
          method: subscribed ? "DELETE" : "POST",
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        },
      );
      if (!res.ok) throw new Error();
      setSubscribed(!subscribed);
    } catch {
      // ignore
    } finally {
      setSubLoading(false);
    }
  };

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
      toast.error((err as Error).message);
      return false;
    } finally {
      setModLoading(false);
    }
  };

  const handleRestorePost = async (postId: number) => {
    const ok = await modAction(
      `/api/v1/forums/posts/${postId}/restore`,
      "POST",
    );
    if (ok) {
      await fetchTopic();
    }
  };

  const handleToggleDeletedContent = (postId: number) => {
    setExpandedDeletedPosts((prev) => {
      const next = new Set(prev);
      if (next.has(postId)) {
        next.delete(postId);
      } else {
        next.add(postId);
      }
      return next;
    });
  };

  const handleViewEditHistory = async (postId: number) => {
    setEditHistoryPostId(postId);
    setEditHistoryLoading(true);
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/forums/posts/${postId}/edits`,
        { headers: token ? { Authorization: `Bearer ${token}` } : {} },
      );
      if (!res.ok) throw new Error("Failed to load edit history");
      const data = await res.json();
      setEditHistory(data.edits ?? []);
    } catch {
      setEditHistory([]);
    } finally {
      setEditHistoryLoading(false);
    }
  };

  const handleToggleLock = async () => {
    if (!topic) return;
    const action = topic.locked ? "unlock" : "lock";
    const body = modReason.trim() ? { reason: modReason.trim() } : undefined;
    const ok = await modAction(
      `/api/v1/forums/topics/${topic.id}/${action}`,
      "POST",
      body,
    );
    if (ok) {
      setTopic({ ...topic, locked: !topic.locked });
      setShowLockConfirm(false);
      setModReason("");
    }
  };

  const handleTogglePin = async () => {
    if (!topic) return;
    const action = topic.pinned ? "unpin" : "pin";
    const body = modReason.trim() ? { reason: modReason.trim() } : undefined;
    const ok = await modAction(
      `/api/v1/forums/topics/${topic.id}/${action}`,
      "POST",
      body,
    );
    if (ok) {
      setTopic({ ...topic, pinned: !topic.pinned });
      setShowPinConfirm(false);
      setModReason("");
    }
  };

  const handleRename = async () => {
    if (!topic || !renameTitle.trim()) return;
    const body: Record<string, unknown> = { title: renameTitle.trim() };
    if (modReason.trim()) body.reason = modReason.trim();
    const ok = await modAction(
      `/api/v1/forums/topics/${topic.id}/title`,
      "PUT",
      body,
    );
    if (ok) {
      setTopic({ ...topic, title: renameTitle.trim() });
      setShowRenameModal(false);
      setModReason("");
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
    const body: Record<string, unknown> = { forum_id: selectedForumId };
    if (modReason.trim()) body.reason = modReason.trim();
    const ok = await modAction(
      `/api/v1/forums/topics/${topic.id}/move`,
      "POST",
      body,
    );
    if (ok) {
      setShowMoveModal(false);
      setModReason("");
      await fetchTopic();
    }
  };

  const handleDelete = async () => {
    if (!topic) return;
    const body = modReason.trim() ? { reason: modReason.trim() } : undefined;
    const ok = await modAction(
      `/api/v1/forums/topics/${topic.id}`,
      "DELETE",
      body,
    );
    if (ok) {
      setShowDeleteConfirm(false);
      setModReason("");
      navigate(`/forums/${topic.forum_id}`);
    }
  };

  if (loading) return <div className="topic-view-page">Loading topic...</div>;
  if (error) return <div className="topic-view-page">Error: {error}</div>;
  if (!topic) return <div className="topic-view-page">Topic not found.</div>;

  const totalPages = Math.ceil(total / PER_PAGE);
  const canReply = !!user && (!topic.locked || user.isAdmin || user.isStaff);

  return (
    <div className="topic-view-page">
      <div className="topic-view-page__breadcrumb">
        <Link to="/forums">Forums</Link> &rsaquo;{" "}
        <Link to={`/forums/${topic.forum_id}`}>{topic.forum_name}</Link>{" "}
        &rsaquo; {topic.title}
      </div>

      <div className="topic-view-page__title-row">
        <h1>
          {topic.title}
          {!canModerate && !!user && user.id === topic.user_id && !topic.locked && (
            <button
              className="forum-post__edit-btn"
              style={{ marginLeft: "0.5rem", fontSize: "0.8rem" }}
              onClick={() => {
                setRenameTitle(topic.title);
                setShowRenameModal(true);
              }}
            >
              Edit Title
            </button>
          )}
        </h1>
        {!!user && (
          <button
            className={`topic-watch-btn${subscribed ? " topic-watch-btn--active" : ""}`}
            onClick={handleToggleSubscription}
            disabled={subLoading}
            title={
              subscribed
                ? "Stop receiving notifications for this topic"
                : "Get notified about new posts in this topic"
            }
          >
            {subLoading ? "..." : subscribed ? "Watching" : "Watch"}
          </button>
        )}
      </div>

      {canModerate && (
        <div className="forum-mod-toolbar">
          <button
            onClick={() => {
              setModReason("");
              setShowLockConfirm(true);
            }}
            disabled={modLoading}
          >
            {topic.locked ? "Unlock" : "Lock"}
          </button>
          <button
            onClick={() => {
              setModReason("");
              setShowPinConfirm(true);
            }}
            disabled={modLoading}
          >
            {topic.pinned ? "Unpin" : "Pin"}
          </button>
          <button
            onClick={() => {
              setRenameTitle(topic.title);
              setModReason("");
              setShowRenameModal(true);
            }}
            disabled={modLoading}
          >
            Rename
          </button>
          <button
            onClick={() => {
              setModReason("");
              handleOpenMoveModal();
            }}
            disabled={modLoading}
          >
            Move
          </button>
          <button
            onClick={() => {
              setModReason("");
              setShowDeleteConfirm(true);
            }}
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
          <div
            key={post.id}
            id={`post-${post.id}`}
            className={`forum-post${post.is_deleted ? " forum-post--deleted" : ""}`}
          >
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
              {post.is_deleted ? (
                <div className="forum-post__deleted-placeholder">
                  <em style={{ color: "var(--text-muted, #888)" }}>
                    [This post has been deleted]
                  </em>
                  {(user?.isAdmin || user?.isStaff) && (
                    <div
                      className="forum-post__actions"
                      style={{ marginTop: "0.5rem" }}
                    >
                      <button
                        className="forum-post__edit-btn"
                        onClick={() => handleToggleDeletedContent(post.id)}
                      >
                        {expandedDeletedPosts.has(post.id)
                          ? "Hide Content"
                          : "View Content"}
                      </button>
                      <button
                        className="forum-post__edit-btn"
                        onClick={() => handleRestorePost(post.id)}
                        disabled={modLoading}
                      >
                        Restore
                      </button>
                    </div>
                  )}
                  {(user?.isAdmin || user?.isStaff) && expandedDeletedPosts.has(post.id) && (
                    <div
                      className="forum-post__body"
                      style={{ marginTop: "0.5rem", opacity: 0.6 }}
                    >
                      <MarkdownRenderer content={post.body} />
                    </div>
                  )}
                </div>
              ) : editingPostId === post.id ? (
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
                      {(user?.isAdmin || user?.isStaff) && (
                        <button
                          className="forum-post__edit-btn"
                          style={{ marginLeft: "0.5rem", fontSize: "0.75rem" }}
                          onClick={() => handleViewEditHistory(post.id)}
                        >
                          History
                        </button>
                      )}
                    </div>
                  )}
                  <div className="forum-post__actions">
                    {canReply && !post.is_deleted && (
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
                        {!post.is_first_post && (
                          <button
                            className="forum-post__delete-btn"
                            onClick={() => setDeletePostId(post.id)}
                          >
                            Delete
                          </button>
                        )}
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

      {/* Lock/Unlock Confirm */}
      <Modal
        isOpen={showLockConfirm}
        onClose={() => setShowLockConfirm(false)}
        title={topic.locked ? "Unlock Topic" : "Lock Topic"}
      >
        <div className="modal-body">
          <p style={{ margin: "0 0 0.75rem" }}>
            {topic.locked
              ? "This will allow new replies to be posted."
              : "This will prevent new replies from being posted."}
          </p>
          <textarea
            value={modReason}
            onChange={(e) => setModReason(e.target.value)}
            placeholder="Reason (optional)"
            rows={2}
            style={{ width: "100%", padding: "0.5rem", fontSize: "0.95rem" }}
            aria-label="Moderation reason"
          />
        </div>
        <div className="modal-footer">
          <button
            className="modal-btn modal-btn--secondary"
            onClick={() => setShowLockConfirm(false)}
          >
            Cancel
          </button>
          <button
            className="modal-btn modal-btn--primary"
            onClick={handleToggleLock}
            disabled={modLoading}
          >
            {topic.locked ? "Unlock" : "Lock"}
          </button>
        </div>
      </Modal>

      {/* Pin/Unpin Confirm */}
      <Modal
        isOpen={showPinConfirm}
        onClose={() => setShowPinConfirm(false)}
        title={topic.pinned ? "Unpin Topic" : "Pin Topic"}
      >
        <div className="modal-body">
          <p style={{ margin: "0 0 0.75rem" }}>
            {topic.pinned
              ? "This topic will no longer be pinned to the top."
              : "This topic will be pinned to the top of the forum."}
          </p>
          <textarea
            value={modReason}
            onChange={(e) => setModReason(e.target.value)}
            placeholder="Reason (optional)"
            rows={2}
            style={{ width: "100%", padding: "0.5rem", fontSize: "0.95rem" }}
            aria-label="Moderation reason"
          />
        </div>
        <div className="modal-footer">
          <button
            className="modal-btn modal-btn--secondary"
            onClick={() => setShowPinConfirm(false)}
          >
            Cancel
          </button>
          <button
            className="modal-btn modal-btn--primary"
            onClick={handleTogglePin}
            disabled={modLoading}
          >
            {topic.pinned ? "Unpin" : "Pin"}
          </button>
        </div>
      </Modal>

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
          <textarea
            value={modReason}
            onChange={(e) => setModReason(e.target.value)}
            placeholder="Reason (optional)"
            rows={2}
            style={{
              width: "100%",
              padding: "0.5rem",
              fontSize: "0.95rem",
              marginTop: "0.75rem",
            }}
            aria-label="Moderation reason"
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
          <textarea
            value={modReason}
            onChange={(e) => setModReason(e.target.value)}
            placeholder="Reason (optional)"
            rows={2}
            style={{
              width: "100%",
              padding: "0.5rem",
              fontSize: "0.95rem",
              marginTop: "0.75rem",
            }}
            aria-label="Moderation reason"
          />
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
      <Modal
        isOpen={showDeleteConfirm}
        onClose={() => setShowDeleteConfirm(false)}
        title="Delete Topic"
      >
        <div className="modal-body">
          <p style={{ margin: "0 0 0.75rem", color: "var(--color-danger)" }}>
            Are you sure you want to delete this topic? This action cannot be
            undone.
          </p>
          <textarea
            value={modReason}
            onChange={(e) => setModReason(e.target.value)}
            placeholder="Reason (optional)"
            rows={2}
            style={{ width: "100%", padding: "0.5rem", fontSize: "0.95rem" }}
            aria-label="Moderation reason"
          />
        </div>
        <div className="modal-footer">
          <button
            className="modal-btn modal-btn--secondary"
            onClick={() => setShowDeleteConfirm(false)}
          >
            Cancel
          </button>
          <button
            className="modal-btn modal-btn--danger"
            onClick={handleDelete}
            disabled={modLoading}
          >
            Delete
          </button>
        </div>
      </Modal>

      {/* Edit History Modal */}
      <Modal
        isOpen={editHistoryPostId !== null}
        onClose={() => setEditHistoryPostId(null)}
        title="Edit History"
      >
        <div className="modal-body">
          {editHistoryLoading ? (
            <p>Loading...</p>
          ) : editHistory.length === 0 ? (
            <p>No edit history found.</p>
          ) : (
            <div style={{ maxHeight: "400px", overflow: "auto" }}>
              {editHistory.map((edit) => (
                <div
                  key={edit.id}
                  style={{
                    marginBottom: "1rem",
                    borderBottom: "1px solid var(--border-color, #333)",
                    paddingBottom: "0.75rem",
                  }}
                >
                  <div
                    style={{
                      fontSize: "0.8rem",
                      color: "var(--text-muted, #888)",
                      marginBottom: "0.25rem",
                    }}
                  >
                    Edited by{" "}
                    <strong>
                      {edit.username ?? `User #${edit.edited_by}`}
                    </strong>{" "}
                    {timeAgo(edit.created_at)}
                  </div>
                  <div style={{ fontSize: "0.85rem" }}>
                    <div
                      style={{
                        color: "var(--danger-color, #c44)",
                        marginBottom: "0.25rem",
                      }}
                    >
                      <strong>Before:</strong>
                      <pre
                        style={{
                          whiteSpace: "pre-wrap",
                          margin: "0.25rem 0",
                          opacity: 0.7,
                        }}
                      >
                        {edit.old_body}
                      </pre>
                    </div>
                    <div style={{ color: "var(--success-color, #4c4)" }}>
                      <strong>After:</strong>
                      <pre
                        style={{ whiteSpace: "pre-wrap", margin: "0.25rem 0" }}
                      >
                        {edit.new_body}
                      </pre>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
        <div className="modal-footer">
          <button
            className="modal-btn modal-btn--secondary"
            onClick={() => setEditHistoryPostId(null)}
          >
            Close
          </button>
        </div>
      </Modal>
    </div>
  );
}
