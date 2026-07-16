/**
 * A single per-torrent activity record for a user, as returned by
 * `GET /api/v1/users/{id}/activity?tab={seeding|leeching|history}`.
 *
 * `ip`/`port` are only present for the seeding/leeching tabs; `completed_at`
 * is only present for the history tab.
 */
export interface ActivityItem {
  torrent_id: number;
  torrent_name: string;
  uploaded: number;
  downloaded: number;
  ratio: number;
  seeder: boolean;
  ip?: string;
  port?: number;
  last_announce?: string;
  completed_at?: string;
}
