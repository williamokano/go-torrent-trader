import { useState } from "react";
import { Outlet, NavLink, Link } from "react-router-dom";
import { useTheme } from "@/themes";
import { useAuth } from "@/features/auth";
import "./RootLayout.css";

export function RootLayout() {
  const { theme, toggleTheme } = useTheme();
  const { user, isAuthenticated, logout } = useAuth();
  const [menuOpen, setMenuOpen] = useState(false);

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
            onClick={() => setMenuOpen(false)}
          >
            Home
          </NavLink>
          <NavLink
            to="/browse"
            className={({ isActive }) =>
              `header__nav-link${isActive ? " header__nav-link--active" : ""}`
            }
            onClick={() => setMenuOpen(false)}
          >
            Browse
          </NavLink>
          <NavLink
            to="/forums"
            className={({ isActive }) =>
              `header__nav-link${isActive ? " header__nav-link--active" : ""}`
            }
            onClick={() => setMenuOpen(false)}
          >
            Forums
          </NavLink>
          <NavLink
            to="/upload"
            className={({ isActive }) =>
              `header__nav-link${isActive ? " header__nav-link--active" : ""}`
            }
            onClick={() => setMenuOpen(false)}
          >
            Upload
          </NavLink>
          <NavLink
            to="/members"
            className={({ isActive }) =>
              `header__nav-link${isActive ? " header__nav-link--active" : ""}`
            }
            onClick={() => setMenuOpen(false)}
          >
            Members
          </NavLink>
          <NavLink
            to="/staff"
            className={({ isActive }) =>
              `header__nav-link${isActive ? " header__nav-link--active" : ""}`
            }
            onClick={() => setMenuOpen(false)}
          >
            Staff
          </NavLink>
          <NavLink
            to="/rss"
            className={({ isActive }) =>
              `header__nav-link${isActive ? " header__nav-link--active" : ""}`
            }
            onClick={() => setMenuOpen(false)}
          >
            RSS
          </NavLink>
          <NavLink
            to="/log"
            className={({ isActive }) =>
              `header__nav-link${isActive ? " header__nav-link--active" : ""}`
            }
            onClick={() => setMenuOpen(false)}
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
          Torrents: -- | Peers: -- | Seeders: -- | Leechers: --
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
