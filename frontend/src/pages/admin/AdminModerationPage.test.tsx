import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { AdminModerationPage } from "./AdminModerationPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "test-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080" }),
}));

const mockToast = { success: vi.fn(), error: vi.fn() };
vi.mock("@/components/toast", () => ({
  useToast: () => mockToast,
}));

const queue = [
  {
    id: 1,
    name: "Pending Movie",
    uploader_name: "alice",
    category_name: "Movies",
    size: 1024,
    created_at: "2026-07-01T00:00:00Z",
    moderation: { status: "pending", message_count: 2 },
  },
  {
    id: 2,
    name: "Claimed Show",
    uploader_name: "bob",
    category_name: "TV",
    size: 2048,
    created_at: "2026-07-02T00:00:00Z",
    moderation: {
      status: "pending",
      assigned_moderator_id: 9,
      assigned_moderator_name: "mod",
      message_count: 0,
    },
  },
];

function okJson(body: unknown): Response {
  return { ok: true, json: async () => body } as Response;
}

describe("AdminModerationPage", () => {
  beforeEach(() => {
    cleanup();
    vi.restoreAllMocks();
    mockToast.success.mockClear();
    mockToast.error.mockClear();
  });

  it("renders the queue with assignment state", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      okJson({ torrents: queue, total: 2 }),
    );

    render(
      <MemoryRouter>
        <AdminModerationPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("Pending Movie")).toBeInTheDocument();
    });
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("mod")).toBeInTheDocument();
    // "Unassigned" appears both as a filter button and as t1's moderator cell.
    expect(screen.getAllByText("Unassigned").length).toBeGreaterThanOrEqual(2);
    // Only the unassigned torrent offers a Claim button.
    expect(screen.getAllByText("Claim")).toHaveLength(1);
  });

  it("shows an empty state", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      okJson({ torrents: [], total: 0 }),
    );
    render(
      <MemoryRouter>
        <AdminModerationPage />
      </MemoryRouter>,
    );
    await waitFor(() => {
      expect(
        screen.getByText("Nothing awaiting moderation here."),
      ).toBeInTheDocument();
    });
  });

  it("claims a torrent and refetches", async () => {
    const user = userEvent.setup();
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(okJson({ torrents: queue, total: 2 }))
      .mockResolvedValueOnce(okJson({ torrent: {} }))
      .mockResolvedValueOnce(okJson({ torrents: queue, total: 2 }));

    render(
      <MemoryRouter>
        <AdminModerationPage />
      </MemoryRouter>,
    );

    await waitFor(() => screen.getByText("Pending Movie"));
    await user.click(screen.getByText("Claim"));

    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/admin/moderation/torrents/1/claim",
        expect.objectContaining({ method: "POST" }),
      );
    });
    expect(mockToast.success).toHaveBeenCalledWith("Claimed for review");
  });

  it("switches the assignment filter", async () => {
    const user = userEvent.setup();
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(okJson({ torrents: queue, total: 2 }));

    render(
      <MemoryRouter>
        <AdminModerationPage />
      </MemoryRouter>,
    );

    await waitFor(() => screen.getByText("Pending Movie"));
    await user.click(screen.getByRole("button", { name: "Unassigned" }));

    await waitFor(() => {
      expect(
        fetchSpy.mock.calls.some((c) =>
          String(c[0]).includes("assigned=unassigned"),
        ),
      ).toBe(true);
    });
  });
});
