import { useCallback, useEffect, useState } from "react";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { Input } from "@/components/form";
import { Select } from "@/components/form";
import { Modal } from "@/components/modal/Modal";
import "./admin-forums.css";

interface Group {
  id: number;
  name: string;
  level: number;
}

interface ForumCategory {
  id: number;
  name: string;
  sort_order: number;
  created_at: string;
}

interface Forum {
  id: number;
  category_id: number;
  name: string;
  description: string;
  sort_order: number;
  topic_count: number;
  post_count: number;
  min_group_level: number;
  min_post_level: number;
  created_at: string;
}

interface CategoryFormData {
  name: string;
  sort_order: string;
}

interface ForumFormData {
  name: string;
  description: string;
  category_id: string;
  sort_order: string;
  min_group_level: string;
  min_post_level: string;
}

const emptyCategoryForm: CategoryFormData = {
  name: "",
  sort_order: "0",
};

const emptyForumForm: ForumFormData = {
  name: "",
  description: "",
  category_id: "",
  sort_order: "0",
  min_group_level: "0",
  min_post_level: "0",
};

export function AdminForumsPage() {
  const toast = useToast();
  const [categories, setCategories] = useState<ForumCategory[]>([]);
  const [forums, setForums] = useState<Forum[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [loading, setLoading] = useState(true);

  // Category modal state
  const [catModalOpen, setCatModalOpen] = useState(false);
  const [editingCatId, setEditingCatId] = useState<number | null>(null);
  const [catForm, setCatForm] = useState<CategoryFormData>(emptyCategoryForm);
  const [catSaving, setCatSaving] = useState(false);

  // Forum modal state
  const [forumModalOpen, setForumModalOpen] = useState(false);
  const [editingForumId, setEditingForumId] = useState<number | null>(null);
  const [forumForm, setForumForm] = useState<ForumFormData>(emptyForumForm);
  const [forumSaving, setForumSaving] = useState(false);

  const [deleteError, setDeleteError] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    const token = getAccessToken();
    try {
      const [catRes, forumRes, groupsRes] = await Promise.all([
        fetch(`${getConfig().API_URL}/api/v1/admin/forum-categories`, {
          headers: { Authorization: `Bearer ${token}` },
        }),
        fetch(`${getConfig().API_URL}/api/v1/admin/forums`, {
          headers: { Authorization: `Bearer ${token}` },
        }),
        fetch(`${getConfig().API_URL}/api/v1/admin/groups`, {
          headers: { Authorization: `Bearer ${token}` },
        }),
      ]);
      setDeleteError(null);
      if (catRes.ok) {
        const data = await catRes.json();
        setCategories(data.categories ?? []);
      } else {
        toast.error("Failed to load forum categories");
      }
      if (forumRes.ok) {
        const data = await forumRes.json();
        setForums(data.forums ?? []);
      } else {
        toast.error("Failed to load forums");
      }
      if (groupsRes.ok) {
        const data = await groupsRes.json();
        setGroups(data.groups ?? []);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  // Category CRUD
  const openCreateCatModal = () => {
    setEditingCatId(null);
    const nextSort =
      categories.length > 0
        ? Math.max(...categories.map((c) => c.sort_order)) + 1
        : 1;
    setCatForm({ ...emptyCategoryForm, sort_order: String(nextSort) });
    setCatModalOpen(true);
  };

  const openEditCatModal = (cat: ForumCategory) => {
    setEditingCatId(cat.id);
    setCatForm({
      name: cat.name,
      sort_order: String(cat.sort_order),
    });
    setCatModalOpen(true);
  };

  const closeCatModal = () => {
    setCatModalOpen(false);
    setEditingCatId(null);
    setCatForm(emptyCategoryForm);
  };

  const handleSaveCat = async () => {
    setCatSaving(true);
    const token = getAccessToken();
    const payload = {
      name: catForm.name,
      sort_order: Number(catForm.sort_order) || 0,
    };

    try {
      const url = editingCatId
        ? `${getConfig().API_URL}/api/v1/admin/forum-categories/${editingCatId}`
        : `${getConfig().API_URL}/api/v1/admin/forum-categories`;

      const res = await fetch(url, {
        method: editingCatId ? "PUT" : "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(payload),
      });

      if (res.ok) {
        toast.success(
          editingCatId
            ? "Category updated successfully"
            : "Category created successfully",
        );
        closeCatModal();
        fetchData();
      } else {
        const data = await res.json();
        toast.error(data?.error?.message ?? "Failed to save category");
      }
    } finally {
      setCatSaving(false);
    }
  };

  const handleDeleteCat = async (id: number) => {
    setDeleteError(null);
    if (
      !window.confirm("Are you sure you want to delete this forum category?")
    ) {
      return;
    }

    const token = getAccessToken();
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/forum-categories/${id}`,
      {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      },
    );

    if (res.ok || res.status === 204) {
      toast.success("Category deleted");
      fetchData();
    } else {
      const data = await res.json().catch(() => null);
      const msg = data?.error?.message ?? "Failed to delete category";
      setDeleteError(msg);
      toast.error(msg);
    }
  };

  // Forum CRUD
  const openCreateForumModal = () => {
    setEditingForumId(null);
    const nextSort =
      forums.length > 0 ? Math.max(...forums.map((f) => f.sort_order)) + 1 : 1;
    setForumForm({ ...emptyForumForm, sort_order: String(nextSort) });
    setForumModalOpen(true);
  };

  const openEditForumModal = (forum: Forum) => {
    setEditingForumId(forum.id);
    setForumForm({
      name: forum.name,
      description: forum.description,
      category_id: String(forum.category_id),
      sort_order: String(forum.sort_order),
      min_group_level: String(forum.min_group_level),
      min_post_level: String(forum.min_post_level),
    });
    setForumModalOpen(true);
  };

  const closeForumModal = () => {
    setForumModalOpen(false);
    setEditingForumId(null);
    setForumForm(emptyForumForm);
  };

  const handleSaveForum = async () => {
    setForumSaving(true);
    const token = getAccessToken();
    const payload = {
      name: forumForm.name,
      description: forumForm.description,
      category_id: Number(forumForm.category_id) || 0,
      sort_order: Number(forumForm.sort_order) || 0,
      min_group_level: Number(forumForm.min_group_level) || 0,
      min_post_level: Number(forumForm.min_post_level) || 0,
    };

    try {
      const url = editingForumId
        ? `${getConfig().API_URL}/api/v1/admin/forums/${editingForumId}`
        : `${getConfig().API_URL}/api/v1/admin/forums`;

      const res = await fetch(url, {
        method: editingForumId ? "PUT" : "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(payload),
      });

      if (res.ok) {
        toast.success(
          editingForumId
            ? "Forum updated successfully"
            : "Forum created successfully",
        );
        closeForumModal();
        fetchData();
      } else {
        const data = await res.json();
        toast.error(data?.error?.message ?? "Failed to save forum");
      }
    } finally {
      setForumSaving(false);
    }
  };

  const handleDeleteForum = async (id: number) => {
    setDeleteError(null);
    if (!window.confirm("Are you sure you want to delete this forum?")) {
      return;
    }

    const token = getAccessToken();
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/forums/${id}`,
      {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      },
    );

    if (res.ok || res.status === 204) {
      toast.success("Forum deleted");
      fetchData();
    } else {
      const data = await res.json().catch(() => null);
      const msg = data?.error?.message ?? "Failed to delete forum";
      setDeleteError(msg);
      toast.error(msg);
    }
  };

  const getCategoryName = (id: number): string => {
    const cat = categories.find((c) => c.id === id);
    return cat ? cat.name : String(id);
  };

  const categoryOptions = categories.map((c) => ({
    value: String(c.id),
    label: c.name,
  }));

  // Build unique level options from groups, sorted ascending
  const levelOptions = [
    ...new Map(
      groups
        .sort((a, b) => a.level - b.level)
        .map((g) => [
          g.level,
          { value: String(g.level), label: `${g.name} (level ${g.level})` },
        ]),
    ).values(),
  ];

  const getGroupNameByLevel = (level: number): string => {
    const group = groups.find((g) => g.level === level);
    return group ? group.name : String(level);
  };

  if (loading) return <p>Loading...</p>;

  return (
    <div>
      <h1>Forum Administration</h1>

      {deleteError && <p className="admin-forums__error">{deleteError}</p>}

      {/* Forum Categories Section */}
      <div className="admin-forums__section">
        <div className="admin-forums__section-header">
          <h2>Forum Categories</h2>
          <button
            className="admin-forums__add-btn"
            onClick={openCreateCatModal}
          >
            Add Category
          </button>
        </div>

        {categories.length === 0 ? (
          <p className="admin-forums__empty">No forum categories found.</p>
        ) : (
          <table className="admin-forums__table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Sort Order</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {categories.map((cat) => (
                <tr key={cat.id}>
                  <td>{cat.name}</td>
                  <td>{cat.sort_order}</td>
                  <td>
                    <button
                      className="admin-forums__edit-btn"
                      onClick={() => openEditCatModal(cat)}
                    >
                      Edit
                    </button>
                    <button
                      className="admin-forums__delete-btn"
                      onClick={() => handleDeleteCat(cat.id)}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Forums Section */}
      <div className="admin-forums__section">
        <div className="admin-forums__section-header">
          <h2>Forums</h2>
          <button
            className="admin-forums__add-btn"
            onClick={openCreateForumModal}
          >
            Add Forum
          </button>
        </div>

        {forums.length === 0 ? (
          <p className="admin-forums__empty">No forums found.</p>
        ) : (
          <table className="admin-forums__table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Category</th>
                <th>Description</th>
                <th>Sort</th>
                <th>Min View</th>
                <th>Min Post</th>
                <th>Topics</th>
                <th>Posts</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {forums.map((forum) => (
                <tr key={forum.id}>
                  <td>{forum.name}</td>
                  <td>{getCategoryName(forum.category_id)}</td>
                  <td className="admin-forums__description">
                    {forum.description || "-"}
                  </td>
                  <td>{forum.sort_order}</td>
                  <td>{getGroupNameByLevel(forum.min_group_level)}</td>
                  <td>{getGroupNameByLevel(forum.min_post_level)}</td>
                  <td>{forum.topic_count}</td>
                  <td>{forum.post_count}</td>
                  <td>
                    <button
                      className="admin-forums__edit-btn"
                      onClick={() => openEditForumModal(forum)}
                    >
                      Edit
                    </button>
                    <button
                      className="admin-forums__delete-btn"
                      onClick={() => handleDeleteForum(forum.id)}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Category Modal */}
      <Modal
        isOpen={catModalOpen}
        onClose={closeCatModal}
        title={editingCatId ? "Edit Forum Category" : "Add Forum Category"}
      >
        <div className="admin-forums__modal-form">
          <Input
            label="Name"
            value={catForm.name}
            onChange={(e) => setCatForm({ ...catForm, name: e.target.value })}
          />
          <Input
            label="Sort Order"
            type="number"
            value={catForm.sort_order}
            onChange={(e) =>
              setCatForm({ ...catForm, sort_order: e.target.value })
            }
          />
          <div className="admin-forums__modal-actions">
            <button
              className="admin-forums__cancel-btn"
              onClick={closeCatModal}
            >
              Cancel
            </button>
            <button
              className="admin-forums__save-btn"
              onClick={handleSaveCat}
              disabled={catSaving || !catForm.name.trim()}
            >
              {catSaving ? "Saving..." : "Save"}
            </button>
          </div>
        </div>
      </Modal>

      {/* Forum Modal */}
      <Modal
        isOpen={forumModalOpen}
        onClose={closeForumModal}
        title={editingForumId ? "Edit Forum" : "Add Forum"}
      >
        <div className="admin-forums__modal-form">
          <Input
            label="Name"
            value={forumForm.name}
            onChange={(e) =>
              setForumForm({ ...forumForm, name: e.target.value })
            }
          />
          <Input
            label="Description"
            value={forumForm.description}
            onChange={(e) =>
              setForumForm({ ...forumForm, description: e.target.value })
            }
          />
          <Select
            label="Category"
            options={[
              { value: "", label: "Select a category" },
              ...categoryOptions,
            ]}
            value={forumForm.category_id}
            onChange={(e) =>
              setForumForm({ ...forumForm, category_id: e.target.value })
            }
          />
          <Input
            label="Sort Order"
            type="number"
            value={forumForm.sort_order}
            onChange={(e) =>
              setForumForm({ ...forumForm, sort_order: e.target.value })
            }
          />
          <Select
            label="Min Group Level (view access)"
            options={levelOptions}
            value={forumForm.min_group_level}
            onChange={(e) =>
              setForumForm({ ...forumForm, min_group_level: e.target.value })
            }
          />
          <Select
            label="Min Post Level (create topics)"
            options={levelOptions}
            value={forumForm.min_post_level}
            onChange={(e) =>
              setForumForm({ ...forumForm, min_post_level: e.target.value })
            }
          />
          <div className="admin-forums__modal-actions">
            <button
              className="admin-forums__cancel-btn"
              onClick={closeForumModal}
            >
              Cancel
            </button>
            <button
              className="admin-forums__save-btn"
              onClick={handleSaveForum}
              disabled={
                forumSaving || !forumForm.name.trim() || !forumForm.category_id
              }
            >
              {forumSaving ? "Saving..." : "Save"}
            </button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
