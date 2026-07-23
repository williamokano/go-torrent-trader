import { describe, it, expect } from "vitest";
import {
  computeReorder,
  dropZoneFromOffset,
  isInvalidDrop,
} from "@/utils/categoryReorder";
import type { TreeCategory } from "@/utils/categoryTree";

// A: root(0) with children A1(0), A2(1); B: root(1)
const cats: TreeCategory[] = [
  { id: 1, name: "A", parent_id: null, sort_order: 0 },
  { id: 2, name: "B", parent_id: null, sort_order: 1 },
  { id: 3, name: "A1", parent_id: 1, sort_order: 0 },
  { id: 4, name: "A2", parent_id: 1, sort_order: 1 },
];

// Convenience: map id -> {parent_id, sort_order} from a placement list.
function placementMap(items: ReturnType<typeof computeReorder>) {
  const m = new Map<number, { parent_id: number | null; sort_order: number }>();
  for (const it of items ?? []) {
    m.set(it.id, { parent_id: it.parent_id, sort_order: it.sort_order });
  }
  return m;
}

describe("dropZoneFromOffset", () => {
  it("maps top/middle/bottom bands to before/inside/after", () => {
    expect(dropZoneFromOffset(2, 40)).toBe("before");
    expect(dropZoneFromOffset(20, 40)).toBe("inside");
    expect(dropZoneFromOffset(38, 40)).toBe("after");
    expect(dropZoneFromOffset(5, 0)).toBe("inside"); // guard against 0 height
  });
});

describe("isInvalidDrop", () => {
  it("blocks dropping onto self or a descendant", () => {
    expect(isInvalidDrop(cats, 1, 1)).toBe(true); // self
    expect(isInvalidDrop(cats, 1, 3)).toBe(true); // A onto its child A1
    expect(isInvalidDrop(cats, 3, 2)).toBe(false); // A1 onto B is fine
  });
});

describe("computeReorder", () => {
  it("returns null for no-op or cycle-forming drops", () => {
    expect(computeReorder(cats, 1, 1, "before")).toBeNull();
    expect(computeReorder(cats, 1, 3, "inside")).toBeNull(); // onto own child
  });

  it("re-parents a node inside a target (appended last)", () => {
    // Drag B inside A -> B becomes A's last child (after A1, A2).
    const m = placementMap(computeReorder(cats, 2, 1, "inside"));
    expect(m.get(2)).toEqual({ parent_id: 1, sort_order: 2 });
    // A's existing children keep their order.
    expect(m.get(3)).toEqual({ parent_id: 1, sort_order: 0 });
    expect(m.get(4)).toEqual({ parent_id: 1, sort_order: 1 });
  });

  it("reorders siblings with 'before'", () => {
    // Drag A2 before A1 -> A2 first, A1 second within parent A.
    const m = placementMap(computeReorder(cats, 4, 3, "before"));
    expect(m.get(4)).toEqual({ parent_id: 1, sort_order: 0 });
    expect(m.get(3)).toEqual({ parent_id: 1, sort_order: 1 });
  });

  it("moves a node out to a new parent with 'after'", () => {
    // Drag A1 after B at the root -> A1 becomes a root, right after B.
    const m = placementMap(computeReorder(cats, 3, 2, "after"));
    expect(m.get(3)?.parent_id).toBeNull();
    // Roots are A(0), B(1), A1(2) in order.
    expect(m.get(1)).toEqual({ parent_id: null, sort_order: 0 });
    expect(m.get(2)).toEqual({ parent_id: null, sort_order: 1 });
    expect(m.get(3)?.sort_order).toBe(2);
    // A2 remains A's only child, renumbered to 0.
    expect(m.get(4)).toEqual({ parent_id: 1, sort_order: 0 });
  });

  it("emits a placement for every category exactly once", () => {
    const items = computeReorder(cats, 2, 1, "inside")!;
    expect(items.map((i) => i.id).sort()).toEqual([1, 2, 3, 4]);
  });
});
