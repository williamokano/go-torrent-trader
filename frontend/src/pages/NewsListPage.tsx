import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getConfig } from "@/config";
import { timeAgo } from "@/utils/format";
import { Pagination } from "@/components/Pagination";
import "./news.css";

interface NewsArticle {
  id: number;
  title: string;
  body: string;
  author_name: string | null;
  created_at: string;
}

const PER_PAGE = 10;
const PREVIEW_LENGTH = 300;

export function NewsListPage() {
  const [articles, setArticles] = useState<NewsArticle[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);

  const fetchNews = useCallback(async () => {
    setLoading(true);
    const params = new URLSearchParams();
    params.set("page", String(page));
    params.set("per_page", String(PER_PAGE));

    try {
      const res = await fetch(`${getConfig().API_URL}/api/v1/news?${params}`);
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
    fetchNews();
  }, [fetchNews]);

  const totalPages = Math.ceil(total / PER_PAGE);

  return (
    <div className="news">
      <h1 className="news__title">News</h1>

      {loading ? (
        <p className="news__loading">Loading...</p>
      ) : articles.length === 0 ? (
        <p className="news__empty">No news articles yet.</p>
      ) : (
        <>
          <div className="news__list">
            {articles.map((a) => (
              <article key={a.id} className="news__article">
                <h2 className="news__article-title">
                  <Link to={`/news/${a.id}`}>{a.title}</Link>
                </h2>
                <div className="news__article-meta">
                  <span>By {a.author_name ?? "Unknown"}</span>
                  <span>{timeAgo(a.created_at)}</span>
                </div>
                <p className="news__article-preview">
                  {a.body.length > PREVIEW_LENGTH
                    ? a.body.slice(0, PREVIEW_LENGTH) + "..."
                    : a.body}
                </p>
                <Link to={`/news/${a.id}`} className="news__read-more">
                  Read more
                </Link>
              </article>
            ))}
          </div>

          {totalPages > 1 && (
            <Pagination
              currentPage={page}
              totalPages={totalPages}
              onPageChange={setPage}
            />
          )}
        </>
      )}
    </div>
  );
}
