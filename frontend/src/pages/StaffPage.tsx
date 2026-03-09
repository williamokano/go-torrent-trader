import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { WarningBadge } from "@/components/WarningBadge";
import "./staff.css";

interface StaffMember {
  id: number;
  username: string;
  group_id: number;
  group_name: string;
  title: string | null;
  warned: boolean;
}

interface StaffGroup {
  name: string;
  members: StaffMember[];
}

export function StaffPage() {
  const [groups, setGroups] = useState<StaffGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function fetchStaff() {
      setLoading(true);
      setError(null);

      try {
        const token = getAccessToken();
        const res = await fetch(`${getConfig().API_URL}/api/v1/users/staff`, {
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        });

        if (cancelled) return;

        const body = await res.json();

        if (!res.ok) {
          setError(body?.error?.message ?? "Failed to load staff");
          setLoading(false);
          return;
        }

        const staff: StaffMember[] = body?.staff ?? [];

        // Group by role
        const groupMap = new Map<string, StaffMember[]>();
        for (const member of staff) {
          const existing = groupMap.get(member.group_name) ?? [];
          existing.push(member);
          groupMap.set(member.group_name, existing);
        }

        // Convert to array, putting Administrators first
        const sorted: StaffGroup[] = [];
        const adminKey = Array.from(groupMap.keys()).find((k) =>
          k.toLowerCase().includes("admin"),
        );
        if (adminKey) {
          sorted.push({ name: adminKey, members: groupMap.get(adminKey)! });
          groupMap.delete(adminKey);
        }
        for (const [name, members] of groupMap) {
          sorted.push({ name, members });
        }

        setGroups(sorted);
      } catch {
        if (!cancelled) {
          setError("Failed to load staff");
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    fetchStaff();
    return () => {
      cancelled = true;
    };
  }, []);

  if (loading) {
    return (
      <div className="staff">
        <h1 className="staff__title">Staff</h1>
        <div className="staff__loading">Loading staff...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="staff">
        <h1 className="staff__title">Staff</h1>
        <div className="staff__error">{error}</div>
      </div>
    );
  }

  return (
    <div className="staff">
      <h1 className="staff__title">Staff</h1>

      {groups.length === 0 ? (
        <div className="staff__empty">No staff members found.</div>
      ) : (
        groups.map((group) => (
          <section key={group.name} className="staff__group">
            <h2 className="staff__group-name">{group.name}s</h2>
            <div className="staff__cards">
              {group.members.map((member) => (
                <div key={member.id} className="staff__card">
                  <Link
                    to={`/user/${member.id}`}
                    className="staff__card-username"
                  >
                    {member.username}
                  </Link>
                  <WarningBadge warned={member.warned} />
                  {member.title && (
                    <span className="staff__card-title">{member.title}</span>
                  )}
                </div>
              ))}
            </div>
          </section>
        ))
      )}
    </div>
  );
}
