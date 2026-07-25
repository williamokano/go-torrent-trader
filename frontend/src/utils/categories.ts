import {
  buildCategoryTree,
  flattenCategoryTree,
  type TreeCategory,
} from "@/utils/categoryTree";

/** One level of indentation, in non-breaking spaces so an <option> keeps it. */
const INDENT = "\u00a0\u00a0";

interface CategoryOption {
  value: string;
  label: string;
}

/**
 * Builds the options for a category `<select>`, in tree order with the full path
 * on every label.
 *
 * Two things make the label what it is. A child's own name is not unique — "Dub"
 * and "Sub" and "1080p" sit under several parents — so the label carries the
 * whole ancestor path; and in a multi-select the parent row may be scrolled out
 * of sight, so indentation alone would not answer "dub of what?". The path is
 * therefore repeated rather than implied.
 *
 * Order is the tree's: roots by `sort_order` then name, each one immediately
 * followed by its own descendants. It comes from buildCategoryTree rather than
 * from the caller's array order, so a caller that passes an unsorted list still
 * gets a sorted picker.
 *
 * Example output (leading spaces are non-breaking):
 *   Movies
 *     Movies / Action
 *       Movies / Action / 4K
 *     Movies / Comedy
 *   TV Shows
 *     TV Shows / Anime
 *
 * `placeholder` adds a leading empty-valued row ("All categories", "Select a
 * category"). Omit it for a multi-select, where an empty row would be selectable
 * and mean nothing.
 */
export function buildCategoryOptions(
  categories: TreeCategory[],
  placeholder?: string,
): CategoryOption[] {
  const options: CategoryOption[] =
    placeholder === undefined ? [] : [{ value: "", label: placeholder }];

  const pathById = new Map<number, string>();

  for (const { category, depth } of flattenCategoryTree(
    buildCategoryTree(categories),
  )) {
    // Depth-first order guarantees a parent is emitted before its children, so
    // its path is already known here. An orphan surfaces as a root and simply
    // has no prefix to inherit.
    const parentPath =
      category.parent_id == null ? undefined : pathById.get(category.parent_id);
    const path =
      parentPath === undefined
        ? category.name
        : `${parentPath} / ${category.name}`;
    pathById.set(category.id, path);

    options.push({
      value: String(category.id),
      // Indented with non-breaking spaces rather than a dash or arrow: an
      // <option> collapses ordinary spaces, while a punctuation prefix is read
      // out by screen readers ("em dash em dash Movies slash…") in front of the
      // content it is only there to lay out.
      label: `${INDENT.repeat(depth)}${path}`,
    });
  }

  return options;
}
