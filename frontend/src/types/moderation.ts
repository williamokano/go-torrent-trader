/**
 * Torrent moderation state (BE-8.22). Not part of the generated OpenAPI Torrent
 * schema, so it is accessed via a cast on the torrent response object.
 */
export type ModerationStatus = "pending" | "approved" | "rejected";

export interface TorrentModeration {
  status: ModerationStatus;
  assigned_moderator_id?: number;
  assigned_moderator_name?: string;
  approved_by_id?: number;
  approved_by_name?: string;
  approved_at?: string;
  message_count?: number;
}

/**
 * A torrent as it appears in the moderation queue, with the fields the queue and
 * review UI need.
 */
export interface ModeratedTorrent {
  id: number;
  name: string;
  uploader_id?: number;
  uploader_name?: string;
  anonymous?: boolean;
  category_name?: string;
  size?: number;
  created_at?: string;
  moderation?: TorrentModeration;
}
