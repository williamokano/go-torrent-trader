import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { AdminUserDetailPage } from "@/pages/admin/AdminUserDetailPage";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();

vi.stubGlobal("fetch", mockFetch);

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
});

const mockUser = {
  id: 1,
  username: "testuser",
  email: "test@example.com",
  group_id: 5,
  group_name: "User",
  avatar: null,
  title: null,
  info: null,
  uploaded: 1073741824,
  downloaded: 536870912,
  enabled: true,
  can_download: true,
  can_upload: true,
  can_chat: true,
  warned: false,
  donor: false,
  parked: false,
  invites: 2,
  created_at: "2024-01-01T00:00:00Z",
  last_access: "2024-06-01T12:00:00Z",
  ratio: 2.0,
  recent_uploads: [],
  warnings_count: 0,
  mod_notes: [],
};

function renderPage() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={["/admin/users/1"]}>
        <Routes>
          <Route path="/admin/users/:id" element={<AdminUserDetailPage />} />
          <Route path="/admin/users" element={<div>Users List</div>} />
        </Routes>
      </MemoryRouter>
    </ToastProvider>,
  );
}

describe("AdminUserDetailPage", () => {
  test("renders user profile data", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({ user: mockUser }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("testuser")).toBeInTheDocument();
    });
    expect(screen.getByText("test@example.com")).toBeInTheDocument();
    expect(screen.getByText("User")).toBeInTheDocument();
    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  test("renders empty state for no uploads and no notes", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({ user: mockUser }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("No uploads.")).toBeInTheDocument();
    });
    expect(screen.getByText("No staff notes.")).toBeInTheDocument();
  });

  test("renders recent uploads when present", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        user: {
          ...mockUser,
          recent_uploads: [
            {
              id: 10,
              name: "Ubuntu 24.04 LTS",
              size: 4294967296,
              created_at: "2024-05-01T00:00:00Z",
            },
          ],
        },
      }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS")).toBeInTheDocument();
    });
  });

  test("renders mod notes when present", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        user: {
          ...mockUser,
          mod_notes: [
            {
              id: 1,
              user_id: 1,
              author_id: 99,
              author_username: "admin",
              note: "Warned for bad behavior",
              created_at: "2024-05-15T10:00:00Z",
            },
          ],
        },
      }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Warned for bad behavior")).toBeInTheDocument();
    });
    expect(screen.getByText("admin")).toBeInTheDocument();
  });

  test("shows warning badge when user is warned", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        user: { ...mockUser, warned: true },
      }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Warned")).toBeInTheDocument();
    });
  });

  test("shows disabled badge when user is disabled", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        user: { ...mockUser, enabled: false },
      }),
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Disabled")).toBeInTheDocument();
    });
  });

  test("renders loading state initially", () => {
    mockFetch.mockReturnValue(new Promise(() => {}));

    renderPage();

    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });
});
