import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { Input, Select } from "@/components/form";
import { CategoryIcon } from "@/components/CategoryIcon";
import { MetadataFieldsTable } from "@/components/MetadataFieldsTable";
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

/**
 * Full-page create/edit form for a category. Replaces the former modal so the
 * category form and its metadata-fields table have room to breathe. There's no
 * single-category admin GET, so the existing list endpoint supplies both the
 * parent options and (for edit) the category being edited.
 */
export function AdminCategoryEditPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const toast = useToast();
  const isEdit = id != null;
  const categoryId = id ? Number(id) : null;

  const [loading, setLoading] = useState(true);
  const [notFound, setNotFound] = useState(false);
  const [form, setForm] = useState<CategoryFormData>(emptyForm);
  const [schema, setSchema] = useState<MetadataField[]>([]);
  const [parentOptions, setParentOptions] = useState<
    { value: string; label: string }[]
  >([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const token = getAccessToken();
      try {
        const res = await fetch(
          `${getConfig().API_URL}/api/v1/admin/categories`,
          { headers: { Authorization: `Bearer ${token}` } },
        );
        if (!res.ok) {
          // Creating doesn't require the list (only nice-to-have parent
          // options); only editing genuinely can't proceed without it.
          if (isEdit && !cancelled) setNotFound(true);
          return;
        }
        const data = await res.json();
        const cats: Category[] = data.categories ?? [];
        if (cancelled) return;

        // Top-level categories are the eligible parents (excluding self).
        setParentOptions(
          cats
            .filter((c) => c.parent_id == null && c.id !== categoryId)
            .map((c) => ({ value: String(c.id), label: c.name })),
        );

        if (isEdit) {
          const cat = cats.find((c) => c.id === categoryId);
          if (!cat) {
            setNotFound(true);
            return;
          }
          setForm({
            name: cat.name,
            slug: cat.slug,
            parent_id: cat.parent_id != null ? String(cat.parent_id) : "",
            image_url: cat.image_url ?? "",
            sort_order: String(cat.sort_order),
          });
          setSchema(cat.metadata_schema ?? []);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [categoryId, isEdit]);

  const handleSave = async () => {
    setSaving(true);
    const token = getAccessToken();
    const payload = {
      name: form.name,
      slug: form.slug,
      parent_id: form.parent_id ? Number(form.parent_id) : null,
      image_url: form.image_url.trim() || null,
      sort_order: Number(form.sort_order) || 0,
      metadata_schema: schema,
    };

    try {
      const url = isEdit
        ? `${getConfig().API_URL}/api/v1/admin/categories/${categoryId}`
        : `${getConfig().API_URL}/api/v1/admin/categories`;

      const res = await fetch(url, {
        method: isEdit ? "PUT" : "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(payload),
      });

      if (res.ok) {
        toast.success(
          isEdit
            ? "Category updated successfully"
            : "Category created successfully",
        );
        navigate("/admin/categories");
      } else {
        const data = await res.json().catch(() => null);
        toast.error(data?.error?.message ?? "Failed to save category");
      }
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <p>Loading...</p>;

  if (notFound) {
    return (
      <div>
        <p className="admin-categories__error">Category not found.</p>
        <Link className="admin-btn admin-btn--ghost" to="/admin/categories">
          Back to Categories
        </Link>
      </div>
    );
  }

  return (
    <div>
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-header__title">
            {isEdit ? "Edit Category" : "Add Category"}
          </h1>
          <p className="admin-page-header__desc">
            {isEdit
              ? "Update this category and its custom metadata fields."
              : "Create a category and define its custom metadata fields."}
          </p>
        </div>
        <div className="admin-page-header__actions">
          <Link className="admin-btn admin-btn--ghost" to="/admin/categories">
            Back
          </Link>
        </div>
      </div>

      <div className="admin-panel">
        <div className="admin-categories__form">
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

          <MetadataFieldsTable value={schema} onChange={setSchema} />

          <div className="admin-categories__form-actions">
            <Link className="admin-btn admin-btn--ghost" to="/admin/categories">
              Cancel
            </Link>
            <button
              className="admin-btn admin-btn--primary"
              onClick={handleSave}
              disabled={saving || !form.name.trim()}
            >
              {saving ? "Saving..." : "Save Category"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
