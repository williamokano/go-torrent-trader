import { describe, it, expect } from "vitest";
import {
  buildCategoryTree,
  flattenCategoryTree,
  collectSubtreeIds,
  type TreeCategory,
} from "@/utils/categoryTree";

const cats: TreeCategory[] = [
  { id: 1, name: "Video", parent_id: null, sort_order: 2 },
  { id: 2, name: "Audio", parent_id: null, sort_order: 1 },
  { id: 3, name: "Movies", parent_id: 1, sort_order: 1 },
  { id: 4, name: "TV", parent_id: 1, sort_order: 2 },
  { id: 5, name: "HD Movies", parent_id: 3, sort_order: 1 },
];

describe("buildCategoryTree", () => {
  it("nests children and orders siblings by sort_order", () => {
    const roots = buildCategoryTree(cats);
    expect(roots.map((r) => r.category.name)).toEqual(["Audio", "Video"]);
    const video = roots[1];
    expect(video.children.map((c) => c.category.name)).toEqual([
      "Movies",
      "TV",
    ]);
    expect(video.children[0].children[0].category.name).toBe("HD Movies");
    expect(video.children[0].children[0].depth).toBe(2);
  });

  it("surfaces orphans (missing parent) as roots", () => {
    const roots = buildCategoryTree([
      { id: 9, name: "Orphan", parent_id: 999, sort_order: 0 },
    ]);
    expect(roots.map((r) => r.category.id)).toEqual([9]);
  });
});

describe("flattenCategoryTree", () => {
  it("returns depth-first display order with depth and hasChildren", () => {
    const flat = flattenCategoryTree(buildCategoryTree(cats));
    expect(flat.map((f) => f.category.name)).toEqual([
      "Audio",
      "Video",
      "Movies",
      "HD Movies",
      "TV",
    ]);
    const movies = flat.find((f) => f.category.name === "Movies")!;
    expect(movies.depth).toBe(1);
    expect(movies.hasChildren).toBe(true);
    expect(flat.find((f) => f.category.name === "TV")!.hasChildren).toBe(false);
  });
});

describe("collectSubtreeIds", () => {
  it("includes the node and all descendants", () => {
    expect([...collectSubtreeIds(cats, 1)].sort()).toEqual([1, 3, 4, 5]);
    expect([...collectSubtreeIds(cats, 3)].sort()).toEqual([3, 5]);
    expect([...collectSubtreeIds(cats, 5)].sort()).toEqual([5]);
  });
});
