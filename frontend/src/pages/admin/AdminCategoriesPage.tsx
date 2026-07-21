import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { ConfirmModal } from "@/components/modal/ConfirmModal";
import { CategoryIcon } from "@/components/CategoryIcon";
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

  const getCategoryName = (id: number | null): string => {
    if (id == null) return "-";
    const cat = categories.find((c) => c.id === id);
    return cat ? cat.name : String(id);
  };

  if (loading) return <p>Loading...</p>;

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title">Categories</h1>
          <p className="admin-page-header__desc">
            Organize torrents into browsable categories.
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
                  <th>Image</th>
                  <th>Name</th>
                  <th>Slug</th>
                  <th>Parent</th>
                  <th>Sort Order</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {categories.map((cat) => (
                  <tr key={cat.id}>
                    <td>
                      <CategoryIcon
                        name={cat.name}
                        imageUrl={cat.image_url}
                        size="md"
                      />
                    </td>
                    <td className="admin-table__name">{cat.name}</td>
                    <td className="admin-muted">{cat.slug}</td>
                    <td className="admin-muted">
                      {getCategoryName(cat.parent_id)}
                    </td>
                    <td className="admin-num">{cat.sort_order}</td>
                    <td className="admin-table__actions">
                      <button
                        className="admin-btn admin-btn--ghost admin-btn--sm"
                        onClick={() =>
                          navigate(`/admin/categories/${cat.id}/edit`)
                        }
                      >
                        Edit
                      </button>{" "}
                      <button
                        className="admin-btn admin-btn--danger admin-btn--sm"
                        onClick={() => {
                          setDeleteError(null);
                          setDeletingCategory(cat);
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
