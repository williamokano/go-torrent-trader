// Helpers for rendering categories hierarchically. Pure and generic over any
// shape that carries the id / parent_id / sort_order / name fields, so both the
// admin list (tree view) and the edit page (indented parent picker) can share
// them.

export interface TreeCategory {
  id: number;
  name: string;
  parent_id: number | null;
  sort_order: number;
}

export interface CategoryTreeNode<C extends TreeCategory> {
  category: C;
  depth: number;
  children: CategoryTreeNode<C>[];
}

function bySortThenName<C extends TreeCategory>(
  a: CategoryTreeNode<C>,
  b: CategoryTreeNode<C>,
): number {
  return (
    a.category.sort_order - b.category.sort_order ||
    a.category.name.localeCompare(b.category.name)
  );
}

/**
 * Builds a forest from a flat category list. Roots are categories with no
 * parent (or whose parent isn't in the list — orphans surface at the top rather
 * than vanishing). Siblings are ordered by sort_order then name, and each
 * node's depth is filled in. Cycle-safe: a node is only ever attached once, so
 * a malformed parent chain can't loop.
 */
export function buildCategoryTree<C extends TreeCategory>(
  categories: C[],
): CategoryTreeNode<C>[] {
  const nodes = new Map<number, CategoryTreeNode<C>>();
  for (const category of categories) {
    nodes.set(category.id, { category, depth: 0, children: [] });
  }

  const roots: CategoryTreeNode<C>[] = [];
  for (const node of nodes.values()) {
    const parentId = node.category.parent_id;
    const parent = parentId != null ? nodes.get(parentId) : undefined;
    if (parent && parent !== node) {
      parent.children.push(node);
    } else {
      roots.push(node);
    }
  }

  const assignDepth = (list: CategoryTreeNode<C>[], depth: number) => {
    list.sort(bySortThenName);
    for (const node of list) {
      node.depth = depth;
      assignDepth(node.children, depth + 1);
    }
  };
  assignDepth(roots, 0);

  return roots;
}

/** Depth-first flatten of a forest into display order, carrying depth. */
export function flattenCategoryTree<C extends TreeCategory>(
  roots: CategoryTreeNode<C>[],
): { category: C; depth: number; hasChildren: boolean }[] {
  const out: { category: C; depth: number; hasChildren: boolean }[] = [];
  const walk = (nodes: CategoryTreeNode<C>[]) => {
    for (const node of nodes) {
      out.push({
        category: node.category,
        depth: node.depth,
        hasChildren: node.children.length > 0,
      });
      walk(node.children);
    }
  };
  walk(roots);
  return out;
}

/**
 * Returns the id of `rootId` plus all of its descendants. Used to keep a
 * category from being made a child of itself or one of its own descendants
 * (which would create a cycle).
 */
export function collectSubtreeIds<C extends TreeCategory>(
  categories: C[],
  rootId: number,
): Set<number> {
  const childrenByParent = new Map<number, number[]>();
  for (const c of categories) {
    if (c.parent_id != null) {
      const siblings = childrenByParent.get(c.parent_id) ?? [];
      siblings.push(c.id);
      childrenByParent.set(c.parent_id, siblings);
    }
  }

  const ids = new Set<number>();
  const stack = [rootId];
  while (stack.length > 0) {
    const id = stack.pop() as number;
    if (ids.has(id)) continue; // guard against malformed cycles
    ids.add(id);
    for (const childId of childrenByParent.get(id) ?? []) {
      stack.push(childId);
    }
  }
  return ids;
}
