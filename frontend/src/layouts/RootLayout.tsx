import { useEffect, useRef, useState } from "react";
import { Outlet, NavLink, Link, useLocation } from "react-router-dom";
import { useTheme } from "@/themes";
import { useAuth } from "@/features/auth";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { formatNumber } from "@/utils/format";
import { Chat } from "@/components/Chat";
import { useChat } from "@/lib/useChat";
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

  const { pmUnreadCount, setPmUnreadCount, connected } = useChat();
  const prevConnectedRef = useRef(connected);

  // Fetch unread count on mount and on WS reconnection (laptop sleep, network blip).
  useEffect(() => {
    if (!isAuthenticated) return;

    const wasConnected = prevConnectedRef.current;
    prevConnectedRef.current = connected;

    // Skip when disconnecting — only fetch on reconnection (false→true) or initial mount.
    if (!connected && wasConnected) return;

    const token = getAccessToken();
    if (!token) return;
    fetch(`${getConfig().API_URL}/api/v1/messages/unread-count`, {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then((r) => r.json())
      .then((d) => setPmUnreadCount(d?.unread_count ?? 0))
      .catch(() => {});
  }, [isAuthenticated, setPmUnreadCount, connected]);

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
        <div className="header__inner">
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
            to="/news"
            end={false}
            className={({ isActive }) =>
              `header__nav-link${isActive ? " header__nav-link--active" : ""}`
            }
            onClick={closeMenu}
          >
            News
          </NavLink>

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

          <Dropdown
            key={`info-${location.key}`}
            label="Info"
            onNavigate={closeMenu}
          >
            <NavLink
              to="/faq"
              className="header__dropdown-item"
              onClick={closeMenu}
            >
              FAQ
            </NavLink>
            <NavLink
              to="/rules"
              className="header__dropdown-item"
              onClick={closeMenu}
            >
              Rules
            </NavLink>
            <NavLink
              to="/formatting"
              className="header__dropdown-item"
              onClick={closeMenu}
            >
              Formatting Guide
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
                {pmUnreadCount > 0 && (
                  <span className="header__mail-badge">{pmUnreadCount}</span>
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
        </div>
      </header>

      <main className="main">
        <Outlet />
      </main>

      {isAuthenticated && <Chat />}

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
          <Link to="/rules" className="footer__link">
            Rules
          </Link>
          <Link to="/faq" className="footer__link">
            FAQ
          </Link>
          <Link to="/formatting" className="footer__link">
            Formatting
          </Link>
          <a href="#" className="footer__link">
            Contact
          </a>
        </div>
      </footer>
    </div>
  );
}
