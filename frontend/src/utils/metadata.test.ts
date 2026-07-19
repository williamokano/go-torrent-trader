import { afterEach, describe, expect, it, vi } from "vitest";
import {
  cleanMetadataValues,
  formatMetadataValue,
  fetchCategorySchema,
  type MetadataField,
} from "@/utils/metadata";

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://test.local" }),
}));

const schema: MetadataField[] = [
  { key: "year", label: "Year", type: "number", integer: true },
  { key: "title", label: "Title", type: "text" },
  { key: "codec", label: "Codec", type: "select", options: ["x264", "x265"] },
  {
    key: "audio",
    label: "Audio",
    type: "multiselect",
    options: ["FLAC", "AC3"],
  },
  { key: "hdr", label: "HDR", type: "boolean" },
];

describe("cleanMetadataValues", () => {
  it("coerces numbers and drops empty optional fields", () => {
    const out = cleanMetadataValues(schema, { year: "2024", title: "  " });
    expect(out).toEqual({ year: 2024, hdr: false });
  });

  it("drops non-numeric number input", () => {
    const out = cleanMetadataValues(schema, { year: "abc" });
    expect(out.year).toBeUndefined();
  });

  it("keeps multiselect arrays and trims text", () => {
    const out = cleanMetadataValues(schema, {
      title: "  A Movie  ",
      audio: ["FLAC", "AC3"],
    });
    expect(out.title).toBe("A Movie");
    expect(out.audio).toEqual(["FLAC", "AC3"]);
  });

  it("always includes boolean values", () => {
    expect(cleanMetadataValues(schema, { hdr: true }).hdr).toBe(true);
    expect(cleanMetadataValues(schema, {}).hdr).toBe(false);
  });

  it("drops empty multiselect", () => {
    const out = cleanMetadataValues(schema, { audio: [] });
    expect(out.audio).toBeUndefined();
  });
});

describe("formatMetadataValue", () => {
  it("renders booleans as Yes/No", () => {
    expect(formatMetadataValue(schema[4], true)).toBe("Yes");
    expect(formatMetadataValue(schema[4], false)).toBe("No");
  });

  it("joins multiselect values", () => {
    expect(formatMetadataValue(schema[3], ["FLAC", "AC3"])).toBe("FLAC, AC3");
  });

  it("stringifies numbers and returns empty for null", () => {
    expect(formatMetadataValue(schema[0], 2024)).toBe("2024");
    expect(formatMetadataValue(schema[0], null)).toBe("");
  });
});

describe("fetchCategorySchema", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns the fields array on success", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          fields: [{ key: "year", label: "Year", type: "number" }],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    const fields = await fetchCategorySchema(5);
    expect(fields).toHaveLength(1);
    expect(fields[0].key).toBe("year");
  });

  it("returns [] on non-ok response", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("{}", { status: 500 }),
    );
    expect(await fetchCategorySchema(5)).toEqual([]);
  });

  it("returns [] when fetch throws", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("network"));
    expect(await fetchCategorySchema(5)).toEqual([]);
  });
});
