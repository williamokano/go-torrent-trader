import { useEffect, useState } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import "./forums.css";

export function ForumNewTopicPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const [forumName, setForumName] = useState("");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loadingForum, setLoadingForum] = useState(true);

  useEffect(() => {
    async function fetchForum() {
      try {
        const token = getAccessToken();
        const res = await fetch(`${getConfig().API_URL}/api/v1/forums/${id}`, {
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        });
        if (!res.ok) throw new Error("Failed to load forum");
        const data = await res.json();
        setForumName(data.forum?.name ?? "");
      } catch {
        setError("Failed to load forum info");
      } finally {
        setLoadingForum(false);
      }
    }
    fetchForum();
  }, [id]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim() || !body.trim()) return;

    setSubmitting(true);
    setError(null);

    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/forums/${id}/topics`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
          body: JSON.stringify({ title: title.trim(), body: body.trim() }),
        },
      );

      if (!res.ok) {
        const data = await res.json().catch(() => null);
        throw new Error(data?.error?.message ?? "Failed to create topic");
      }

      const data = await res.json();
      const topicId = data.topic?.id;
      if (topicId) {
        navigate(`/forums/topics/${topicId}`);
      } else {
        navigate(`/forums/${id}`);
      }
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  if (loadingForum) return <div className="new-topic-form">Loading...</div>;

  return (
    <div className="new-topic-form">
      <div className="topic-view-page__breadcrumb">
        <Link to="/forums">Forums</Link> &rsaquo;{" "}
        <Link to={`/forums/${id}`}>{forumName}</Link> &rsaquo; New Topic
      </div>

      <h1>New Topic</h1>

      <form onSubmit={handleSubmit}>
        <label htmlFor="topic-title">Title</label>
        <input
          id="topic-title"
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Topic title"
          maxLength={200}
          required
        />

        <label htmlFor="topic-body">Body</label>
        <textarea
          id="topic-body"
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="Write your post... (Markdown supported)"
          required
        />

        {error && (
          <div style={{ color: "red", marginBottom: "0.5rem" }}>{error}</div>
        )}

        <div className="new-topic-form__actions">
          <button
            type="submit"
            className="btn btn--primary"
            disabled={submitting || !title.trim() || !body.trim()}
          >
            {submitting ? "Creating..." : "Create Topic"}
          </button>
          <button
            type="button"
            className="btn btn--secondary"
            onClick={() => navigate(`/forums/${id}`)}
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  );
}
