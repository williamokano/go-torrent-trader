import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { Pagination } from "@/components/Pagination";
import { timeAgo } from "@/utils/format";
import "./activity-log.css";

interface LogEntry {
  id: number;
  event_type: string;
  actor_id: number;
  message: string;
  created_at: string;
}

const PER_PAGE = 25;

export function ActivityLogPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const page = Math.max(1, Number(searchParams.get("page")) || 1);

  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);

  const fetchLogs = useCallback(async () => {
    setLoading(true);
    const token = getAccessToken();
    const params = new URLSearchParams();
    params.set("page", String(page));
    params.set("per_page", String(PER_PAGE));

    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/activity-logs?${params}`,
        {
          headers: { Authorization: `Bearer ${token}` },
        },
      );
      if (res.ok) {
        const data = await res.json();
        setLogs(data.logs ?? []);
        setTotal(data.total ?? 0);
      }
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  const handlePageChange = (newPage: number) => {
    const next = new URLSearchParams(searchParams);
    next.set("page", String(newPage));
    setSearchParams(next);
  };

  const totalPages = Math.ceil(total / PER_PAGE);

  return (
    <div>
      <h1 className="activity-log__title">Activity Log</h1>

      {loading ? (
        <p>Loading...</p>
      ) : logs.length === 0 ? (
        <p className="activity-log__empty">No activity yet.</p>
      ) : (
        <>
          <table className="activity-log__table">
            <thead>
              <tr>
                <th>Activity</th>
                <th>When</th>
              </tr>
            </thead>
            <tbody>
              {logs.map((log) => (
                <tr key={log.id}>
                  <td>{log.message}</td>
                  <td className="activity-log__time">
                    {timeAgo(log.created_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          <Pagination
            currentPage={page}
            totalPages={totalPages}
            onPageChange={handlePageChange}
          />
        </>
      )}
    </div>
  );
}
