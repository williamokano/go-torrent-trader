import { useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { useToast } from "@/components/toast";
import { Input } from "@/components/form";
import { Select } from "@/components/form";
import { Pagination } from "@/components/Pagination";
import { AdminUserEditModal } from "./AdminUserEditModal";
import { formatBytes, timeAgo } from "@/utils/format";
import "./admin-users.css";

interface AdminUser {
  id: number;
  username: string;
  email: string;
  group_id: number;
  group_name: string;
  avatar: string | null;
  title: string | null;
  info: string | null;
  uploaded: number;
  downloaded: number;
  enabled: boolean;
  warned: boolean;
  donor: boolean;
  parked: boolean;
  invites: number;
  created_at: string;
  last_access: string | null;
}

interface GroupOption {
  value: string;
  label: string;
}

const PER_PAGE = 25;

export function AdminUsersPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const toast = useToast();

  const query = searchParams.get("q") ?? "";
  const groupFilter = searchParams.get("group") ?? "";
  const enabledFilter = searchParams.get("enabled") ?? "";
  const page = Math.max(1, Number(searchParams.get("page")) || 1);

  const [searchInput, setSearchInput] = useState(query);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const [users, setUsers] = useState<AdminUser[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [groups, setGroups] = useState<GroupOption[]>([]);
  const [editingUser, setEditingUser] = useState<AdminUser | null>(null);

  // Fetch groups for filter and edit modal
  useEffect(() => {
    async function fetchGroups() {
      const token = getAccessToken();
      const res = await fetch(`${getConfig().API_URL}/api/v1/admin/groups`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setGroups(
          (data.groups ?? []).map((g: { id: number; name: string }) => ({
            value: String(g.id),
            label: g.name,
          })),
        );
      }
    }
    fetchGroups();
  }, []);

  const fetchUsers = useCallback(async () => {
    setLoading(true);
    const token = getAccessToken();
    const params = new URLSearchParams();
    if (query) params.set("search", query);
    if (groupFilter) params.set("group_id", groupFilter);
    if (enabledFilter) params.set("enabled", enabledFilter);
    params.set("page", String(page));
    params.set("per_page", String(PER_PAGE));

    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/admin/users?${params}`,
        {
          headers: { Authorization: `Bearer ${token}` },
        },
      );
      if (res.ok) {
        const data = await res.json();
        setUsers(data.users ?? []);
        setTotal(data.total ?? 0);
      }
    } finally {
      setLoading(false);
    }
  }, [query, groupFilter, enabledFilter, page]);

  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  const handleSearchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    setSearchInput(val);
    clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      const next = new URLSearchParams(searchParams);
      if (val) {
        next.set("q", val);
      } else {
        next.delete("q");
      }
      next.delete("page");
      setSearchParams(next);
    }, 250);
  };

  const handleGroupChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const next = new URLSearchParams(searchParams);
    if (e.target.value) {
      next.set("group", e.target.value);
    } else {
      next.delete("group");
    }
    next.delete("page");
    setSearchParams(next);
  };

  const handleEnabledChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const next = new URLSearchParams(searchParams);
    if (e.target.value) {
      next.set("enabled", e.target.value);
    } else {
      next.delete("enabled");
    }
    next.delete("page");
    setSearchParams(next);
  };

  const handlePageChange = (newPage: number) => {
    const next = new URLSearchParams(searchParams);
    next.set("page", String(newPage));
    setSearchParams(next);
  };

  const handleSaveUser = async (
    userId: number,
    data: { group_id?: number; enabled?: boolean; warned?: boolean },
  ) => {
    const token = getAccessToken();
    const res = await fetch(
      `${getConfig().API_URL}/api/v1/admin/users/${userId}`,
      {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(data),
      },
    );

    if (res.ok) {
      toast.success("User updated successfully");
      fetchUsers();
    } else {
      toast.error("Failed to update user");
    }
  };

  const totalPages = Math.ceil(total / PER_PAGE);

  return (
    <div>
      <div className="admin-users__header">
        <h1>Users</h1>
        <div className="admin-users__filters">
          <div className="admin-users__search">
            <Input
              label="Search"
              placeholder="Username or email..."
              value={searchInput}
              onChange={handleSearchChange}
            />
          </div>
          <Select
            label="Group"
            options={[{ value: "", label: "All Groups" }, ...groups]}
            value={groupFilter}
            onChange={handleGroupChange}
          />
          <Select
            label="Status"
            options={[
              { value: "", label: "All" },
              { value: "true", label: "Enabled" },
              { value: "false", label: "Disabled" },
            ]}
            value={enabledFilter}
            onChange={handleEnabledChange}
          />
        </div>
      </div>

      {loading ? (
        <p>Loading...</p>
      ) : users.length === 0 ? (
        <p className="admin-users__empty">No users found.</p>
      ) : (
        <>
          <table className="admin-users__table">
            <thead>
              <tr>
                <th>Username</th>
                <th>Email</th>
                <th>Group</th>
                <th>Uploaded</th>
                <th>Downloaded</th>
                <th>Status</th>
                <th>Created</th>
                <th>Last Active</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id}>
                  <td>{user.username}</td>
                  <td>{user.email}</td>
                  <td>{user.group_name}</td>
                  <td>{formatBytes(user.uploaded)}</td>
                  <td>{formatBytes(user.downloaded)}</td>
                  <td>
                    {!user.enabled && (
                      <span className="admin-users__badge admin-users__badge--disabled">
                        Disabled
                      </span>
                    )}
                    {user.enabled && !user.warned && (
                      <span className="admin-users__badge admin-users__badge--enabled">
                        Active
                      </span>
                    )}
                    {user.warned && (
                      <span className="admin-users__badge admin-users__badge--warned">
                        Warned
                      </span>
                    )}
                  </td>
                  <td>{timeAgo(user.created_at)}</td>
                  <td>
                    {user.last_access ? timeAgo(user.last_access) : "Never"}
                  </td>
                  <td className="admin-users__actions">
                    <button
                      className="admin-users__edit-btn"
                      onClick={() => setEditingUser(user)}
                    >
                      Edit
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          <Pagination
            currentPage={page}
            totalPages={totalPages}
            onPageChange={handlePageChange}
          />
        </>
      )}

      {editingUser && (
        <AdminUserEditModal
          user={editingUser}
          groups={groups}
          isOpen={!!editingUser}
          onClose={() => setEditingUser(null)}
          onSave={handleSaveUser}
        />
      )}
    </div>
  );
}
