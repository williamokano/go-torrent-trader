import { Link } from "react-router-dom";
import { formatBytes, formatRatio, timeAgo } from "@/utils/format";
import type { ActivityItem } from "@/types/activity";

interface ActivityHistoryTableProps {
  items: ActivityItem[];
  page: number;
  totalPages: number;
  onPageChange: (page: number) => void;
  /** Shown in place of the table when `items` is empty. */
  emptyMessage?: string;
}

/**
 * Renders a user's completed-download history: a torrent/uploaded/downloaded/
 * ratio/completed-at/last-announce table with simple Previous/Next pagination.
 * Shared by `UserProfilePage`'s "History" tab and the standalone `/completed`
 * page — both consume the same activity endpoint.
 */
export function ActivityHistoryTable({
  items,
  page,
  totalPages,
  onPageChange,
  emptyMessage = "No activity found.",
}: ActivityHistoryTableProps) {
  if (items.length === 0) {
    return <p className="profile-activity__empty">{emptyMessage}</p>;
  }

  return (
    <>
      <table className="profile-activity__table">
        <thead>
          <tr>
            <th>Torrent</th>
            <th>Uploaded</th>
            <th>Downloaded</th>
            <th>Ratio</th>
            <th>Completed</th>
            <th>Last Announce</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.torrent_id}>
              <td>
                <Link to={`/torrent/${item.torrent_id}`}>
                  {item.torrent_name}
                </Link>
              </td>
              <td>{formatBytes(item.uploaded)}</td>
              <td>{formatBytes(item.downloaded)}</td>
              <td>{formatRatio(item.ratio)}</td>
              <td>{item.completed_at ? timeAgo(item.completed_at) : "-"}</td>
              <td>{item.last_announce ? timeAgo(item.last_announce) : "-"}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <HistoryPagination
        page={page}
        totalPages={totalPages}
        onChange={onPageChange}
      />
    </>
  );
}

function HistoryPagination({
  page,
  totalPages,
  onChange,
}: {
  page: number;
  totalPages: number;
  onChange: (p: number) => void;
}) {
  if (totalPages <= 1) return null;

  return (
    <div className="profile-activity__pagination">
      <button
        disabled={page <= 1}
        onClick={() => onChange(page - 1)}
        className="profile-activity__page-btn"
      >
        Previous
      </button>
      <span className="profile-activity__page-info">
        Page {page} of {totalPages}
      </span>
      <button
        disabled={page >= totalPages}
        onClick={() => onChange(page + 1)}
        className="profile-activity__page-btn"
      >
        Next
      </button>
    </div>
  );
}
