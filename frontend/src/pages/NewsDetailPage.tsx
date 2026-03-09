import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { getConfig } from "@/config";
import { formatDate } from "@/utils/format";
import type { NewsArticle } from "@/types/news";
import "./news.css";

export function NewsDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [article, setArticle] = useState<NewsArticle | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function fetchArticle() {
      setLoading(true);
      setError(null);
      try {
        const res = await fetch(`${getConfig().API_URL}/api/v1/news/${id}`);
        if (cancelled) return;

        if (!res.ok) {
          setError("Article not found");
          setLoading(false);
          return;
        }

        const data = await res.json();
        setArticle(data.article ?? null);
      } catch {
        if (!cancelled) {
          setError("Failed to load article");
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    fetchArticle();
    return () => {
      cancelled = true;
    };
  }, [id]);

  if (loading) {
    return <div className="news">Loading...</div>;
  }

  if (error || !article) {
    return (
      <div className="news">
        <p className="news__error">{error ?? "Article not found"}</p>
        <Link to="/news" className="news__back-link">
          Back to News
        </Link>
      </div>
    );
  }

  return (
    <div className="news">
      <Link to="/news" className="news__back-link">
        Back to News
      </Link>
      <article className="news__detail">
        <h1 className="news__detail-title">{article.title}</h1>
        <div className="news__detail-meta">
          <span>By {article.author_name ?? "Unknown"}</span>
          <span>{formatDate(article.created_at)}</span>
        </div>
        <div className="news__detail-body">{article.body}</div>
      </article>
    </div>
  );
}
