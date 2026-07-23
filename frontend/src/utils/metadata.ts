import { getConfig } from "@/config";

/** The field types an admin can attach to a category (mirrors backend internal/metadata). */
export type MetadataFieldType =
  | "text"
  | "number"
  | "select"
  | "multiselect"
  | "boolean";

/** A single metadata field definition, as returned by the resolve endpoint. */
export interface MetadataField {
  key: string;
  label: string;
  type: MetadataFieldType;
  required?: boolean;
  help?: string;
  // select / multiselect
  options?: string[];
  max_items?: number;
  // number
  min?: number;
  max?: number;
  integer?: boolean;
  // text
  max_length?: number;
  pattern?: string;
}

/** A torrent's metadata values, keyed by field key. Values are loosely typed
 *  because they cross the wire as JSON and are edited in form state. */
export type MetadataValues = Record<string, unknown>;

/**
 * Fetches a category's effective (inherited) metadata schema. Returns an empty
 * array on any failure so callers can render nothing rather than crash.
 */
export async function fetchCategorySchema(
  categoryId: number | string,
): Promise<MetadataField[]> {
  try {
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/categories/${encodeURIComponent(String(categoryId))}/metadata-schema`,
    );
    if (!res.ok) return [];
    const data = await res.json().catch(() => null);
    return (data?.fields as MetadataField[]) ?? [];
  } catch {
    return [];
  }
}

/**
 * Coerces raw form values into the typed object to submit. Numbers become
 * numbers, empty optional fields are dropped, multiselect stays an array, and
 * booleans are always sent. Server-side validation is the source of truth; this
 * just shapes the payload sensibly.
 */
export function cleanMetadataValues(
  schema: MetadataField[],
  values: MetadataValues,
): MetadataValues {
  const out: MetadataValues = {};
  for (const field of schema) {
    const v = values[field.key];
    switch (field.type) {
      case "number": {
        if (v === "" || v == null) continue;
        const n = typeof v === "number" ? v : Number(v);
        if (Number.isNaN(n)) continue;
        out[field.key] = n;
        break;
      }
      case "boolean": {
        out[field.key] = Boolean(v);
        break;
      }
      case "multiselect": {
        const arr = Array.isArray(v)
          ? v.filter((x) => x != null && x !== "")
          : [];
        if (arr.length > 0) out[field.key] = arr;
        break;
      }
      default: {
        // text, select
        const s = typeof v === "string" ? v.trim() : v == null ? "" : String(v);
        if (s !== "") out[field.key] = s;
      }
    }
  }
  return out;
}

// --- Auto-detect metadata from a torrent name (BE-3.13b) --------------------

/** Attributes we can infer from a scene-style torrent name. */
type DetectedAttr = "year" | "resolution" | "source" | "codec" | "audio";

// Candidate schema field keys per detected attribute. A category's admin picks
// the key; we fill whichever alias exists in the schema.
const FIELD_KEY_ALIASES: Record<DetectedAttr, string[]> = {
  year: ["year"],
  resolution: ["resolution", "quality", "res"],
  source: ["source", "medium", "rip"],
  codec: ["codec", "video_codec", "videocodec", "video"],
  audio: ["audio", "audio_codec", "audiocodec", "sound"],
};

// Ordered [pattern, canonical] pairs; the first match wins, so more specific
// tokens are listed before their broader fallbacks.
const RESOLUTION_PATTERNS: [RegExp, string][] = [
  [/\b4320p\b/i, "4320p"],
  [/\b2160p\b/i, "2160p"],
  [/\b(?:4k|uhd)\b/i, "2160p"],
  [/\b1440p\b/i, "1440p"],
  [/\b1080p\b/i, "1080p"],
  [/\b720p\b/i, "720p"],
  [/\b576p\b/i, "576p"],
  [/\b480p\b/i, "480p"],
];

const SOURCE_PATTERNS: [RegExp, string][] = [
  [/\bremux\b/i, "Remux"],
  [/\bblu-?ray\b/i, "BluRay"],
  [/\bbd-?rip\b/i, "BDRip"],
  [/\bbr-?rip\b/i, "BRRip"],
  [/\bweb-?dl\b/i, "WEB-DL"],
  [/\bweb-?rip\b/i, "WEBRip"],
  [/\bhdtv\b/i, "HDTV"],
  [/\bdvd-?rip\b/i, "DVDRip"],
  [/\bhd-?rip\b/i, "HDRip"],
  [/\bweb\b/i, "WEB"],
  [/\bdvd\b/i, "DVD"],
];

const CODEC_PATTERNS: [RegExp, string][] = [
  [/\bx265\b/i, "x265"],
  [/\bx264\b/i, "x264"],
  [/\bh\.?265\b/i, "h265"],
  [/\bh\.?264\b/i, "h264"],
  [/\bhevc\b/i, "HEVC"],
  [/\bav1\b/i, "AV1"],
  [/\bxvid\b/i, "XviD"],
  [/\bdivx\b/i, "DivX"],
  [/\bavc\b/i, "AVC"],
];

const AUDIO_PATTERNS: [RegExp, string][] = [
  [/\bdts-?hd\b/i, "DTS-HD"],
  [/\bdts\b/i, "DTS"],
  [/\btruehd\b/i, "TrueHD"],
  [/\batmos\b/i, "Atmos"],
  [/\b(?:eac3|ddp?\+|ddp)\b/i, "EAC3"],
  [/\b(?:ac3|dd5\.1|dd)\b/i, "AC3"],
  [/\baac\b/i, "AAC"],
  [/\bflac\b/i, "FLAC"],
  [/\bmp3\b/i, "MP3"],
];

function firstMatch(name: string, patterns: [RegExp, string][]): string | null {
  for (const [re, val] of patterns) {
    if (re.test(name)) return val;
  }
  return null;
}

/** Strip separators/case so "Blu-Ray", "BluRay" and "blu ray" all compare equal. */
function normalizeToken(s: string): string {
  return s.toLowerCase().replace(/[^a-z0-9]/g, "");
}

// applyToField shapes a detected canonical value to a schema field, returning
// undefined when the field can't sensibly accept it (unknown select option,
// non-numeric into a number field, booleans — which we never infer here).
function applyToField(field: MetadataField, raw: string | number): unknown {
  switch (field.type) {
    case "number": {
      const n = typeof raw === "number" ? raw : Number(raw);
      return Number.isNaN(n) ? undefined : n;
    }
    case "text":
      return String(raw);
    case "select":
    case "multiselect": {
      const target = normalizeToken(String(raw));
      const option = field.options?.find((o) => normalizeToken(o) === target);
      if (!option) return undefined;
      return field.type === "multiselect" ? [option] : option;
    }
    default:
      return undefined; // boolean
  }
}

/**
 * Infers metadata values from a scene-style torrent name (e.g.
 * `Movie.2024.1080p.BluRay.x265.DTS-HD`) and maps them onto the fields the
 * given category schema actually defines. Only fields present in the schema are
 * returned, and select/multiselect values are only set when they match one of
 * the field's options (adapting to the admin's exact option spelling). Purely a
 * UI convenience — the caller decides whether to apply the suggestions, and the
 * server still validates on submit.
 */
export function detectMetadataFromName(
  name: string,
  schema: MetadataField[],
): MetadataValues {
  const trimmed = name.trim();
  if (!trimmed || schema.length === 0) return {};

  const detected: Partial<Record<DetectedAttr, string | number>> = {};
  const yearMatch = trimmed.match(/\b(19\d{2}|20\d{2})\b/);
  if (yearMatch) detected.year = Number(yearMatch[1]);
  const resolution = firstMatch(trimmed, RESOLUTION_PATTERNS);
  if (resolution) detected.resolution = resolution;
  const source = firstMatch(trimmed, SOURCE_PATTERNS);
  if (source) detected.source = source;
  const codec = firstMatch(trimmed, CODEC_PATTERNS);
  if (codec) detected.codec = codec;
  const audio = firstMatch(trimmed, AUDIO_PATTERNS);
  if (audio) detected.audio = audio;

  const byKey = new Map(schema.map((f) => [f.key.toLowerCase(), f]));
  const out: MetadataValues = {};

  for (const attr of Object.keys(detected) as DetectedAttr[]) {
    const field = FIELD_KEY_ALIASES[attr]
      .map((k) => byKey.get(k))
      .find((f): f is MetadataField => f != null);
    if (!field) continue;
    const value = applyToField(field, detected[attr] as string | number);
    if (value !== undefined) out[field.key] = value;
  }
  return out;
}

/** Formats a stored value for read-only display on the detail page. */
export function formatMetadataValue(
  field: MetadataField,
  value: unknown,
): string {
  if (value == null || value === "") return "";
  if (field.type === "boolean") return value ? "Yes" : "No";
  if (field.type === "multiselect" && Array.isArray(value)) {
    return value.join(", ");
  }
  return String(value);
}

/** Human-readable label per field type, for admin field tables. */
export const METADATA_TYPE_LABELS: Record<MetadataFieldType, string> = {
  text: "Text",
  number: "Number",
  select: "Select",
  multiselect: "Multi-select",
  boolean: "Checkbox",
};

/** One-line summary of a field's type-specific constraints, for admin tables. */
export function metadataFieldDetails(field: MetadataField): string {
  switch (field.type) {
    case "select":
    case "multiselect": {
      const opts = (field.options ?? []).join(", ");
      const max =
        field.type === "multiselect" && field.max_items != null
          ? ` (max ${field.max_items})`
          : "";
      return opts ? `${opts}${max}` : "—";
    }
    case "number": {
      const parts: string[] = [];
      if (field.min != null) parts.push(`min ${field.min}`);
      if (field.max != null) parts.push(`max ${field.max}`);
      if (field.integer) parts.push("whole");
      return parts.length > 0 ? parts.join(", ") : "—";
    }
    case "text": {
      const parts: string[] = [];
      if (field.max_length != null)
        parts.push(`max length ${field.max_length}`);
      if (field.pattern) parts.push(`pattern ${field.pattern}`);
      return parts.length > 0 ? parts.join(", ") : "—";
    }
    default:
      return "—";
  }
}
