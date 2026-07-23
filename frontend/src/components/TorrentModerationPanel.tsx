import { useState } from "react";
import { Link } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useToast } from "@/components/toast";
import { UsernameDisplay } from "@/components/UsernameDisplay";
import type { TorrentModeration } from "@/types/moderation";
import "./torrent-moderation-panel.css";

interface Props {
  torrentId: number;
  moderation: TorrentModeration;
  /** Viewer is staff (admin or moderator). */
  isStaff: boolean;
  /** Viewer may approve (staff, or a self-approving uploader on their own upload). */
  canApprove: boolean;
  /** Called after a successful action so the parent can reload the torrent. */
  onChanged: () => void;
}

const STATUS_LABEL: Record<string, string> = {
  pending: "Awaiting moderation",
  rejected: "Rejected",
  approved: "Approved",
};

/**
 * Staff/author moderation panel shown on a pending or rejected torrent's detail
 * page: current status, assigned moderator, and the claim/approve/reject actions.
 * (BE-8.22b adds the message thread here.)
 */
export function TorrentModerationPanel({
  torrentId,
  moderation,
  isStaff,
  canApprove,
  onChanged,
}: Props) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);

  const status = moderation.status;

  async function act(path: string, successMsg: string) {
    if (busy) return;
    setBusy(true);
    try {
      const token = getAccessToken();
      if (!token) {
        toast.error("You must be logged in");
        return;
      }
      const res = await fetch(`${getConfig().API_URL}${path}`, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!res.ok) {
        const data = await res.json().catch(() => null);
        throw new Error(
          data?.error?.message ?? `Action failed (${res.status})`,
        );
      }
      toast.success(successMsg);
      onChanged();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Action failed");
    } finally {
      setBusy(false);
    }
  }

  const claim = () =>
    act(
      `/api/v1/admin/moderation/torrents/${torrentId}/claim`,
      "You are now moderating this torrent",
    );
  const approve = () =>
    act(`/api/v1/torrents/${torrentId}/moderation/approve`, "Torrent approved");
  const reject = () =>
    act(
      `/api/v1/admin/moderation/torrents/${torrentId}/reject`,
      "Torrent rejected",
    );

  return (
    <section
      className={`torrent-moderation-panel torrent-moderation-panel--${status}`}
      aria-label="Moderation"
    >
      <div className="torrent-moderation-panel__header">
        <h2 className="torrent-moderation-panel__title">Moderation</h2>
        <span className="torrent-moderation-panel__badge">
          {STATUS_LABEL[status] ?? status}
        </span>
      </div>

      <div className="torrent-moderation-panel__row">
        <span className="torrent-moderation-panel__label">Moderator</span>
        <span className="torrent-moderation-panel__value">
          {moderation.assigned_moderator_id ? (
            <UsernameDisplay
              userId={moderation.assigned_moderator_id}
              username={moderation.assigned_moderator_name ?? "Unknown"}
            />
          ) : (
            <span className="torrent-moderation-panel__muted">Unassigned</span>
          )}
          {isStaff && status === "pending" && (
            <button
              className="torrent-moderation-panel__link-btn"
              onClick={claim}
              disabled={busy}
            >
              {moderation.assigned_moderator_id ? "Claim (reassign)" : "Claim"}
            </button>
          )}
        </span>
      </div>

      {status === "pending" && (canApprove || isStaff) && (
        <div className="torrent-moderation-panel__actions">
          {canApprove && (
            <button
              className="torrent-moderation-panel__approve"
              onClick={approve}
              disabled={busy}
            >
              Approve
            </button>
          )}
          {isStaff && (
            <button
              className="torrent-moderation-panel__reject"
              onClick={reject}
              disabled={busy}
            >
              Reject
            </button>
          )}
        </div>
      )}

      {status === "rejected" && (
        <p className="torrent-moderation-panel__note">
          This torrent was rejected. The uploader can make changes; a staff
          member can approve it once the issues are resolved.
        </p>
      )}
    </section>
  );
}

/**
 * "Approved by X" line shown to everyone on an approved torrent that carries an
 * approver (auto-approved torrents have none).
 */
export function ApprovedByLine({
  moderation,
}: {
  moderation: TorrentModeration | undefined;
}) {
  if (
    !moderation ||
    moderation.status !== "approved" ||
    !moderation.approved_by_id
  ) {
    return null;
  }
  return (
    <div className="torrent-detail__info-row">
      <span className="torrent-detail__info-label">Approved by</span>
      <span className="torrent-detail__info-value">
        <Link to={`/user/${moderation.approved_by_name ?? ""}`}>
          {moderation.approved_by_name ?? "staff"}
        </Link>
      </span>
    </div>
  );
}
