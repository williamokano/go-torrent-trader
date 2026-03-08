interface RawCategory {
  id: number;
  name: string;
  parent_id: number | null;
  sort_order: number;
}

interface CategoryOption {
  value: string;
  label: string;
}

/**
 * Builds a flat list of category options with indented labels for hierarchy.
 * Top-level categories appear first, children are indented with a prefix.
 *
 * Example output:
 *   Movies
 *   — Movies / Action
 *   — Movies / Comedy
 *   TV Shows
 *   — TV Shows / Anime
 */
export function buildCategoryOptions(
  categories: RawCategory[],
  placeholder: string,
): CategoryOption[] {
  const byId = new Map<number, RawCategory>();
  for (const c of categories) {
    byId.set(c.id, c);
  }

  const topLevel = categories.filter((c) => !c.parent_id);
  const children = categories.filter((c) => c.parent_id);
  const childrenByParent = new Map<number, RawCategory[]>();
  for (const c of children) {
    const list = childrenByParent.get(c.parent_id!) ?? [];
    list.push(c);
    childrenByParent.set(c.parent_id!, list);
  }

  const options: CategoryOption[] = [{ value: "", label: placeholder }];

  for (const parent of topLevel) {
    options.push({ value: String(parent.id), label: parent.name });
    const kids = childrenByParent.get(parent.id) ?? [];
    for (const child of kids) {
      options.push({
        value: String(child.id),
        label: `— ${parent.name} / ${child.name}`,
      });
    }
  }

  return options;
}
