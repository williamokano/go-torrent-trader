import { useEffect, useState } from "react";
import { getAccessToken } from "@/features/auth/token";

interface Group {
  id: number;
  name: string;
  slug: string;
  level: number;
  color: string | null;
  can_upload: boolean;
  can_download: boolean;
  can_invite: boolean;
  can_comment: boolean;
  can_forum: boolean;
  is_admin: boolean;
  is_moderator: boolean;
  is_immune: boolean;
}

const CAPABILITY_COLUMNS = [
  { key: "can_upload", label: "Upload" },
  { key: "can_download", label: "Download" },
  { key: "can_invite", label: "Invite" },
  { key: "can_comment", label: "Comment" },
  { key: "can_forum", label: "Forum" },
  { key: "is_admin", label: "Admin" },
  { key: "is_moderator", label: "Moderator" },
  { key: "is_immune", label: "Immune" },
] as const;

export function AdminGroupsPage() {
  const [groups, setGroups] = useState<Group[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchGroups() {
      const token = getAccessToken();
      try {
        const res = await fetch("/api/v1/admin/groups", {
          headers: { Authorization: `Bearer ${token}` },
        });
        if (res.ok) {
          const data = await res.json();
          setGroups(data.groups ?? []);
        }
      } finally {
        setLoading(false);
      }
    }
    fetchGroups();
  }, []);

  if (loading) return <p>Loading...</p>;

  return (
    <div>
      <h1
        style={{ fontSize: "var(--text-xl)", marginBottom: "var(--space-lg)" }}
      >
        Groups
      </h1>
      <table
        style={{
          width: "100%",
          borderCollapse: "collapse",
          fontSize: "var(--text-sm)",
        }}
      >
        <thead>
          <tr>
            <th
              style={{
                textAlign: "left",
                padding: "var(--space-xs) var(--space-sm)",
                borderBottom: "1px solid var(--color-border)",
                color: "var(--color-text-muted)",
              }}
            >
              Name
            </th>
            <th
              style={{
                textAlign: "left",
                padding: "var(--space-xs) var(--space-sm)",
                borderBottom: "1px solid var(--color-border)",
                color: "var(--color-text-muted)",
              }}
            >
              Level
            </th>
            <th
              style={{
                textAlign: "left",
                padding: "var(--space-xs) var(--space-sm)",
                borderBottom: "1px solid var(--color-border)",
                color: "var(--color-text-muted)",
              }}
            >
              Color
            </th>
            {CAPABILITY_COLUMNS.map((col) => (
              <th
                key={col.key}
                style={{
                  textAlign: "center",
                  padding: "var(--space-xs) var(--space-sm)",
                  borderBottom: "1px solid var(--color-border)",
                  color: "var(--color-text-muted)",
                  whiteSpace: "nowrap",
                }}
              >
                {col.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {groups.map((group) => (
            <tr key={group.id}>
              <td
                style={{
                  padding: "var(--space-xs) var(--space-sm)",
                  borderBottom: "1px solid var(--color-border)",
                  fontWeight: 600,
                }}
              >
                {group.name}
              </td>
              <td
                style={{
                  padding: "var(--space-xs) var(--space-sm)",
                  borderBottom: "1px solid var(--color-border)",
                }}
              >
                {group.level}
              </td>
              <td
                style={{
                  padding: "var(--space-xs) var(--space-sm)",
                  borderBottom: "1px solid var(--color-border)",
                }}
              >
                {group.color ? (
                  <span
                    style={{
                      display: "inline-block",
                      width: 16,
                      height: 16,
                      borderRadius: 4,
                      backgroundColor: group.color,
                      border: "1px solid var(--color-border)",
                      verticalAlign: "middle",
                    }}
                    title={group.color}
                  />
                ) : (
                  "-"
                )}
              </td>
              {CAPABILITY_COLUMNS.map((col) => (
                <td
                  key={col.key}
                  style={{
                    textAlign: "center",
                    padding: "var(--space-xs) var(--space-sm)",
                    borderBottom: "1px solid var(--color-border)",
                  }}
                >
                  {group[col.key] ? "Y" : "N"}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
