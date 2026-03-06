import { useCallback, useEffect, useState } from "react";
import { useToast } from "@/components/toast";
import { useAuth } from "@/features/auth";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";

interface RatingResponse {
  average: number;
  count: number;
  user_rating?: number;
}

interface RatingWidgetProps {
  torrentId: string;
}

const MAX_STARS = 5;

export function RatingWidget({ torrentId }: RatingWidgetProps) {
  const toast = useToast();
  const { user } = useAuth();

  const [average, setAverage] = useState(0);
  const [count, setCount] = useState(0);
  const [userRating, setUserRating] = useState<number | null>(null);
  const [hoveredStar, setHoveredStar] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  const fetchRating = useCallback(async () => {
    setLoading(true);
    try {
      const token = getAccessToken();
      const baseUrl = getConfig().API_URL;
      const response = await fetch(
        `${baseUrl}/api/v1/torrents/${encodeURIComponent(torrentId)}/rating`,
        {
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        },
      );

      if (!response.ok) {
        throw new Error("Failed to load rating");
      }

      const data: RatingResponse = await response.json();
      setAverage(data.average ?? 0);
      setCount(data.count ?? 0);
      setUserRating(data.user_rating ?? null);
    } catch {
      // Silently fail — rating is supplementary
    } finally {
      setLoading(false);
    }
  }, [torrentId]);

  useEffect(() => {
    fetchRating();
  }, [fetchRating]);

  async function handleRate(rating: number) {
    if (!user || submitting) return;

    setSubmitting(true);
    try {
      const token = getAccessToken();
      if (!token) {
        toast.error("You must be logged in to rate");
        return;
      }

      const baseUrl = getConfig().API_URL;
      const response = await fetch(
        `${baseUrl}/api/v1/torrents/${encodeURIComponent(torrentId)}/rating`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify({ rating }),
        },
      );

      if (!response.ok) {
        const data = await response.json().catch(() => null);
        throw new Error(
          data?.error?.message ??
            `Failed to submit rating (${response.status})`,
        );
      }

      toast.success("Rating submitted");
      await fetchRating();
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to submit rating",
      );
    } finally {
      setSubmitting(false);
    }
  }

  const displayRating = hoveredStar ?? userRating ?? 0;

  if (loading) {
    return (
      <div className="rating-widget" aria-label="Rating">
        <span className="rating-widget__loading">Loading rating...</span>
      </div>
    );
  }

  return (
    <div className="rating-widget" aria-label="Rating">
      <div className="rating-widget__stars">
        {Array.from({ length: MAX_STARS }, (_, i) => {
          const starValue = i + 1;
          const filled = starValue <= displayRating;
          const isUserRated =
            userRating !== null &&
            starValue <= userRating &&
            hoveredStar === null;

          return (
            <button
              key={starValue}
              type="button"
              className={`rating-widget__star${filled ? " rating-widget__star--filled" : ""}${isUserRated ? " rating-widget__star--user" : ""}`}
              onClick={() => handleRate(starValue)}
              onMouseEnter={() => user && setHoveredStar(starValue)}
              onMouseLeave={() => setHoveredStar(null)}
              disabled={!user || submitting}
              aria-label={`Rate ${starValue} star${starValue !== 1 ? "s" : ""}`}
              title={
                user
                  ? `Rate ${starValue} star${starValue !== 1 ? "s" : ""}`
                  : "Log in to rate"
              }
            >
              {filled ? "\u2605" : "\u2606"}
            </button>
          );
        })}
      </div>

      <div className="rating-widget__info">
        <span className="rating-widget__average">
          {average > 0 ? average.toFixed(1) : "N/A"}
        </span>
        <span className="rating-widget__count">
          ({count} {count === 1 ? "vote" : "votes"})
        </span>
      </div>
    </div>
  );
}
