import { useCallback, useEffect, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { Input } from "@/components/form";
import { Select } from "@/components/form";
import { Modal } from "@/components/modal/Modal";
import { CategoryIcon } from "@/components/CategoryIcon";
import "./admin-categories.css";

interface Category {
  id: number;
  name: string;
  slug: string;
  parent_id: number | null;
  image_url: string | null;
  sort_order: number;
  created_at: string;
  updated_at: string;
}

interface CategoryFormData {
  name: string;
  slug: string;
  parent_id: string;
  image_url: string;
  sort_order: string;
}

const emptyForm: CategoryFormData = {
  name: "",
  slug: "",
  parent_id: "",
  image_url: "",
  sort_order: "0",
};

export function AdminCategoriesPage() {
  const toast = useToast();
  const [categories, setCategories] = useState<Category[]>([]);
  const [loading, setLoading] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [form, setForm] = useState<CategoryFormData>(emptyForm);
  const [saving, setSaving] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

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

  const openCreateModal = () => {
    setEditingId(null);
    setForm(emptyForm);
    setModalOpen(true);
  };

  const openEditModal = (cat: Category) => {
    setEditingId(cat.id);
    setForm({
      name: cat.name,
      slug: cat.slug,
      parent_id: cat.parent_id != null ? String(cat.parent_id) : "",
      image_url: cat.image_url ?? "",
      sort_order: String(cat.sort_order),
    });
    setModalOpen(true);
  };

  const closeModal = () => {
    setModalOpen(false);
    setEditingId(null);
    setForm(emptyForm);
  };

  const handleSave = async () => {
    setSaving(true);
    const token = getAccessToken();
    const payload = {
      name: form.name,
      slug: form.slug,
      parent_id: form.parent_id ? Number(form.parent_id) : null,
      image_url: form.image_url.trim() || null,
      sort_order: Number(form.sort_order) || 0,
    };

    try {
      const url = editingId
        ? `${getConfig().API_URL}/api/v1/admin/categories/${editingId}`
        : `${getConfig().API_URL}/api/v1/admin/categories`;

      const res = await fetch(url, {
        method: editingId ? "PUT" : "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(payload),
      });

      if (res.ok) {
        toast.success(
          editingId
            ? "Category updated successfully"
            : "Category created successfully",
        );
        closeModal();
        fetchCategories();
      } else {
        const data = await res.json();
        toast.error(data?.error?.message ?? "Failed to save category");
      }
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: number) => {
    setDeleteError(null);
    if (!window.confirm("Are you sure you want to delete this category?")) {
      return;
    }

    const token = getAccessToken();
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/categories/${id}`,
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
  };

  const parentOptions = categories
    .filter((c) => c.parent_id == null)
    .map((c) => ({
      value: String(c.id),
      label: c.name,
    }));

  const getCategoryName = (id: number | null): string => {
    if (id == null) return "-";
    const cat = categories.find((c) => c.id === id);
    return cat ? cat.name : String(id);
  };

  if (loading) return <p>Loading...</p>;

  return (
    <div>
      <div className="admin-categories__header">
        <h1>Categories</h1>
        <button className="admin-categories__add-btn" onClick={openCreateModal}>
          Add Category
        </button>
      </div>

      {deleteError && <p className="admin-categories__error">{deleteError}</p>}

      {categories.length === 0 ? (
        <p className="admin-categories__empty">No categories found.</p>
      ) : (
        <table className="admin-categories__table">
          <thead>
            <tr>
              <th>Image</th>
              <th>Name</th>
              <th>Slug</th>
              <th>Parent</th>
              <th>Sort Order</th>
              <th>Actions</th>
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
                <td>{cat.name}</td>
                <td>{cat.slug}</td>
                <td className="admin-categories__parent">
                  {getCategoryName(cat.parent_id)}
                </td>
                <td>{cat.sort_order}</td>
                <td className="admin-categories__actions">
                  <button
                    className="admin-categories__edit-btn"
                    onClick={() => openEditModal(cat)}
                  >
                    Edit
                  </button>
                  <button
                    className="admin-categories__delete-btn"
                    onClick={() => handleDelete(cat.id)}
                  >
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <Modal
        isOpen={modalOpen}
        onClose={closeModal}
        title={editingId ? "Edit Category" : "Add Category"}
      >
        <div className="admin-categories__modal-form">
          <Input
            label="Name"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
          />
          <Input
            label="Slug"
            value={form.slug}
            placeholder="Auto-generated from name if empty"
            onChange={(e) => setForm({ ...form, slug: e.target.value })}
          />
          <Select
            label="Parent Category"
            options={[
              { value: "", label: "None (top-level)" },
              ...parentOptions,
            ]}
            value={form.parent_id}
            onChange={(e) => setForm({ ...form, parent_id: e.target.value })}
          />
          <Input
            label="Image URL"
            value={form.image_url}
            placeholder="https://example.com/icon.png"
            onChange={(e) => setForm({ ...form, image_url: e.target.value })}
          />
          {form.image_url.trim() && (
            <div className="admin-categories__image-preview">
              <CategoryIcon
                name={form.name || "?"}
                imageUrl={form.image_url.trim()}
                size="lg"
              />
            </div>
          )}
          <Input
            label="Sort Order"
            type="number"
            value={form.sort_order}
            onChange={(e) => setForm({ ...form, sort_order: e.target.value })}
          />
          <div className="admin-categories__modal-actions">
            <button
              className="admin-categories__cancel-btn"
              onClick={closeModal}
            >
              Cancel
            </button>
            <button
              className="admin-categories__save-btn"
              onClick={handleSave}
              disabled={saving || !form.name.trim()}
            >
              {saving ? "Saving..." : "Save"}
            </button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
