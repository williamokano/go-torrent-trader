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
