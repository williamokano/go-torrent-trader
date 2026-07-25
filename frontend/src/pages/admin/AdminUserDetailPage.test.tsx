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
  can_invite: true,
  can_feed: true,
  warned: false,
  donor: false,
  parked: false,
  passkey: "abc123def456",
  invites: 2,
  bonus_points: 1500,
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
  invitesOverrides: unknown[] = [],
  historyOverrides: unknown[] = [],
) {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/admin/users/1/edit-history")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          entries: historyOverrides,
          total: historyOverrides.length,
        }),
      });
    }
    if (url.includes("/admin/users/1/restrictions")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({ restrictions: restrictionsOverrides }),
      });
    }
    if (url.includes("/admin/users/1/invites")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({ invites: invitesOverrides }),
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
    expect(screen.getByText("Edit profile")).toBeInTheDocument();
    expect(screen.getByDisplayValue("test@example.com")).toBeInTheDocument();
  });

  test("renders edit form with user data populated", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Edit profile")).toBeInTheDocument();
    });

    // Form fields should be populated with user data. Uploaded/downloaded
    // show in the best-fit unit: 1073741824 B = 1 GB, 536870912 B = 512 MB.
    expect(screen.getByDisplayValue("testuser")).toBeInTheDocument();
    expect(screen.getByDisplayValue("test@example.com")).toBeInTheDocument();
    expect(screen.getByLabelText("Uploaded")).toHaveValue(1);
    expect(screen.getByLabelText("Uploaded unit")).toHaveValue("3");
    expect(screen.getByLabelText("Downloaded")).toHaveValue(512);
    expect(screen.getByLabelText("Downloaded unit")).toHaveValue("2");
    expect(screen.getByDisplayValue("2")).toBeInTheDocument();
    expect(screen.getByLabelText("Bonus Points")).toHaveValue(1500);
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
      expect(screen.getByText("Reset password")).toBeInTheDocument();
    });
    expect(screen.getByText("Reset passkey")).toBeInTheDocument();
  });

  test("opens password reset modal when clicking Reset Password", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Reset password")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Reset password"));

    expect(screen.getByText("Reset Password for testuser")).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText("Leave blank to auto-generate"),
    ).toBeInTheDocument();
  });

  test("calls reset password API and shows generated password", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Reset password")).toBeInTheDocument();
    });

    // Override fetch for the password reset call
    mockFetch.mockImplementationOnce(() =>
      Promise.resolve({
        ok: true,
        json: async () => ({ new_password: "GeneratedPass123!" }),
      }),
    );

    fireEvent.click(screen.getByText("Reset password"));

    const resetButtons = screen.getAllByText("Reset password");
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
      expect(screen.getByText("Reset passkey")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Reset passkey"));

    expect(
      screen.getByText(/invalidate all existing \.torrent files/),
    ).toBeInTheDocument();
  });

  test("renders save changes button", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Save changes")).toBeInTheDocument();
    });
  });

  test("renders empty state for no uploads and no notes", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("No uploads yet.")).toBeInTheDocument();
    });
    expect(screen.getByText("No staff notes yet.")).toBeInTheDocument();
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

    expect(screen.getByText("Loading…")).toBeInTheDocument();
  });

  test("renders group dropdown in edit form", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Edit profile")).toBeInTheDocument();
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

  test("renders every privilege as allowed", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Invite")).toBeInTheDocument();
    });
    expect(screen.getByText("Download")).toBeInTheDocument();
    expect(screen.getByText("Upload")).toBeInTheDocument();
    expect(screen.getByText("Chat")).toBeInTheDocument();
    expect(screen.getByText("Live feeds")).toBeInTheDocument();
    expect(screen.getAllByText("Allowed")).toHaveLength(5);
    expect(screen.queryByText("Suspended")).not.toBeInTheDocument();
  });

  test("shows a suspended live feed privilege with a restore button", async () => {
    mockFetchResponses({ can_feed: false });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Suspended")).toBeInTheDocument();
    });
    expect(screen.getAllByText("Allowed")).toHaveLength(4);
    expect(screen.getByText("Restore")).toBeInTheDocument();
  });

  test("suspending live feeds submits the feed privilege", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByLabelText("Suspend live feeds")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByLabelText("Suspend live feeds"));
    fireEvent.change(screen.getByLabelText("Reason"), {
      target: { value: "watching from a shared account" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Apply restrictions" }));

    await waitFor(() => {
      const putCall = mockFetch.mock.calls.find(
        ([url, opts]) =>
          typeof url === "string" &&
          url.includes("/restrictions") &&
          opts?.method === "PUT",
      );
      expect(putCall).toBeTruthy();
      expect(JSON.parse(putCall![1].body).can_feed).toBe(false);
    });
  });

  test("shows suspended invite privilege with restore button", async () => {
    mockFetchResponses({ can_invite: false });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Suspended")).toBeInTheDocument();
    });
    expect(screen.getAllByText("Allowed")).toHaveLength(4);
    expect(screen.getByText("Restore")).toBeInTheDocument();
  });

  test("renders suspend invite checkbox in restriction form", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByLabelText("Suspend invite")).toBeInTheDocument();
    });
    expect(screen.getByLabelText("Suspend download")).toBeInTheDocument();
    expect(screen.getByLabelText("Suspend upload")).toBeInTheDocument();
    expect(screen.getByLabelText("Suspend chat")).toBeInTheDocument();
  });

  test("sends can_invite=false when applying an invite restriction", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByLabelText("Suspend invite")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText("Suspend invite"));
    fireEvent.change(
      screen.getByPlaceholderText("Reason for suspending these privileges…"),
      { target: { value: "invite abuse" } },
    );
    fireEvent.click(screen.getByText("Apply restrictions"));

    await waitFor(() => {
      const putCall = mockFetch.mock.calls.find(
        ([url, opts]) =>
          typeof url === "string" &&
          url.includes("/restrictions") &&
          opts?.method === "PUT",
      );
      expect(putCall).toBeTruthy();
      const body = JSON.parse(putCall![1].body);
      expect(body.can_invite).toBe(false);
      expect(body.reason).toBe("invite abuse");
    });
  });

  test("renders restriction history table", async () => {
    mockFetchResponses({}, [
      {
        id: 1,
        user_id: 1,
        restriction_type: "invite",
        reason: "invite abuse",
        issued_by: 99,
        issued_by_username: "admin",
        expires_at: null,
        lifted_at: null,
        lifted_by: null,
        lifted_by_username: "",
        created_at: "2024-05-01T00:00:00Z",
      },
    ]);
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("invite abuse")).toBeInTheDocument();
    });
    expect(screen.getByText("Lift")).toBeInTheDocument();
    expect(screen.getByText("Permanent")).toBeInTheDocument();
  });

  test("shows empty state when the user has no invites", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(
        screen.getByText("No invites created by this user."),
      ).toBeInTheDocument();
    });
  });

  test("renders outstanding invites with status badges", async () => {
    mockFetchResponses(
      {},
      [],
      [
        {
          id: 1,
          token: "PENDINGTOKEN",
          status: "pending",
          expires_at: "2030-01-01T00:00:00Z",
          created_at: "2024-05-01T00:00:00Z",
        },
        {
          id: 2,
          token: "REDEEMEDTOKEN",
          status: "redeemed",
          expires_at: "2030-01-01T00:00:00Z",
          created_at: "2024-05-01T00:00:00Z",
          invitee_name: "newbie",
        },
      ],
    );
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("PENDINGTOKEN")).toBeInTheDocument();
    });
    expect(screen.getByText("REDEEMEDTOKEN")).toBeInTheDocument();
    expect(screen.getByText("Pending")).toBeInTheDocument();
    expect(screen.getByText("Redeemed")).toBeInTheDocument();
    expect(screen.getByText("newbie")).toBeInTheDocument();

    // Only the pending invite can be revoked.
    expect(screen.getAllByText("Revoke")).toHaveLength(1);
  });

  test("shows voided status and still allows revoking it", async () => {
    mockFetchResponses(
      {},
      [],
      [
        {
          id: 3,
          token: "VOIDEDTOKEN",
          status: "voided",
          expires_at: "2030-01-01T00:00:00Z",
          created_at: "2024-05-01T00:00:00Z",
        },
      ],
    );
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Voided")).toBeInTheDocument();
    });
    // A voided (unredeemed) invite can still be revoked for cleanup.
    expect(screen.getByText("Revoke")).toBeInTheDocument();
  });

  test("revokes a pending invite", async () => {
    mockFetchResponses(
      {},
      [],
      [
        {
          id: 1,
          token: "PENDINGTOKEN",
          status: "pending",
          expires_at: "2030-01-01T00:00:00Z",
          created_at: "2024-05-01T00:00:00Z",
        },
      ],
    );
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Revoke")).toBeInTheDocument();
    });

    mockFetch.mockImplementationOnce(() =>
      Promise.resolve({
        ok: true,
        json: async () => ({ message: "invite revoked" }),
      }),
    );

    fireEvent.click(screen.getByText("Revoke"));

    await waitFor(() => {
      expect(
        screen.getByText(/permanently deletes the invite/),
      ).toBeInTheDocument();
    });

    const confirmButtons = screen.getAllByText("Revoke");
    fireEvent.click(confirmButtons[confirmButtons.length - 1]);

    await waitFor(() => {
      const deleteCall = mockFetch.mock.calls.find(
        ([url, opts]) =>
          typeof url === "string" &&
          url.includes("/admin/invites/1") &&
          opts?.method === "DELETE",
      );
      expect(deleteCall).toBeTruthy();
    });
  });

  test("sends the byte value converted from the chosen unit on save", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByLabelText("Uploaded")).toBeInTheDocument();
    });

    // Switch uploaded to TB and type 2 → 2 TiB in bytes.
    fireEvent.change(screen.getByLabelText("Uploaded unit"), {
      target: { value: "4" },
    });
    fireEvent.change(screen.getByLabelText("Uploaded"), {
      target: { value: "2" },
    });
    fireEvent.click(screen.getByText("Save changes"));

    await waitFor(() => {
      const putCall = mockFetch.mock.calls.find(
        ([url, opts]) =>
          typeof url === "string" &&
          url.endsWith("/admin/users/1") &&
          opts?.method === "PUT",
      );
      expect(putCall).toBeTruthy();
      const body = JSON.parse(putCall![1].body);
      expect(body.uploaded).toBe(2 * 1024 ** 4);
      // Untouched fields are not sent at all: counters like downloaded accrue
      // concurrently, so re-sending an absolute value would revert accrual.
      expect(body).not.toHaveProperty("downloaded");
      expect(body).not.toHaveProperty("username");
    });
  });

  test("does not send a request when nothing changed", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Save changes")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Save changes"));

    await waitFor(() => {
      expect(screen.getByText("No changes to save")).toBeInTheDocument();
    });
    const putCall = mockFetch.mock.calls.find(
      ([url, opts]) =>
        typeof url === "string" &&
        url.endsWith("/admin/users/1") &&
        opts?.method === "PUT",
    );
    expect(putCall).toBeFalsy();
  });

  test("renders empty edit history state", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("No edits recorded yet.")).toBeInTheDocument();
    });
  });

  test("renders edit history entries with humanized byte values", async () => {
    mockFetchResponses(
      {},
      [],
      [],
      [
        {
          id: 2,
          user_id: 1,
          changed_by: 99,
          changed_by_username: "admin",
          field: "uploaded",
          old_value: "0",
          new_value: "1099511627776",
          created_at: "2024-06-01T00:00:00Z",
        },
        {
          id: 1,
          user_id: 1,
          changed_by: 99,
          changed_by_username: "admin",
          field: "invites",
          old_value: "2",
          new_value: "5",
          created_at: "2024-05-01T00:00:00Z",
        },
      ],
    );
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Edit history")).toBeInTheDocument();
    });
    // 1099511627776 bytes renders as 1.00 TB; the raw value stays as a title.
    expect(screen.getByText("1.00 TB")).toBeInTheDocument();
    expect(screen.getByTitle("1099511627776")).toBeInTheDocument();
    // "Invites" also appears as the form field label, hence getAllByText.
    expect(screen.getAllByText("Invites").length).toBeGreaterThan(1);
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getAllByText("admin")).not.toHaveLength(0);
  });

  test("loads older edits without duplicating rows", async () => {
    const makeEntry = (id: number) => ({
      id,
      user_id: 1,
      changed_by: 99,
      changed_by_username: "admin",
      field: "invites",
      old_value: String(id),
      new_value: String(id + 1),
      created_at: "2024-05-01T00:00:00Z",
    });
    // First page: entries 100..51 (newest first), total 60.
    const firstPage = Array.from({ length: 50 }, (_, i) => makeEntry(100 - i));
    // Second page overlaps (a fresh edit shifted the offsets): 51..41.
    const secondPage = Array.from({ length: 11 }, (_, i) => makeEntry(51 - i));

    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/edit-history")) {
        const offset = url.includes("offset=0") ? 0 : 50;
        return Promise.resolve({
          ok: true,
          json: async () => ({
            entries: offset === 0 ? firstPage : secondPage,
            total: 60,
          }),
        });
      }
      if (url.includes("/restrictions") || url.includes("/invites")) {
        return Promise.resolve({
          ok: true,
          json: async () => ({ restrictions: [], invites: [] }),
        });
      }
      if (url.includes("/admin/groups")) {
        return Promise.resolve({ ok: true, json: async () => mockGroups });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({ user: mockUser }),
      });
    });
    renderPage();

    await waitFor(() => {
      expect(
        screen.getByText(/Show older edits \(10 more\)/),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText(/Show older edits/));

    await waitFor(() => {
      // 50 unique + 10 new unique (entry 51 deduped) = 60 rows, button gone.
      expect(screen.queryByText(/Show older edits/)).not.toBeInTheDocument();
    });
    const rows = document.querySelectorAll(".admin-user-detail__history-arrow");
    expect(rows).toHaveLength(60);
  });

  test("shows a retry state when the edit history fails to load", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/edit-history")) {
        return Promise.resolve({ ok: false, json: async () => ({}) });
      }
      if (url.includes("/restrictions") || url.includes("/invites")) {
        return Promise.resolve({
          ok: true,
          json: async () => ({ restrictions: [], invites: [] }),
        });
      }
      if (url.includes("/admin/groups")) {
        return Promise.resolve({ ok: true, json: async () => mockGroups });
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({ user: mockUser }),
      });
    });
    renderPage();

    await waitFor(() => {
      expect(
        screen.getByText(/Couldn't load the edit history/),
      ).toBeInTheDocument();
    });
    expect(screen.getByText("Retry")).toBeInTheDocument();
    expect(
      screen.queryByText("No edits recorded yet."),
    ).not.toBeInTheDocument();
  });

  test("shows Ban user button for enabled users", async () => {
    mockFetchResponses();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Ban user")).toBeInTheDocument();
    });
  });

  test("hides Ban User button for disabled users", async () => {
    mockFetchResponses({ enabled: false });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("testuser")).toBeInTheDocument();
    });
    expect(screen.queryByText("Ban user")).not.toBeInTheDocument();
  });
});
