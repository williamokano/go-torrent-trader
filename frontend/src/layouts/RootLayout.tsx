import { useEffect, useState } from "react";
import { Outlet, NavLink, Link, useLocation } from "react-router-dom";
import { useTheme } from "@/themes";
import { useAuth } from "@/features/auth";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { formatNumber } from "@/utils/format";
import "./RootLayout.css";

function Dropdown({
  label,
  children,
  onNavigate,
}: {
  label: string;
  children: React.ReactNode;
  onNavigate: () => void;
}) {
  const [open, setOpen] = useState(false);

  return (
    <div className="header__dropdown">
      <button
        className="header__dropdown-toggle"
        onClick={() => setOpen((prev) => !prev)}
      >
        {label}{" "}
        <span className="header__dropdown-arrow">
          {open ? "\u25B4" : "\u25BE"}
        </span>
      </button>
      {open && (
        <>
          <div
            className="header__dropdown-backdrop"
            onClick={() => setOpen(false)}
          />
          <div className="header__dropdown-menu" onClick={onNavigate}>
            {children}
          </div>
        </>
      )}
    </div>
  );
}

export function RootLayout() {
  const { theme, toggleTheme } = useTheme();
  const { user, isAuthenticated, logout } = useAuth();
  const [menuOpen, setMenuOpen] = useState(false);
  const location = useLocation();
  const closeMenu = () => setMenuOpen(false);

  const [unreadCount, setUnreadCount] = useState(0);

  useEffect(() => {
    if (!isAuthenticated) return;
    function fetchUnread() {
      const token = getAccessToken();
      if (!token) return;
      fetch(`${getConfig().API_URL}/api/v1/messages/unread-count`, {
        headers: { Authorization: `Bearer ${token}` },
      })
        .then((r) => r.json())
        .then((d) => setUnreadCount(d?.unread_count ?? 0))
        .catch(() => {});
    }
    fetchUnread();
    const interval = setInterval(fetchUnread, 30_000);
    return () => clearInterval(interval);
  }, [isAuthenticated]);

  const [siteStats, setSiteStats] = useState<{
    users: number;
    torrents: number;
    peers: number;
    seeders: number;
    leechers: number;
  } | null>(null);

  useEffect(() => {
    function fetchStats() {
      fetch(`${getConfig().API_URL}/api/v1/stats`)
        .then((r) => r.json())
        .then((d) => setSiteStats(d?.stats ?? null))
        .catch(() => {});
    }
    fetchStats();
    const interval = setInterval(fetchStats, 60_000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="root-layout">
      <header className="header">
        <NavLink to="/" className="header__brand">
          TorrentTrader
        </NavLink>

        <button
          className="header__hamburger"
          onClick={() => setMenuOpen((prev) => !prev)}
          aria-label="Toggle menu"
        >
          {menuOpen ? "\u2715" : "\u2630"}
        </button>

        <nav
          className={`header__nav${menuOpen ? " header__nav--open" : ""}`}
          role="navigation"
        >
          <NavLink
            to="/"
            end
            className={({ isActive }) =>
              `header__nav-link${isActive ? " header__nav-link--active" : ""}`
            }
            onClick={closeMenu}
          >
            Home
          </NavLink>

          <Dropdown
            key={`torrents-${location.key}`}
            label="Torrents"
            onNavigate={closeMenu}
          >
            <NavLink
              to="/browse"
              className="header__dropdown-item"
              onClick={closeMenu}
            >
              Browse
            </NavLink>
            <NavLink
              to="/upload"
              className="header__dropdown-item"
              onClick={closeMenu}
            >
              Upload
            </NavLink>
            <NavLink
              to="/today"
              className="header__dropdown-item"
              onClick={closeMenu}
            >
              Today
            </NavLink>
            <NavLink
              to="/needseed"
              className="header__dropdown-item"
              onClick={closeMenu}
            >
              Need Seed
            </NavLink>
            <NavLink
              to="/rss"
              className="header__dropdown-item"
              onClick={closeMenu}
            >
              RSS
            </NavLink>
          </Dropdown>

          <NavLink
            to="/forums"
            className={({ isActive }) =>
              `header__nav-link${isActive ? " header__nav-link--active" : ""}`
            }
            onClick={closeMenu}
          >
            Forums
          </NavLink>

          <Dropdown
            key={`community-${location.key}`}
            label="Community"
            onNavigate={closeMenu}
          >
            <NavLink
              to="/messages"
              className="header__dropdown-item"
              onClick={closeMenu}
            >
              Messages
            </NavLink>
            <NavLink
              to="/members"
              className="header__dropdown-item"
              onClick={closeMenu}
            >
              Members
            </NavLink>
            <NavLink
              to="/staff"
              className="header__dropdown-item"
              onClick={closeMenu}
            >
              Staff
            </NavLink>
            <NavLink
              to="/invites"
              className="header__dropdown-item"
              onClick={closeMenu}
            >
              Invites
            </NavLink>
          </Dropdown>

          <NavLink
            to="/log"
            className={({ isActive }) =>
              `header__nav-link${isActive ? " header__nav-link--active" : ""}`
            }
            onClick={closeMenu}
          >
            Log
          </NavLink>
        </nav>

        <div className="header__actions">
          <button className="header__theme-btn" onClick={toggleTheme}>
            {theme === "dark" ? "Light" : "Dark"}
          </button>
          {isAuthenticated ? (
            <>
              <Link
                to="/messages"
                className="header__mail-link"
                title="Messages"
              >
                <span className="header__mail-icon">&#9993;</span>
                {unreadCount > 0 && (
                  <span className="header__mail-badge">{unreadCount}</span>
                )}
              </Link>
              <Link to={`/user/${user?.id}`} className="header__username-link">
                {user?.username}
              </Link>
              {user?.isAdmin && (
                <NavLink
                  to="/admin"
                  className={({ isActive }) =>
                    `header__nav-link${isActive ? " header__nav-link--active" : ""}`
                  }
                >
                  Admin
                </NavLink>
              )}
              <NavLink
                to="/settings"
                className={({ isActive }) =>
                  `header__nav-link${isActive ? " header__nav-link--active" : ""}`
                }
              >
                Settings
              </NavLink>
              <button className="header__theme-btn" onClick={logout}>
                Logout
              </button>
            </>
          ) : (
            <>
              <NavLink to="/login" className="header__nav-link">
                Login
              </NavLink>
              <NavLink to="/signup" className="header__nav-link">
                Sign Up
              </NavLink>
            </>
          )}
        </div>
      </header>

      <main className="main">
        <Outlet />
      </main>

      <footer className="footer">
        <p className="footer__stats">
          Users: {siteStats ? formatNumber(siteStats.users) : "--"} | Torrents:{" "}
          {siteStats ? formatNumber(siteStats.torrents) : "--"} | Peers:{" "}
          {siteStats ? formatNumber(siteStats.peers) : "--"} | Seeders:{" "}
          {siteStats ? formatNumber(siteStats.seeders) : "--"} | Leechers:{" "}
          {siteStats ? formatNumber(siteStats.leechers) : "--"}
        </p>
        <div className="footer__links">
          <a href="#" className="footer__link">
            About
          </a>
          <a href="#" className="footer__link">
            Rules
          </a>
          <a href="#" className="footer__link">
            FAQ
          </a>
          <a href="#" className="footer__link">
            Contact
          </a>
        </div>
      </footer>
    </div>
  );
}
