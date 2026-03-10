import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { timeAgo } from "@/utils/format";
import "./forums.css";

interface ForumData {
  id: number;
  name: string;
  description: string;
  topic_count: number;
  post_count: number;
  last_post_at?: string;
  last_post_username?: string;
  last_post_topic_id?: number;
  last_post_topic_title?: string;
}

interface CategoryData {
  id: number;
  name: string;
  forums: ForumData[];
}

export function ForumIndexPage() {
  const [categories, setCategories] = useState<CategoryData[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const navigate = useNavigate();

  useEffect(() => {
    let cancelled = false;

    async function fetchForums() {
      setLoading(true);
      setError(null);
      try {
        const token = getAccessToken();
        const res = await fetch(`${getConfig().API_URL}/api/v1/forums`, {
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        });
        if (!res.ok) throw new Error("Failed to load forums");
        const data = await res.json();
        if (!cancelled) {
          setCategories(data.categories ?? []);
        }
      } catch (err) {
        if (!cancelled) setError((err as Error).message);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    fetchForums();
    return () => {
      cancelled = true;
    };
  }, []);

  if (loading) return <div className="forums-page">Loading forums...</div>;
  if (error) return <div className="forums-page">Error: {error}</div>;

  return (
    <div className="forums-page">
      <h1>Forums</h1>

      <form
        className="forums-page__search-bar"
        onSubmit={(e) => {
          e.preventDefault();
          if (searchQuery.trim()) {
            navigate(
              `/forums/search?q=${encodeURIComponent(searchQuery.trim())}`,
            );
          }
        }}
      >
        <input
          type="text"
          placeholder="Search forums..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
        />
        <button type="submit" className="btn btn--primary">
          Search
        </button>
      </form>

      {categories.length === 0 && (
        <p className="forum-empty">No forums available.</p>
      )}

      {categories.map((cat) => (
        <div key={cat.id} className="forum-category">
          <h2 className="forum-category__name">{cat.name}</h2>
          <table className="forum-table">
            <thead>
              <tr>
                <th>Forum</th>
                <th>Topics</th>
                <th>Posts</th>
                <th>Last Post</th>
              </tr>
            </thead>
            <tbody>
              {cat.forums.map((forum) => (
                <tr key={forum.id}>
                  <td>
                    <div className="forum-name">
                      <Link to={`/forums/${forum.id}`}>{forum.name}</Link>
                    </div>
                    {forum.description && (
                      <div className="forum-description">
                        {forum.description}
                      </div>
                    )}
                  </td>
                  <td>{forum.topic_count}</td>
                  <td>{forum.post_count}</td>
                  <td>
                    {forum.last_post_at ? (
                      <div className="forum-last-post">
                        {forum.last_post_topic_id && (
                          <Link
                            to={`/forums/topics/${forum.last_post_topic_id}`}
                            className="forum-last-post__topic"
                            title={forum.last_post_topic_title}
                          >
                            {forum.last_post_topic_title}
                          </Link>
                        )}
                        <span className="forum-last-post__meta">
                          by {forum.last_post_username}{" "}
                          {timeAgo(forum.last_post_at)}
                        </span>
                      </div>
                    ) : (
                      <span className="forum-last-post__meta">
                        No posts yet
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ))}
    </div>
  );
}
