import { useCallback, useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { useToast } from "@/components/toast";
import { Select } from "@/components/form";
import { Pagination } from "@/components/Pagination";
import { timeAgo } from "@/utils/format";
import "./admin-reports.css";

interface Report {
  id: number;
  reporter_id: number;
  reporter_username: string;
  torrent_id?: number;
  torrent_name?: string;
  reason: string;
  resolved: boolean;
  resolved_by?: number;
  resolved_at?: string;
  created_at: string;
}

const PER_PAGE = 25;

export function AdminReportsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const toast = useToast();

  const statusFilter = searchParams.get("status") ?? "";
  const page = Math.max(1, Number(searchParams.get("page")) || 1);

  const [reports, setReports] = useState<Report[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [resolving, setResolving] = useState<number | null>(null);

  const fetchReports = useCallback(async () => {
    setLoading(true);
    const token = getAccessToken();
    const params = new URLSearchParams();
    if (statusFilter) params.set("status", statusFilter);
    params.set("page", String(page));
    params.set("per_page", String(PER_PAGE));

    try {
      const res = await fetch(`/api/v1/reports?${params}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setReports(data.reports ?? []);
        setTotal(data.total ?? 0);
      }
    } finally {
      setLoading(false);
    }
  }, [statusFilter, page]);

  useEffect(() => {
    fetchReports();
  }, [fetchReports]);

  const handleStatusChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const next = new URLSearchParams(searchParams);
    if (e.target.value) {
      next.set("status", e.target.value);
    } else {
      next.delete("status");
    }
    next.delete("page");
    setSearchParams(next);
  };

  const handlePageChange = (newPage: number) => {
    const next = new URLSearchParams(searchParams);
    next.set("page", String(newPage));
    setSearchParams(next);
  };

  const handleResolve = async (reportId: number) => {
    setResolving(reportId);
    const token = getAccessToken();
    try {
      const res = await fetch(`/api/v1/reports/${reportId}/resolve`, {
        method: "PUT",
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        toast.success("Report resolved");
        fetchReports();
      } else {
        toast.error("Failed to resolve report");
      }
    } finally {
      setResolving(null);
    }
  };

  const totalPages = Math.ceil(total / PER_PAGE);

  return (
    <div>
      <div className="admin-reports__header">
        <h1>Reports</h1>
        <Select
          label="Status"
          options={[
            { value: "", label: "All" },
            { value: "pending", label: "Pending" },
            { value: "resolved", label: "Resolved" },
          ]}
          value={statusFilter}
          onChange={handleStatusChange}
        />
      </div>

      {loading ? (
        <p>Loading...</p>
      ) : reports.length === 0 ? (
        <p className="admin-reports__empty">No reports found.</p>
      ) : (
        <>
          <table className="admin-reports__table">
            <thead>
              <tr>
                <th>Reporter</th>
                <th>Torrent</th>
                <th>Reason</th>
                <th>Status</th>
                <th>Created</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {reports.map((report) => (
                <tr key={report.id}>
                  <td>
                    <Link
                      to={`/user/${report.reporter_id}`}
                      className="admin-reports__link"
                    >
                      {report.reporter_username ||
                        `User #${report.reporter_id}`}
                    </Link>
                  </td>
                  <td>
                    {report.torrent_id ? (
                      <Link
                        to={`/torrent/${report.torrent_id}`}
                        className="admin-reports__link"
                      >
                        {report.torrent_name || `Torrent #${report.torrent_id}`}
                      </Link>
                    ) : (
                      "General"
                    )}
                  </td>
                  <td>{report.reason}</td>
                  <td>
                    <span
                      className={`admin-reports__status admin-reports__status--${report.resolved ? "resolved" : "pending"}`}
                    >
                      {report.resolved ? "Resolved" : "Pending"}
                    </span>
                  </td>
                  <td>{timeAgo(report.created_at)}</td>
                  <td>
                    {!report.resolved && (
                      <button
                        className="admin-reports__resolve-btn"
                        onClick={() => handleResolve(report.id)}
                        disabled={resolving === report.id}
                      >
                        {resolving === report.id ? "..." : "Resolve"}
                      </button>
                    )}
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
