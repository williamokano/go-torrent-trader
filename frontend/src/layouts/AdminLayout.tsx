import { NavLink, Outlet } from "react-router-dom";
import "./admin-layout.css";

export function AdminLayout() {
  return (
    <div className="admin-layout">
      <nav className="admin-layout__sidebar">
        <h2 className="admin-layout__sidebar-title">Admin</h2>
        <NavLink
          to="/admin/users"
          className={({ isActive }) =>
            `admin-layout__nav-link${isActive ? " admin-layout__nav-link--active" : ""}`
          }
        >
          Users
        </NavLink>
        <NavLink
          to="/admin/reports"
          className={({ isActive }) =>
            `admin-layout__nav-link${isActive ? " admin-layout__nav-link--active" : ""}`
          }
        >
          Reports
        </NavLink>
        <NavLink
          to="/admin/groups"
          className={({ isActive }) =>
            `admin-layout__nav-link${isActive ? " admin-layout__nav-link--active" : ""}`
          }
        >
          Groups
        </NavLink>
      </nav>
      <div className="admin-layout__content">
        <Outlet />
      </div>
    </div>
  );
}
