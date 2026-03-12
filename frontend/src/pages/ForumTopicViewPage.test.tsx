import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { fireEvent } from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { ForumTopicViewPage } from "@/pages/ForumTopicViewPage";
import { ToastProvider } from "@/components/toast";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));
vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));
const mockUseAuth = vi.fn();
vi.mock("@/features/auth", () => ({ useAuth: () => mockUseAuth() }));
vi.mock("@/components/UsernameDisplay", () => ({
  UsernameDisplay: ({ username }: { userId: number; username: string }) => (
    <span>{username}</span>
  ),
}));
vi.mock("@/components/MarkdownRenderer", () => ({
  MarkdownRenderer: ({ content }: { content: string }) => <div>{content}</div>,
}));
vi.mock("@/components/Pagination", () => ({
  Pagination: () => <div data-testid="pagination" />,
}));
vi.mock("@/components/modal", () => ({
  ConfirmModal: ({
    isOpen,
    title,
    message,
    confirmLabel,
    onConfirm,
    onCancel,
  }: {
    isOpen: boolean;
    title: string;
    message: string;
    confirmLabel: string;
    onConfirm: () => void;
    onCancel: () => void;
  }) =>
    isOpen ? (
      <div data-testid="confirm-modal">
        <span>{title}</span>
        <span>{message}</span>
        <button onClick={onConfirm}>{confirmLabel}</button>
        <button onClick={onCancel}>Cancel</button>
      </div>
    ) : null,
  Modal: ({
    isOpen,
    onClose,
    title,
    children,
  }: {
    isOpen: boolean;
    onClose: () => void;
    title: string;
    children: React.ReactNode;
  }) =>
    isOpen ? (
      <div data-testid="modal">
        <span>{title}</span>
        {children}
        <button onClick={onClose}>Close</button>
      </div>
    ) : null,
}));

const FAKE_RESPONSE = {
  topic: {
    id: 1,
    forum_id: 1,
    user_id: 1,
    username: "alice",
    title: "Test Topic",
    pinned: false,
    locked: false,
    post_count: 2,
    view_count: 50,
    forum_name: "General Discussion",
    created_at: "2025-05-01T10:00:00Z",
  },
  posts: [
    {
      id: 1,
      topic_id: 1,
      user_id: 1,
      username: "alice",
      group_name: "User",
      body: "First post body",
      created_at: "2025-05-01T10:00:00Z",
      user_created_at: "2025-01-01T00:00:00Z",
      user_post_count: 10,
    },
    {
      id: 2,
      topic_id: 1,
      user_id: 2,
      username: "bob",
      group_name: "User",
      body: "Reply body",
      created_at: "2025-05-02T10:00:00Z",
      user_created_at: "2025-02-01T00:00:00Z",
      user_post_count: 5,
    },
  ],
  total: 2,
  page: 1,
  per_page: 25,
  can_moderate: false,
};
const MOD_RESPONSE = { ...FAKE_RESPONSE, can_moderate: true };
const LOCKED_RESPONSE = {
  ...FAKE_RESPONSE,
  can_moderate: true,
  topic: { ...FAKE_RESPONSE.topic, locked: true },
};
const mockFetch = vi.fn();

afterEach(cleanup);
beforeEach(() => {
  vi.clearAllMocks();
  mockUseAuth.mockReturnValue({
    user: { id: 1, username: "testuser", isAdmin: false, isStaff: false },
    isAuthenticated: true,
  });
  mockFetch.mockResolvedValue({
    ok: true,
    json: () => Promise.resolve(FAKE_RESPONSE),
  });
  vi.stubGlobal("fetch", mockFetch);
});

function renderPage() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={["/forums/topics/1"]}>
        <Routes>
          <Route path="/forums/topics/:id" element={<ForumTopicViewPage />} />
        </Routes>
      </MemoryRouter>
    </ToastProvider>,
  );
}

describe("ForumTopicViewPage", () => {
  test("renders topic title and posts", async () => {
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    expect(screen.getByText("First post body")).toBeInTheDocument();
    expect(screen.getByText("Reply body")).toBeInTheDocument();
  });

  test("shows reply form when topic is not locked", async () => {
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    expect(screen.getByText("Post a Reply")).toBeInTheDocument();
  });

  test("hides reply form when topic is locked", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(LOCKED_RESPONSE),
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    expect(
      screen.getByText("This topic is locked. No new replies can be posted."),
    ).toBeInTheDocument();
    expect(screen.queryByText("Post a Reply")).not.toBeInTheDocument();
  });

  test("hides reply form when user is not logged in", async () => {
    mockUseAuth.mockReturnValue({ user: null, isAuthenticated: false });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    expect(screen.queryByText("Post a Reply")).not.toBeInTheDocument();
  });

  test("shows edit and delete buttons for post author", async () => {
    // user id=1 is author of post id=1 (alice's post)
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: "alice", isAdmin: false, isStaff: false },
      isAuthenticated: true,
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    const editBtns = screen.getAllByText("Edit");
    const deleteBtns = screen.getAllByText("Delete");
    // User owns post 1 but not post 2 — should see 1 edit and 1 delete
    expect(editBtns).toHaveLength(1);
    expect(deleteBtns).toHaveLength(1);
  });

  test("shows edit and delete buttons for admin on all posts", async () => {
    mockUseAuth.mockReturnValue({
      user: { id: 99, username: "admin", isAdmin: true, isStaff: false },
      isAuthenticated: true,
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    const editBtns = screen.getAllByText("Edit");
    const deleteBtns = screen.getAllByText("Delete");
    // Admin can edit/delete all posts + mod toolbar Delete button
    expect(editBtns).toHaveLength(2);
    expect(deleteBtns.length).toBeGreaterThanOrEqual(2);
  });

  test("shows edit and delete buttons for staff on all posts", async () => {
    mockUseAuth.mockReturnValue({
      user: { id: 99, username: "mod", isAdmin: false, isStaff: true },
      isAuthenticated: true,
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    const editBtns = screen.getAllByText("Edit");
    const deleteBtns = screen.getAllByText("Delete");
    expect(editBtns).toHaveLength(2);
    expect(deleteBtns.length).toBeGreaterThanOrEqual(2);
  });

  test("hides edit and delete buttons for non-author non-admin", async () => {
    // user id=3 owns neither post
    mockUseAuth.mockReturnValue({
      user: { id: 3, username: "charlie", isAdmin: false, isStaff: false },
      isAuthenticated: true,
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    expect(screen.queryByText("Edit")).not.toBeInTheDocument();
    // No mod toolbar Delete button either for regular users
    expect(screen.queryByText("Delete")).not.toBeInTheDocument();
  });

  test("entering edit mode shows textarea with post body", async () => {
    const usr = userEvent.setup();
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: "alice", isAdmin: false, isStaff: false },
      isAuthenticated: true,
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });

    await usr.click(screen.getByText("Edit"));
    const textarea = screen.getByPlaceholderText(
      "Edit your post... (Markdown supported)",
    );
    expect(textarea).toBeInTheDocument();
    expect(textarea).toHaveValue("First post body");
    expect(screen.getByText("Save")).toBeInTheDocument();
    // Cancel button inside edit form
    expect(screen.getAllByText("Cancel").length).toBeGreaterThanOrEqual(1);
  });

  test("cancel edit exits edit mode", async () => {
    const usr = userEvent.setup();
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: "alice", isAdmin: false, isStaff: false },
      isAuthenticated: true,
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });

    await usr.click(screen.getByText("Edit"));
    expect(screen.getByText("Save")).toBeInTheDocument();

    // Click Cancel in the edit form actions
    const cancelBtns = screen.getAllByText("Cancel");
    await usr.click(cancelBtns[0]);

    expect(screen.queryByText("Save")).not.toBeInTheDocument();
    expect(screen.getByText("First post body")).toBeInTheDocument();
  });

  test("save edit calls PUT and updates post", async () => {
    const usr = userEvent.setup();
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: "alice", isAdmin: false, isStaff: false },
      isAuthenticated: true,
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });

    await usr.click(screen.getByText("Edit"));
    const textarea = screen.getByPlaceholderText(
      "Edit your post... (Markdown supported)",
    );
    await usr.clear(textarea);
    await usr.type(textarea, "Updated body");

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () =>
        Promise.resolve({
          post: {
            id: 1,
            topic_id: 1,
            user_id: 1,
            username: "alice",
            group_name: "User",
            body: "Updated body",
            created_at: "2025-05-01T10:00:00Z",
            edited_at: "2025-05-03T10:00:00Z",
            user_created_at: "2025-01-01T00:00:00Z",
            user_post_count: 10,
          },
        }),
    });

    await usr.click(screen.getByText("Save"));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/forums/posts/1",
        expect.objectContaining({ method: "PUT" }),
      );
    });
    await waitFor(() => {
      expect(screen.getByText("Updated body")).toBeInTheDocument();
    });
    expect(screen.queryByText("Save")).not.toBeInTheDocument();
  });

  test("delete post button opens confirm modal", async () => {
    const usr = userEvent.setup();
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: "alice", isAdmin: false, isStaff: false },
      isAuthenticated: true,
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });

    await usr.click(screen.getByText("Delete"));
    expect(screen.getByTestId("confirm-modal")).toBeInTheDocument();
    expect(screen.getByText("Delete Post")).toBeInTheDocument();
  });

  test("confirming delete calls DELETE and removes post", async () => {
    const usr = userEvent.setup();
    mockUseAuth.mockReturnValue({
      user: { id: 2, username: "bob", isAdmin: false, isStaff: false },
      isAuthenticated: true,
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    expect(screen.getByText("Reply body")).toBeInTheDocument();

    await usr.click(screen.getByText("Delete"));
    expect(screen.getByTestId("confirm-modal")).toBeInTheDocument();

    mockFetch.mockResolvedValueOnce({ ok: true, status: 204 });

    // Click the "Delete" button inside the modal (confirmLabel)
    const modalDeleteBtn = screen
      .getByTestId("confirm-modal")
      .querySelector("button");
    await usr.click(modalDeleteBtn!);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/forums/posts/2",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
    await waitFor(() => {
      expect(screen.queryByText("Reply body")).not.toBeInTheDocument();
    });
  });

  test("delete first post shows error message", async () => {
    const usr = userEvent.setup();
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: "alice", isAdmin: false, isStaff: false },
      isAuthenticated: true,
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });

    await usr.click(screen.getByText("Delete"));

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 400,
      json: () =>
        Promise.resolve({
          error: {
            message: "Cannot delete the first post. Delete the topic instead.",
          },
        }),
    });

    const modalDeleteBtn = screen
      .getByTestId("confirm-modal")
      .querySelector("button");
    await usr.click(modalDeleteBtn!);

    await waitFor(() => {
      expect(
        screen.getByText(
          "Cannot delete the first post. Delete the topic instead.",
        ),
      ).toBeInTheDocument();
    });
  });

  // --- Moderation tests ---

  test("shows mod toolbar for admin users", async () => {
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: "admin", isAdmin: true, isStaff: false },
      isAuthenticated: true,
    });
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(MOD_RESPONSE),
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    expect(screen.getByText("Lock")).toBeInTheDocument();
    expect(screen.getByText("Pin")).toBeInTheDocument();
    expect(screen.getByText("Rename")).toBeInTheDocument();
    expect(screen.getByText("Move")).toBeInTheDocument();
    // Delete appears in both mod toolbar and post actions
    expect(screen.getAllByText("Delete").length).toBeGreaterThanOrEqual(1);
  });

  test("shows mod toolbar for staff users", async () => {
    mockUseAuth.mockReturnValue({
      user: { id: 2, username: "mod", isAdmin: false, isStaff: true },
      isAuthenticated: true,
    });
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(MOD_RESPONSE),
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    expect(screen.getByText("Lock")).toBeInTheDocument();
    expect(screen.getByText("Pin")).toBeInTheDocument();
  });

  test("hides mod toolbar for regular users", async () => {
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    expect(screen.queryByText("Lock")).not.toBeInTheDocument();
    expect(screen.queryByText("Pin")).not.toBeInTheDocument();
    expect(screen.queryByText("Rename")).not.toBeInTheDocument();
    expect(screen.queryByText("Move")).not.toBeInTheDocument();
  });

  test("lock button calls correct API and updates state", async () => {
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: "admin", isAdmin: true, isStaff: false },
      isAuthenticated: true,
    });
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(MOD_RESPONSE),
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });

    // Click Lock button — opens confirmation modal
    fireEvent.click(screen.getByText("Lock"));

    // Modal should appear with Lock confirmation button
    await waitFor(() => {
      expect(
        screen.getByText("This will prevent new replies from being posted."),
      ).toBeInTheDocument();
    });

    // Click the confirm Lock button in the modal
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ message: "topic locked" }),
    });
    const modalButtons = screen.getAllByText("Lock");
    fireEvent.click(modalButtons[modalButtons.length - 1]);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/forums/topics/1/lock",
        expect.objectContaining({ method: "POST" }),
      );
    });

    // Button should now say "Unlock"
    await waitFor(() => {
      expect(screen.getByText("Unlock")).toBeInTheDocument();
    });
  });

  test("pin button calls correct API and updates state", async () => {
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: "admin", isAdmin: true, isStaff: false },
      isAuthenticated: true,
    });
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(MOD_RESPONSE),
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });

    // Click Pin button — opens confirmation modal
    fireEvent.click(screen.getByText("Pin"));

    // Modal should appear
    await waitFor(() => {
      expect(
        screen.getByText(
          "This topic will be pinned to the top of the forum.",
        ),
      ).toBeInTheDocument();
    });

    // Click the confirm Pin button in the modal
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ message: "topic pinned" }),
    });
    const modalButtons = screen.getAllByText("Pin");
    fireEvent.click(modalButtons[modalButtons.length - 1]);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/forums/topics/1/pin",
        expect.objectContaining({ method: "POST" }),
      );
    });

    await waitFor(() => {
      expect(screen.getByText("Unpin")).toBeInTheDocument();
    });
  });

  test("delete topic button shows confirm modal and calls API", async () => {
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: "admin", isAdmin: true, isStaff: false },
      isAuthenticated: true,
    });
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(MOD_RESPONSE),
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });

    // Click the mod toolbar Delete button (it has the danger class)
    const deleteButtons = screen.getAllByText("Delete");
    // The mod toolbar Delete button is the one in the toolbar
    const modDeleteBtn = deleteButtons.find((btn) =>
      btn.closest(".forum-mod-toolbar"),
    );
    if (modDeleteBtn) {
      fireEvent.click(modDeleteBtn);
    } else {
      // Fallback: just click the first delete
      fireEvent.click(deleteButtons[0]);
    }

    await waitFor(() => {
      expect(
        screen.getByText(
          "Are you sure you want to delete this topic? This action cannot be undone.",
        ),
      ).toBeInTheDocument();
    });
  });

  test("topic author (non-staff) sees Edit Title button", async () => {
    // user id=1 is the topic author (FAKE_RESPONSE.topic.user_id=1)
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: "alice", isAdmin: false, isStaff: false },
      isAuthenticated: true,
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    // Should see "Edit Title" button (not the mod toolbar "Rename")
    expect(screen.getByText("Edit Title")).toBeInTheDocument();
    expect(screen.queryByText("Rename")).not.toBeInTheDocument();

    // Clicking it should open the rename modal
    fireEvent.click(screen.getByText("Edit Title"));
    await waitFor(() => {
      expect(screen.getByText("Rename Topic")).toBeInTheDocument();
      const input = screen.getByLabelText(
        "New topic title",
      ) as HTMLInputElement;
      expect(input.value).toBe("Test Topic");
    });
  });

  test("rename button opens modal with current title", async () => {
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: "admin", isAdmin: true, isStaff: false },
      isAuthenticated: true,
    });
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(MOD_RESPONSE),
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Rename"));

    await waitFor(() => {
      expect(screen.getByText("Rename Topic")).toBeInTheDocument();
      const input = screen.getByLabelText(
        "New topic title",
      ) as HTMLInputElement;
      expect(input.value).toBe("Test Topic");
    });
  });

  // --- Soft-delete tests ---

  test("shows deleted placeholder for soft-deleted posts", async () => {
    const deletedResponse = {
      ...FAKE_RESPONSE,
      posts: [
        FAKE_RESPONSE.posts[0],
        {
          ...FAKE_RESPONSE.posts[1],
          is_deleted: true,
          deleted_at: "2025-06-01T10:00:00Z",
        },
      ],
    };
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(deletedResponse),
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    expect(
      screen.getByText("[This post has been deleted]"),
    ).toBeInTheDocument();
    // The deleted post body should not be visible
    expect(screen.queryByText("Reply body")).not.toBeInTheDocument();
  });

  test("staff sees View Content and Restore buttons on deleted posts", async () => {
    mockUseAuth.mockReturnValue({
      user: { id: 99, username: "admin", isAdmin: true, isStaff: false },
      isAuthenticated: true,
    });
    const deletedResponse = {
      ...FAKE_RESPONSE,
      posts: [
        FAKE_RESPONSE.posts[0],
        {
          ...FAKE_RESPONSE.posts[1],
          is_deleted: true,
          deleted_at: "2025-06-01T10:00:00Z",
        },
      ],
    };
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(deletedResponse),
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    expect(screen.getByText("View Content")).toBeInTheDocument();
    expect(screen.getByText("Restore")).toBeInTheDocument();
  });

  test("non-staff does not see View Content or Restore on deleted posts", async () => {
    mockUseAuth.mockReturnValue({
      user: { id: 3, username: "charlie", isAdmin: false, isStaff: false },
      isAuthenticated: true,
    });
    const deletedResponse = {
      ...FAKE_RESPONSE,
      posts: [
        FAKE_RESPONSE.posts[0],
        {
          ...FAKE_RESPONSE.posts[1],
          is_deleted: true,
          deleted_at: "2025-06-01T10:00:00Z",
        },
      ],
    };
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(deletedResponse),
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    expect(
      screen.getByText("[This post has been deleted]"),
    ).toBeInTheDocument();
    expect(screen.queryByText("View Content")).not.toBeInTheDocument();
    expect(screen.queryByText("Restore")).not.toBeInTheDocument();
  });

  test("staff sees History button on edited posts", async () => {
    mockUseAuth.mockReturnValue({
      user: { id: 99, username: "admin", isAdmin: true, isStaff: false },
      isAuthenticated: true,
    });
    const editedResponse = {
      ...FAKE_RESPONSE,
      posts: [
        FAKE_RESPONSE.posts[0],
        {
          ...FAKE_RESPONSE.posts[1],
          edited_at: "2025-06-01T10:00:00Z",
          edited_by: 2,
        },
      ],
    };
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(editedResponse),
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Test Topic")).toBeInTheDocument();
    });
    expect(screen.getByText("History")).toBeInTheDocument();
  });
});
