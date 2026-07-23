import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { ConfirmModal } from "@/components/modal/ConfirmModal";
import { CategoryIcon } from "@/components/CategoryIcon";
import { buildCategoryTree, type CategoryTreeNode } from "@/utils/categoryTree";
import {
  computeReorder,
  dropZoneFromOffset,
  isInvalidDrop,
  type DropZone,
  type ReorderItem,
} from "@/utils/categoryReorder";
import type { MetadataField } from "@/utils/metadata";
import "./admin-ui.css";
import "./admin-categories.css";

interface Category {
  id: number;
  name: string;
  slug: string;
  parent_id: number | null;
  image_url: string | null;
  sort_order: number;
  metadata_schema?: MetadataField[];
  created_at: string;
  updated_at: string;
}

interface VisibleRow {
  category: Category;
  depth: number;
  hasChildren: boolean;
}

export function AdminCategoriesPage() {
  const toast = useToast();
  const navigate = useNavigate();
  const [categories, setCategories] = useState<Category[]>([]);
  const [loading, setLoading] = useState(true);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [deletingCategory, setDeletingCategory] = useState<Category | null>(
    null,
  );
  const [deleting, setDeleting] = useState(false);
  // Collapsed node ids; empty means the whole tree is expanded by default.
  const [collapsed, setCollapsed] = useState<Set<number>>(new Set());
  // Drag-and-drop reorder state.
  const [draggedId, setDraggedId] = useState<number | null>(null);
  const [dropTarget, setDropTarget] = useState<{
    id: number;
    zone: DropZone;
  } | null>(null);

  const fetchCategories = useCallback(async () => {
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/categories`,
        {
          headers: { Authorization: `Bearer ${token}` },
        },
      );
      if (res.ok) {
        const data = await res.json();
        setCategories(data.categories ?? []);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchCategories();
  }, [fetchCategories]);

  const tree = useMemo(() => buildCategoryTree(categories), [categories]);

  // Flatten the tree into the rows currently visible, honoring collapsed nodes.
  const rows = useMemo(() => {
    const out: VisibleRow[] = [];
    const walk = (nodes: CategoryTreeNode<Category>[]) => {
      for (const node of nodes) {
        const hasChildren = node.children.length > 0;
        out.push({ category: node.category, depth: node.depth, hasChildren });
        if (hasChildren && !collapsed.has(node.category.id)) {
          walk(node.children);
        }
      }
    };
    walk(tree);
    return out;
  }, [tree, collapsed]);

  const toggle = (id: number) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  // Persist a reorder: apply optimistically, then send the batch; on failure,
  // surface the error and refetch to fall back to the server's truth.
  const applyReorder = useCallback(
    async (items: ReorderItem[]) => {
      setCategories((prev) => {
        const byId = new Map(items.map((it) => [it.id, it]));
        return prev.map((c) => {
          const it = byId.get(c.id);
          return it
            ? { ...c, parent_id: it.parent_id, sort_order: it.sort_order }
            : c;
        });
      });

      const token = getAccessToken();
      try {
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/admin/categories/reorder`,
          {
            method: "PUT",
            headers: {
              "Content-Type": "application/json",
              Authorization: `Bearer ${token}`,
            },
            body: JSON.stringify({ items }),
          },
        );
        if (!res.ok) {
          const data = await res.json().catch(() => null);
          toast.error(data?.error?.message ?? "Failed to reorder categories");
          fetchCategories();
        }
      } catch {
        toast.error("Failed to reorder categories");
        fetchCategories();
      }
    },
    [toast, fetchCategories],
  );

  const handleDragStart = (e: React.DragEvent, id: number) => {
    setDraggedId(id);
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = "move";
      e.dataTransfer.setData("text/plain", String(id));
    }
  };

  const handleDragOver = (e: React.DragEvent, id: number) => {
    if (draggedId == null) return;
    e.preventDefault();
    if (isInvalidDrop(categories, draggedId, id)) {
      if (e.dataTransfer) e.dataTransfer.dropEffect = "none";
      setDropTarget(null);
      return;
    }
    const rect = e.currentTarget.getBoundingClientRect();
    const zone = dropZoneFromOffset(e.clientY - rect.top, rect.height);
    if (e.dataTransfer) e.dataTransfer.dropEffect = "move";
    setDropTarget({ id, zone });
  };

  const handleDrop = (e: React.DragEvent, id: number) => {
    e.preventDefault();
    const zone = dropTarget?.id === id ? dropTarget.zone : "inside";
    const dragged = draggedId;
    setDraggedId(null);
    setDropTarget(null);
    if (dragged == null) return;
    const items = computeReorder(categories, dragged, id, zone);
    if (items) applyReorder(items);
  };

  const handleDragEnd = () => {
    setDraggedId(null);
    setDropTarget(null);
  };

  const rowClass = (id: number): string => {
    const classes: string[] = [];
    if (draggedId === id) classes.push("cat-tree__row--dragging");
    if (dropTarget?.id === id) {
      classes.push(`cat-tree__row--drop-${dropTarget.zone}`);
    }
    return classes.join(" ");
  };

  const handleDelete = async () => {
    if (!deletingCategory) return;
    setDeleteError(null);
    setDeleting(true);

    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/categories/${deletingCategory.id}`,
        {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        },
      );

      if (res.ok || res.status === 204) {
        toast.success("Category deleted");
        fetchCategories();
      } else {
        const data = await res.json().catch(() => null);
        const msg = data?.error?.message ?? "Failed to delete category";
        setDeleteError(msg);
        toast.error(msg);
      }
    } finally {
      setDeleting(false);
      setDeletingCategory(null);
    }
  };

  if (loading) return <p>Loading...</p>;

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title">Categories</h1>
          <p className="admin-page-header__desc">
            Organize torrents into a browsable category hierarchy. Drag a row to
            reorder it, or drop it onto another category to nest it.
          </p>
        </div>
        <div className="admin-page-header__actions">
          <button
            className="admin-btn admin-btn--primary"
            onClick={() => navigate("/admin/categories/new")}
          >
            Add Category
          </button>
        </div>
      </div>

      {deleteError && <p className="admin-categories__error">{deleteError}</p>}

      {categories.length === 0 ? (
        <div className="admin-panel">
          <p className="admin-empty">No categories found.</p>
        </div>
      ) : (
        <div className="admin-panel">
          <div className="admin-table-scroll">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Slug</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {rows.map(({ category, depth, hasChildren }) => (
                  <tr
                    key={category.id}
                    className={rowClass(category.id)}
                    draggable
                    onDragStart={(e) => handleDragStart(e, category.id)}
                    onDragOver={(e) => handleDragOver(e, category.id)}
                    onDrop={(e) => handleDrop(e, category.id)}
                    onDragEnd={handleDragEnd}
                  >
                    <td>
                      <div
                        className="cat-tree__cell"
                        style={{ paddingLeft: `${depth * 1.5}rem` }}
                      >
                        <span
                          className="cat-tree__grip"
                          aria-hidden="true"
                          title="Drag to reorder"
                        >
                          ⠿
                        </span>
                        {hasChildren ? (
                          <button
                            type="button"
                            className="cat-tree__toggle"
                            aria-label={
                              collapsed.has(category.id) ? "Expand" : "Collapse"
                            }
                            aria-expanded={!collapsed.has(category.id)}
                            onClick={() => toggle(category.id)}
                          >
                            {collapsed.has(category.id) ? "▸" : "▾"}
                          </button>
                        ) : (
                          <span className="cat-tree__toggle cat-tree__toggle--leaf" />
                        )}
                        <CategoryIcon
                          name={category.name}
                          imageUrl={category.image_url}
                          size="sm"
                        />
                        <span className="admin-table__name">
                          {category.name}
                        </span>
                      </div>
                    </td>
                    <td className="admin-muted">{category.slug}</td>
                    <td className="admin-table__actions">
                      <button
                        className="admin-btn admin-btn--ghost admin-btn--sm"
                        onClick={() =>
                          navigate(
                            `/admin/categories/new?parent=${category.id}`,
                          )
                        }
                      >
                        Add sub
                      </button>{" "}
                      <button
                        className="admin-btn admin-btn--ghost admin-btn--sm"
                        onClick={() =>
                          navigate(`/admin/categories/${category.id}/edit`)
                        }
                      >
                        Edit
                      </button>{" "}
                      <button
                        className="admin-btn admin-btn--danger admin-btn--sm"
                        onClick={() => {
                          setDeleteError(null);
                          setDeletingCategory(category);
                        }}
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      <ConfirmModal
        isOpen={deletingCategory !== null}
        title="Delete Category"
        message={`Delete the "${deletingCategory?.name}" category? This cannot be undone.`}
        confirmLabel="Delete"
        danger
        loading={deleting}
        onConfirm={handleDelete}
        onCancel={() => setDeletingCategory(null)}
      />
    </div>
  );
}
