import { useCallback, useEffect, useState } from "react";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useAuth } from "@/features/auth";
import { ActivityHistoryTable } from "@/components/ActivityHistoryTable";
import type { ActivityItem } from "@/types/activity";
import "./profile.css";
import "./completed.css";

const PER_PAGE = 25;

/**
 * Standalone "Completed" quick-access view for the logged-in user: their
 * download history, reusing the same `/api/v1/users/{id}/activity?tab=history`
 * endpoint (BE-3.16) and table (`ActivityHistoryTable`) as the History tab on
 * `UserProfilePage`, without the surrounding profile chrome.
 */
export function CompletedPage() {
  const { user: currentUser } = useAuth();

  const [items, setItems] = useState<ActivityItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchHistory = useCallback(async (userId: number, p: number) => {
    setLoading(true);
    setError(null);
    try {
      const token = getAccessToken();
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/users/${userId}/activity?tab=history&page=${p}&per_page=${PER_PAGE}`,
        { headers: token ? { Authorization: `Bearer ${token}` } : {} },
      );
      if (res.ok) {
        const body = await res.json();
        setItems(body.activity ?? []);
        setTotal(body.total ?? 0);
      } else {
        setError("Failed to load completed downloads");
      }
    } catch {
      setError("Failed to load completed downloads");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!currentUser) return;
    fetchHistory(currentUser.id, page);
  }, [currentUser, page, fetchHistory]);

  const totalPages = Math.ceil(total / PER_PAGE);

  return (
    <div className="completed-page">
      <div className="completed-page__header">
        <h1 className="completed-page__title">Completed</h1>
      </div>

      {error ? (
        <p className="completed-page__error">{error}</p>
      ) : loading ? (
        <p className="completed-page__loading">Loading...</p>
      ) : (
        <div className="profile-activity">
          <div className="profile-activity__content">
            <ActivityHistoryTable
              items={items}
              page={page}
              totalPages={totalPages}
              onPageChange={setPage}
              emptyMessage="You haven't completed any downloads yet."
            />
          </div>
        </div>
      )}
    </div>
  );
}
