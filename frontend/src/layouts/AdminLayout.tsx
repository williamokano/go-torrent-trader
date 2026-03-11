import { NavLink, Outlet } from "react-router-dom";
import { useAuth } from "@/features/auth";
import "./admin-layout.css";

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  `admin-layout__nav-link${isActive ? " admin-layout__nav-link--active" : ""}`;

export function AdminLayout() {
  const { user } = useAuth();
  const isAdmin = user?.isAdmin ?? false;

  return (
    <div className="admin-layout">
      <nav className="admin-layout__sidebar">
        <h2 className="admin-layout__sidebar-title">Admin</h2>
        {isAdmin && (
          <NavLink to="/admin" end className={navLinkClass}>
            Dashboard
          </NavLink>
        )}
        {isAdmin && (
          <NavLink to="/admin/users" className={navLinkClass}>
            Users
          </NavLink>
        )}
        {isAdmin && (
          <NavLink to="/admin/reports" className={navLinkClass}>
            Reports
          </NavLink>
        )}
        {isAdmin && (
          <NavLink to="/admin/torrents" className={navLinkClass}>
            Torrents
          </NavLink>
        )}
        {isAdmin && (
          <NavLink to="/admin/categories" className={navLinkClass}>
            Categories
          </NavLink>
        )}
        {isAdmin && (
          <NavLink to="/admin/groups" className={navLinkClass}>
            Groups
          </NavLink>
        )}
        {isAdmin && (
          <NavLink to="/admin/warnings" className={navLinkClass}>
            Warnings
          </NavLink>
        )}
        <NavLink to="/admin/chat-mutes" className={navLinkClass}>
          Chat Mutes
        </NavLink>
        {isAdmin && (
          <NavLink to="/admin/news" className={navLinkClass}>
            News
          </NavLink>
        )}
        {isAdmin && (
          <NavLink to="/admin/settings" className={navLinkClass}>
            Settings
          </NavLink>
        )}
        {isAdmin && (
          <NavLink to="/admin/bans" className={navLinkClass}>
            Bans
          </NavLink>
        )}
        {isAdmin && (
          <NavLink to="/admin/forums" className={navLinkClass}>
            Forums
          </NavLink>
        )}
      </nav>
      <div className="admin-layout__content">
        <Outlet />
      </div>
    </div>
  );
}
