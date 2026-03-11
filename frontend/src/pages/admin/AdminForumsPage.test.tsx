import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AdminForumsPage } from "@/pages/admin/AdminForumsPage";
import { ToastProvider } from "@/components/toast";

vi.mock("@/features/auth/token", async () => {
  const actual = await vi.importActual<typeof import("@/features/auth/token")>(
    "@/features/auth/token",
  );
  return { ...actual, getAccessToken: () => "fake-admin-token" };
});

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const FAKE_CATEGORIES = [
  { id: 1, name: "General", sort_order: 1, created_at: "2024-01-01T00:00:00Z" },
  { id: 2, name: "Support", sort_order: 2, created_at: "2024-01-01T00:00:00Z" },
];

const FAKE_FORUMS = [
  {
    id: 1,
    category_id: 1,
    name: "Announcements",
    description: "Site news",
    sort_order: 1,
    topic_count: 5,
    post_count: 20,
    min_group_level: 0,
    min_post_level: 5,
    created_at: "2024-01-01T00:00:00Z",
  },
  {
    id: 2,
    category_id: 2,
    name: "Help",
    description: "Get help here",
    sort_order: 1,
    topic_count: 10,
    post_count: 50,
    min_group_level: 0,
    min_post_level: 0,
    created_at: "2024-01-01T00:00:00Z",
  },
];

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

function mockFetchBoth(categories = FAKE_CATEGORIES, forums = FAKE_FORUMS) {
  const fetchSpy = vi.spyOn(globalThis, "fetch");
  fetchSpy.mockImplementation(async (input) => {
    const url =
      typeof input === "string"
        ? input
        : ((input as Request).url ?? input.toString());
    if (url.includes("/admin/forum-categories")) {
      return { ok: true, json: async () => ({ categories }) } as Response;
    }
    if (url.includes("/admin/forums")) {
      return { ok: true, json: async () => ({ forums }) } as Response;
    }
    return { ok: false, json: async () => ({}) } as Response;
  });
  return fetchSpy;
}

function renderPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <AdminForumsPage />
      </ToastProvider>
    </MemoryRouter>,
  );
}

describe("AdminForumsPage", () => {
  test("renders categories and forums tables", async () => {
    mockFetchBoth();

    renderPage();

    await waitFor(() => {
      expect(screen.getAllByText("General").length).toBeGreaterThanOrEqual(1);
    });

    expect(screen.getAllByText("Support").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Announcements")).toBeInTheDocument();
    expect(screen.getByText("Help")).toBeInTheDocument();
  });

  test("shows loading state", () => {
    vi.spyOn(globalThis, "fetch").mockReturnValue(new Promise(() => {}));

    renderPage();

    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  test("shows empty state when no data", async () => {
    mockFetchBoth([], []);

    renderPage();

    await waitFor(() => {
      expect(
        screen.getByText("No forum categories found."),
      ).toBeInTheDocument();
    });

    expect(screen.getByText("No forums found.")).toBeInTheDocument();
  });

  test("opens create category modal", async () => {
    mockFetchBoth();

    renderPage();

    await waitFor(() => {
      expect(screen.getAllByText("General").length).toBeGreaterThanOrEqual(1);
    });

    fireEvent.click(screen.getByRole("button", { name: "Add Category" }));

    await waitFor(() => {
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });

    expect(screen.getByText("Add Forum Category")).toBeInTheDocument();
  });

  test("opens create forum modal", async () => {
    mockFetchBoth();

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Announcements")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Add Forum" }));

    await waitFor(() => {
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });

    expect(
      screen.getByText("Add Forum", { selector: "h2" }),
    ).toBeInTheDocument();
  });

  test("opens edit forum modal when Edit is clicked", async () => {
    mockFetchBoth();

    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Announcements")).toBeInTheDocument();
    });

    // Find edit buttons in the forums table (skip category edit buttons)
    const editButtons = screen.getAllByText("Edit");
    // Click the last edit button (which should be in the forums table)
    fireEvent.click(editButtons[editButtons.length - 1]);

    await waitFor(() => {
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });

    expect(screen.getByText("Edit Forum")).toBeInTheDocument();
  });

  test("shows conflict error when deleting category with forums", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);

    let callCount = 0;
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      callCount++;
      const url =
        typeof input === "string"
          ? input
          : ((input as Request).url ?? input.toString());

      // First two calls are the initial data load
      if (callCount <= 2) {
        if (url.includes("/admin/forum-categories")) {
          return {
            ok: true,
            json: async () => ({ categories: FAKE_CATEGORIES }),
          } as Response;
        }
        return {
          ok: true,
          json: async () => ({ forums: FAKE_FORUMS }),
        } as Response;
      }

      // Third call is the DELETE
      return {
        ok: false,
        status: 409,
        json: async () => ({
          error: {
            code: "conflict",
            message: "category has forums and cannot be deleted",
          },
        }),
      } as Response;
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getAllByText("General").length).toBeGreaterThanOrEqual(1);
    });

    const deleteButtons = screen.getAllByText("Delete");
    fireEvent.click(deleteButtons[0]);

    await waitFor(() => {
      expect(
        screen.getAllByText("category has forums and cannot be deleted").length,
      ).toBeGreaterThanOrEqual(1);
    });
  });
});
