/** One announce, as the client reported it. Retained only for `retention_days`. */
export interface AnnounceLogEntry {
  id: number;
  torrent_id: number | null;
  torrent_name: string;
  event: string;
  /** The port the client listened on. There is no address: it is not retained. */
  port: number;
  /** Hex-encoded — the raw peer ID is arbitrary bytes. */
  peer_id: string;
  uploaded: number;
  downloaded: number;
  left_bytes: number;
  uploaded_delta: number;
  downloaded_delta: number;
  counted_downloaded_delta: number;
  seeder: boolean;
  announced_at: string;
}

/**
 * One calendar month (UTC) of totals. Aggregated before the raw rows are pruned
 * and kept permanently, so these months reach further back than the log itself.
 */
export interface AnnounceLogPeriod {
  year_month: string;
  uploaded: number;
  downloaded: number;
  counted_downloaded: number;
  announces: number;
  seed_announces: number;
  ratio: number;
}

export interface AnnounceLogResponse {
  events: AnnounceLogEntry[];
  total: number;
  page: number;
  per_page: number;
  monthly: AnnounceLogPeriod[];
  /** 0 means pruning is disabled and raw rows are kept indefinitely. */
  retention_days: number;
}
