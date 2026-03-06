export interface TorrentListItem {
  id: number;
  name: string;
  category_id: number;
  size: number;
  seeders: number;
  leechers: number;
  times_completed: number;
  created_at: string;
  free: boolean;
  uploader: string;
}
