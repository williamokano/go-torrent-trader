import { useEffect, useState } from "react";
import {
  Link,
  useParams,
  useSearchParams,
  useNavigate,
} from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { timeAgo } from "@/utils/format";
import { UsernameDisplay } from "@/components/UsernameDisplay";
import { Pagination } from "@/components/Pagination";
import "./forums.css";

interface ForumData {
  id: number;
  name: string;
  description: string;
  topic_count: number;
  post_count: number;
  min_post_level: number;
}

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
  last_post_at?: string;
  last_post_username?: string;
  created_at: string;
}

const PER_PAGE = 25;

export function ForumTopicListPage() {
  const { id } = useParams<{ id: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();

  const page = Math.max(1, Number(searchParams.get("page")) || 1);

  const [forum, setForum] = useState<ForumData | null>(null);
  const [topics, setTopics] = useState<TopicData[]>([]);
  const [total, setTotal] = useState(0);
  const [canCreateTopic, setCanCreateTopic] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function fetchTopics() {
      setLoading(true);
      setError(null);
      try {
        const token = getAccessToken();
        const params = new URLSearchParams();
        params.set("page", String(page));
        params.set("per_page", String(PER_PAGE));

        const res = await fetch(
          `${getConfig().API_URL}/api/v1/forums/${id}/topics?${params.toString()}`,
          { headers: token ? { Authorization: `Bearer ${token}` } : {} },
        );
        if (res.status === 403) {
          if (!cancelled) setError("You do not have access to this forum.");
          return;
        }
        if (!res.ok) throw new Error("Failed to load topics");
        const data = await res.json();
        if (!cancelled) {
          setForum(data.forum ?? null);
          setTopics(data.topics ?? []);
          setTotal(data.total ?? 0);
          setCanCreateTopic(data.can_create_topic ?? false);
        }
      } catch (err) {
        if (!cancelled) setError((err as Error).message);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    fetchTopics();
    return () => {
      cancelled = true;
    };
  }, [id, page]);

  const handlePageChange = (newPage: number) => {
    const params = new URLSearchParams(searchParams);
    params.set("page", String(newPage));
    setSearchParams(params);
  };

  if (loading) return <div className="topic-list-page">Loading topics...</div>;
  if (error) return <div className="topic-list-page">Error: {error}</div>;
  if (!forum) return <div className="topic-list-page">Forum not found.</div>;

  const totalPages = Math.ceil(total / PER_PAGE);
  const canPost = canCreateTopic;

  return (
    <div className="topic-list-page">
      <div className="topic-view-page__breadcrumb">
        <Link to="/forums">Forums</Link> &rsaquo; {forum.name}
      </div>

      <div className="topic-list-page__header">
        <div>
          <h1>{forum.name}</h1>
          {forum.description && (
            <p className="topic-list-page__description">{forum.description}</p>
          )}
        </div>
        {canPost && (
          <button
            className="btn btn--primary"
            onClick={() => navigate(`/forums/${id}/new`)}
          >
            New Topic
          </button>
        )}
      </div>

      {topics.length === 0 ? (
        <p className="forum-empty">No topics yet. Be the first to post!</p>
      ) : (
        <table className="topic-table">
          <thead>
            <tr>
              <th>Topic</th>
              <th>Author</th>
              <th>Replies</th>
              <th>Views</th>
              <th>Last Post</th>
            </tr>
          </thead>
          <tbody>
            {topics.map((topic) => (
              <tr
                key={topic.id}
                className={topic.pinned ? "topic-row--pinned" : ""}
              >
                <td>
                  <div className="topic-title">
                    {topic.pinned && (
                      <span className="topic-title__icon" title="Pinned">
                        &#128204;
                      </span>
                    )}
                    {topic.locked && (
                      <span className="topic-title__icon" title="Locked">
                        &#128274;
                      </span>
                    )}
                    <Link to={`/forums/topics/${topic.id}`}>{topic.title}</Link>
                  </div>
                </td>
                <td>
                  <UsernameDisplay
                    userId={topic.user_id}
                    username={topic.username}
                  />
                </td>
                <td>{Math.max(0, topic.post_count - 1)}</td>
                <td>{topic.view_count}</td>
                <td>
                  {topic.last_post_at ? (
                    <div className="topic-last-post">
                      <span className="topic-last-post__meta">
                        {topic.last_post_username && (
                          <>by {topic.last_post_username} </>
                        )}
                        {timeAgo(topic.last_post_at)}
                      </span>
                    </div>
                  ) : (
                    <span className="topic-last-post__meta">
                      {timeAgo(topic.created_at)}
                    </span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <Pagination
        currentPage={page}
        totalPages={totalPages}
        onPageChange={handlePageChange}
      />
    </div>
  );
}
