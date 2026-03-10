import { useCallback, useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useAuth } from "@/features/auth";
import { timeAgo } from "@/utils/format";
import { UsernameDisplay } from "@/components/UsernameDisplay";
import { MarkdownRenderer } from "@/components/MarkdownRenderer";
import { Pagination } from "@/components/Pagination";
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
  created_at: string;
  user_created_at: string;
  user_post_count: number;
}

const PER_PAGE = 25;

export function ForumTopicViewPage() {
  const { id } = useParams<{ id: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
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
              <div className="forum-post__body">
                <MarkdownRenderer content={post.body} />
              </div>
              {post.edited_at && (
                <div className="forum-post__edited">
                  Edited {timeAgo(post.edited_at)}
                </div>
              )}
              {canReply && (
                <div className="forum-post__actions">
                  <button
                    className="forum-post__quote-btn"
                    onClick={() => handleQuote(post)}
                  >
                    Quote
                  </button>
                </div>
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
            <div style={{ color: "red", marginBottom: "0.5rem" }}>
              {submitError}
            </div>
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
    </div>
  );
}
