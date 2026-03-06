import type { components } from "@/api";

/**
 * Torrent shape as returned by the API.
 * All fields are optional in the OpenAPI schema, but we use the type directly
 * so consumers handle optionality explicitly.
 */
export type Torrent = components["schemas"]["Torrent"];

/**
 * Response shape from GET /api/v1/torrents
 */
export interface TorrentListResponse {
  torrents?: Torrent[];
  total?: number;
  page?: number;
  per_page?: number;
}
