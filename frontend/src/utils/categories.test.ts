import { describe, it, expect } from "vitest";
import { buildCategoryOptions } from "@/utils/categories";

/** One indent level, matching the non-breaking spaces the builder emits. */
const I = "\u00a0\u00a0";
import type { TreeCategory } from "@/utils/categoryTree";

// Deliberately out of display order, and with the same child name under two
// parents — the exact shape that made the connector picker unusable.
const cats: TreeCategory[] = [
  { id: 10, name: "Anime", parent_id: null, sort_order: 2 },
  { id: 11, name: "Dub", parent_id: 10, sort_order: 2 },
  { id: 12, name: "Sub", parent_id: 10, sort_order: 1 },
  { id: 20, name: "Movies", parent_id: null, sort_order: 1 },
  { id: 21, name: "Dub", parent_id: 20, sort_order: 1 },
];

describe("buildCategoryOptions", () => {
  it("orders by parent, then sort_order, and nests children under them", () => {
    expect(buildCategoryOptions(cats).map((o) => o.label)).toEqual([
      "Movies",
      `${I}Movies / Dub`,
      "Anime",
      `${I}Anime / Sub`,
      `${I}Anime / Dub`,
    ]);
  });

  it("distinguishes same-named children of different parents", () => {
    const labels = buildCategoryOptions(cats);
    const dubs = labels.filter((o) => o.label.endsWith("Dub"));
    expect(dubs.map((o) => o.label)).toEqual([
      `${I}Movies / Dub`,
      `${I}Anime / Dub`,
    ]);
    // And each still points at its own category.
    expect(dubs.map((o) => o.value)).toEqual(["21", "11"]);
  });

  it("does not depend on the caller's array order", () => {
    const shuffled = [...cats].reverse();
    expect(buildCategoryOptions(shuffled)).toEqual(buildCategoryOptions(cats));
  });

  it("keeps categories deeper than two levels", () => {
    // The previous implementation walked roots and their direct children only,
    // so a third level vanished from every picker on the site.
    const deep: TreeCategory[] = [
      ...cats,
      { id: 30, name: "1080p", parent_id: 21, sort_order: 1 },
    ];
    const labels = buildCategoryOptions(deep).map((o) => o.label);
    expect(labels).toContain(`${I}${I}Movies / Dub / 1080p`);
    expect(labels.indexOf(`${I}${I}Movies / Dub / 1080p`)).toBe(
      labels.indexOf(`${I}Movies / Dub`) + 1,
    );
  });

  it("surfaces an orphan rather than dropping it", () => {
    const orphaned: TreeCategory[] = [
      { id: 40, name: "Stray", parent_id: 999, sort_order: 0 },
    ];
    expect(buildCategoryOptions(orphaned)).toEqual([
      { value: "40", label: "Stray" },
    ]);
  });

  it("adds a placeholder row only when one is given", () => {
    expect(buildCategoryOptions(cats, "All categories")[0]).toEqual({
      value: "",
      label: "All categories",
    });
    // A multi-select gets no empty row: it would be selectable and mean nothing.
    expect(buildCategoryOptions(cats)[0].value).toBe("20");
  });

  it("returns nothing but the placeholder for an empty list", () => {
    expect(buildCategoryOptions([])).toEqual([]);
    expect(buildCategoryOptions([], "All categories")).toEqual([
      { value: "", label: "All categories" },
    ]);
  });
});
