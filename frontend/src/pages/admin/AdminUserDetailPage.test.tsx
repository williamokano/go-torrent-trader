import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
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
  passkey: "abc123def456",
  invites: 2,
  created_at: "2024-01-01T00:00:00Z",
  last_access: "2024-06-01T12:00:00Z",
  ratio: 2.0,
  recent_uploads: [],
  warnings_count: 0,
  mod_notes: [],
};

const mockGroups = {
  groups: [
    { id: 1, name: "Administrator" },
    { id: 5, name: "User" },
  ],
};

function mockFetchResponses(
  userOverrides: Record<string, unknown> = {},
  restrictionsOverrides: unknown[] = [],
) {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/admin/users/1/restrictions")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({ restrictions: restrictionsOverrides }),
      });
    }
    if (url.includes("/admin/groups")) {
      return Promise.resolve({
        ok: true,
        json: async () => mockGroups,
      });
    }
    if (url.includes("/admin/users/1")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({ user: { ...mockUser, ...userOverrides } }),
      });
    }
    return Promise.resolve({ ok: true, json: async () => ({}) });
  });
}

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
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("testuser")).toBeInTheDocument();
    });
    expect(screen.getByText("Edit Profile")).toBeInTheDocument();
    expect(screen.getByDisplayValue("test@example.com")).toBeInTheDocument();
  });

  test("renders edit form with user data populated", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Edit Profile")).toBeInTheDocument();
    });

    // Form fields should be populated with user data
    expect(screen.getByDisplayValue("testuser")).toBeInTheDocument();
    expect(screen.getByDisplayValue("test@example.com")).toBeInTheDocument();
    expect(screen.getByDisplayValue("1073741824")).toBeInTheDocument();
    expect(screen.getByDisplayValue("536870912")).toBeInTheDocument();
    expect(screen.getByDisplayValue("2")).toBeInTheDocument();
  });

  test("renders passkey read-only display", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("abc123def456")).toBeInTheDocument();
    });
  });

  test("renders reset password and reset passkey buttons", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Reset Password")).toBeInTheDocument();
    });
    expect(screen.getByText("Reset Passkey")).toBeInTheDocument();
  });

  test("opens password reset modal when clicking Reset Password", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Reset Password")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Reset Password"));

    expect(screen.getByText("Reset Password for testuser")).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText("Leave blank to auto-generate"),
    ).toBeInTheDocument();
  });

  test("calls reset password API and shows generated password", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Reset Password")).toBeInTheDocument();
    });

    // Override fetch for the password reset call
    mockFetch.mockImplementationOnce(() =>
      Promise.resolve({
        ok: true,
        json: async () => ({ new_password: "GeneratedPass123!" }),
      }),
    );

    fireEvent.click(screen.getByText("Reset Password"));

    const resetButtons = screen.getAllByText("Reset Password");
    fireEvent.click(resetButtons[resetButtons.length - 1]);

    await waitFor(() => {
      expect(screen.getByText("GeneratedPass123!")).toBeInTheDocument();
    });
    expect(screen.getByText("Copy")).toBeInTheDocument();
  });

  test("opens passkey confirm modal when clicking Reset Passkey", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Reset Passkey")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Reset Passkey"));

    expect(
      screen.getByText(/invalidate all existing \.torrent files/),
    ).toBeInTheDocument();
  });

  test("renders save changes button", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Save Changes")).toBeInTheDocument();
    });
  });

  test("renders empty state for no uploads and no notes", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("No uploads.")).toBeInTheDocument();
    });
    expect(screen.getByText("No staff notes.")).toBeInTheDocument();
  });

  test("renders recent uploads when present", async () => {
    mockFetchResponses({
      recent_uploads: [
        {
          id: 10,
          name: "Ubuntu 24.04 LTS",
          size: 4294967296,
          created_at: "2024-05-01T00:00:00Z",
        },
      ],
    });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Ubuntu 24.04 LTS")).toBeInTheDocument();
    });
  });

  test("renders mod notes when present", async () => {
    mockFetchResponses({
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
    });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Warned for bad behavior")).toBeInTheDocument();
    });
    expect(screen.getByText("admin")).toBeInTheDocument();
  });

  test("shows warning badge in header when user is warned", async () => {
    mockFetchResponses({ warned: true });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("testuser")).toBeInTheDocument();
    });
    // WarningBadge renders in the header
    const warningBadge = document.querySelector(".warning-badge");
    expect(warningBadge).toBeInTheDocument();
  });

  test("shows enabled checkbox unchecked when user is disabled", async () => {
    mockFetchResponses({ enabled: false });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("testuser")).toBeInTheDocument();
    });
    const enabledCheckbox = screen.getByLabelText("Enabled");
    expect(enabledCheckbox).not.toBeChecked();
  });

  test("renders loading state initially", () => {
    mockFetch.mockReturnValue(new Promise(() => {}));
    renderPage();

    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  test("renders group dropdown in edit form", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Edit Profile")).toBeInTheDocument();
    });

    // Group select should show groups from API
    await waitFor(() => {
      expect(screen.getByText("Administrator")).toBeInTheDocument();
    });
  });

  test("renders flag checkboxes in edit form", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Enabled")).toBeInTheDocument();
    });

    // The Warned label appears both as a checkbox label and possibly as badge
    // Just check the form contains expected checkbox labels
    expect(screen.getByText("Donor")).toBeInTheDocument();
    expect(screen.getByText("Parked")).toBeInTheDocument();
  });

  test("shows Ban User button for enabled users", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Ban User")).toBeInTheDocument();
    });
  });

  test("hides Ban User button for disabled users", async () => {
    mockFetchResponses({ enabled: false });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("testuser")).toBeInTheDocument();
    });
    expect(screen.queryByText("Ban User")).not.toBeInTheDocument();
  });
});
