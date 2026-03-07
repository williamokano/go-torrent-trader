import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { CommentsSection } from "@/components/CommentsSection";
import { ToastProvider } from "@/components/toast";
import { AuthContext } from "@/features/auth/AuthContextDef";
import type { AuthContextValue, User } from "@/features/auth/AuthContextDef";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const FAKE_COMMENTS = {
  comments: [
    {
      id: 1,
      user_id: 5,
      username: "alice",
      body: "Great torrent!",
      created_at: "2026-03-05T10:00:00Z",
      updated_at: "2026-03-05T10:00:00Z",
    },
    {
      id: 2,
      user_id: 6,
      username: "bob",
      body: "Thanks for sharing.",
      created_at: "2026-03-05T11:00:00Z",
      updated_at: "2026-03-05T12:00:00Z",
    },
  ],
  total: 2,
  page: 1,
  per_page: 10,
};

function makeUser(overrides: Partial<User> = {}): User {
  return {
    id: 5,
    username: "alice",
    email: "alice@example.com",
    group_id: 1,
    avatar: "",
    title: "",
    info: "",
    uploaded: 0,
    downloaded: 0,
    ratio: 0,
    passkey: "",
    invites: 0,
    warned: false,
    donor: false,
    enabled: true,
    created_at: "",
    last_login: "",
    isAdmin: false,
    ...overrides,
  };
}

function makeAuthContext(user: User | null = makeUser()): AuthContextValue {
  return {
    user,
    isAuthenticated: !!user,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    register: vi.fn(),
    refreshUser: vi.fn(),
  };
}

function renderComments(
  torrentId = "1",
  authContext: AuthContextValue = makeAuthContext(),
) {
  return render(
    <AuthContext.Provider value={authContext}>
      <ToastProvider>
        <CommentsSection torrentId={torrentId} />
      </ToastProvider>
    </AuthContext.Provider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

beforeEach(() => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(JSON.stringify(FAKE_COMMENTS), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
});

describe("CommentsSection", () => {
  test("shows loading state initially", () => {
    vi.spyOn(globalThis, "fetch").mockReturnValue(new Promise(() => {}));
    renderComments();
    expect(screen.getByText("Loading comments...")).toBeInTheDocument();
  });

  test("renders comments after loading", async () => {
    renderComments();
    await waitFor(() => {
      expect(screen.getByText("Great torrent!")).toBeInTheDocument();
    });
    expect(screen.getByText("Thanks for sharing.")).toBeInTheDocument();
  });

  test("shows comment count in title", async () => {
    renderComments();
    await waitFor(() => {
      expect(screen.getByText("(2)")).toBeInTheDocument();
    });
  });

  test("shows author username", async () => {
    renderComments();
    await waitFor(() => {
      expect(screen.getByText("alice")).toBeInTheDocument();
    });
    expect(screen.getByText("bob")).toBeInTheDocument();
  });

  test("shows (edited) for edited comments", async () => {
    renderComments();
    await waitFor(() => {
      expect(screen.getByText("(edited)")).toBeInTheDocument();
    });
  });

  test("shows empty state when no comments", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({ comments: [], total: 0, page: 1, per_page: 10 }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    renderComments();
    await waitFor(() => {
      expect(screen.getByText("No comments yet.")).toBeInTheDocument();
    });
  });

  test("shows comment form for authenticated user", async () => {
    renderComments();
    await waitFor(() => {
      expect(screen.getByLabelText("Add a comment")).toBeInTheDocument();
    });
    expect(
      screen.getByRole("button", { name: "Post Comment" }),
    ).toBeInTheDocument();
  });

  test("hides comment form for unauthenticated user", async () => {
    renderComments("1", makeAuthContext(null));
    await waitFor(() => {
      expect(screen.getByText("Great torrent!")).toBeInTheDocument();
    });
    expect(screen.queryByLabelText("Add a comment")).not.toBeInTheDocument();
  });

  test("submit button is disabled when body is empty", async () => {
    renderComments();
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Post Comment" }),
      ).toBeDisabled();
    });
  });

  test("submits comment via POST", async () => {
    const mockFetch = vi.spyOn(globalThis, "fetch");
    // First call: fetch comments; second call: POST; third call: refetch
    mockFetch
      .mockResolvedValueOnce(
        new Response(JSON.stringify(FAKE_COMMENTS), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(new Response(null, { status: 201 }))
      .mockResolvedValueOnce(
        new Response(JSON.stringify(FAKE_COMMENTS), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );

    renderComments();

    await waitFor(() => {
      expect(screen.getByLabelText("Add a comment")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Add a comment"), {
      target: { value: "Nice upload!" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Post Comment" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/torrents/1/comments",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ body: "Nice upload!" }),
        }),
      );
    });
  });

  test("shows edit button for comment author (no delete — admin only)", async () => {
    renderComments();
    await waitFor(() => {
      expect(screen.getByText("Great torrent!")).toBeInTheDocument();
    });
    // User id=5 owns comment id=1 (user_id=5) — can edit but not delete
    const editButtons = screen.getAllByRole("button", { name: "Edit" });
    expect(editButtons.length).toBeGreaterThanOrEqual(1);
    const deleteButtons = screen.queryAllByRole("button", { name: "Delete" });
    expect(deleteButtons).toHaveLength(0);
  });

  test("shows edit and delete buttons for admin on all comments", async () => {
    renderComments("1", makeAuthContext(makeUser({ id: 999, isAdmin: true })));
    await waitFor(() => {
      expect(screen.getByText("Great torrent!")).toBeInTheDocument();
    });
    const editButtons = screen.getAllByRole("button", { name: "Edit" });
    expect(editButtons).toHaveLength(2);
    const deleteButtons = screen.getAllByRole("button", { name: "Delete" });
    expect(deleteButtons).toHaveLength(2);
  });

  test("clicking edit shows textarea with comment body", async () => {
    renderComments();
    await waitFor(() => {
      expect(screen.getByText("Great torrent!")).toBeInTheDocument();
    });

    const editButtons = screen.getAllByRole("button", { name: "Edit" });
    fireEvent.click(editButtons[0]);

    await waitFor(() => {
      expect(screen.getByLabelText("Edit comment")).toBeInTheDocument();
    });
    expect(screen.getByDisplayValue("Great torrent!")).toBeInTheDocument();
  });

  test("delete comment calls API (admin)", async () => {
    const mockFetch = vi.spyOn(globalThis, "fetch");
    mockFetch
      .mockResolvedValueOnce(
        new Response(JSON.stringify(FAKE_COMMENTS), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ comments: [], total: 0, page: 1, per_page: 10 }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );

    renderComments("1", makeAuthContext(makeUser({ id: 999, isAdmin: true })));
    await waitFor(() => {
      expect(screen.getByText("Great torrent!")).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByRole("button", { name: "Delete" });
    fireEvent.click(deleteButtons[0]);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/comments/1",
        expect.objectContaining({ method: "DELETE" }),
      );
    });
  });
});
