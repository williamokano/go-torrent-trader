// Pure drag-and-drop placement math for the category tree. Kept separate from
// the component so the tricky reordering logic is unit-testable without DOM
// drag events.
import { collectSubtreeIds, type TreeCategory } from "@/utils/categoryTree";

/** Where a drop lands relative to the target row. */
export type DropZone = "before" | "inside" | "after";

/** One category's new placement, matching the backend reorder payload. */
export interface ReorderItem {
  id: number;
  parent_id: number | null;
  sort_order: number;
}

/**
 * Maps a pointer offset within a row to a drop zone: the top and bottom bands
 * insert as a sibling before/after the target; the middle band nests inside it.
 */
export function dropZoneFromOffset(offsetY: number, height: number): DropZone {
  if (height <= 0) return "inside";
  const ratio = offsetY / height;
  if (ratio < 0.3) return "before";
  if (ratio > 0.7) return "after";
  return "inside";
}

/** True if dropping `draggedId` onto `targetId` would be a no-op or a cycle. */
export function isInvalidDrop<C extends TreeCategory>(
  categories: C[],
  draggedId: number,
  targetId: number,
): boolean {
  // Dropping onto the dragged node itself or any of its descendants would make
  // the node its own ancestor.
  return collectSubtreeIds(categories, draggedId).has(targetId);
}

const groupKey = (parentId: number | null): string =>
  parentId == null ? "root" : String(parentId);

/**
 * Computes the full new placement list after dragging `draggedId` onto
 * `targetId` at `zone`. Returns placements for *every* category (parent +
 * gap-free sort_order per sibling group) so the backend can apply the whole
 * tree atomically, or null if the move is invalid (self/descendant/no-op).
 */
export function computeReorder<C extends TreeCategory>(
  categories: C[],
  draggedId: number,
  targetId: number,
  zone: DropZone,
): ReorderItem[] | null {
  if (draggedId === targetId) return null;
  if (isInvalidDrop(categories, draggedId, targetId)) return null;

  const byId = new Map(categories.map((c) => [c.id, c]));
  const dragged = byId.get(draggedId);
  const target = byId.get(targetId);
  if (!dragged || !target) return null;

  // Ordered child ids per parent group, matching the tree's display order.
  const sorted = [...categories].sort(
    (a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name),
  );
  const groups = new Map<string, number[]>();
  for (const c of sorted) {
    const key = groupKey(c.parent_id);
    const arr = groups.get(key) ?? [];
    arr.push(c.id);
    groups.set(key, arr);
  }

  // Detach the dragged node from its current group.
  const fromArr = groups.get(groupKey(dragged.parent_id));
  if (fromArr) {
    const i = fromArr.indexOf(draggedId);
    if (i >= 0) fromArr.splice(i, 1);
  }

  // Re-insert at the requested position.
  let newParent: number | null;
  if (zone === "inside") {
    newParent = targetId;
    const arr = groups.get(groupKey(targetId)) ?? [];
    groups.set(groupKey(targetId), arr);
    arr.push(draggedId); // append as last child
  } else {
    newParent = target.parent_id;
    const arr = groups.get(groupKey(newParent)) ?? [];
    groups.set(groupKey(newParent), arr);
    const ti = arr.indexOf(targetId);
    const at = ti < 0 ? arr.length : zone === "before" ? ti : ti + 1;
    arr.splice(at, 0, draggedId);
  }

  const out: ReorderItem[] = [];
  for (const [key, ids] of groups) {
    const parent = key === "root" ? null : Number(key);
    ids.forEach((id, index) => {
      out.push({ id, parent_id: parent, sort_order: index });
    });
  }
  return out;
}
