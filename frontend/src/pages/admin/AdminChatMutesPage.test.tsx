import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { AdminChatMutesPage } from "./AdminChatMutesPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "test-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080" }),
}));

const mockToast = {
  success: vi.fn(),
  error: vi.fn(),
};

vi.mock("@/components/toast", () => ({
  useToast: () => mockToast,
}));

const mockMutes = [
  {
    id: 1,
    user_id: 10,
    username: "spammer",
    muted_by: 1,
    muted_by_name: "admin",
    reason: "Spamming chat",
    expires_at: "2026-04-01T00:00:00Z",
    created_at: "2026-03-01T00:00:00Z",
  },
  {
    id: 2,
    user_id: 20,
    username: "troll",
    muted_by: null,
    muted_by_name: null,
    reason: "Auto-muted: flood detection",
    expires_at: "2026-03-15T00:00:00Z",
    created_at: "2026-03-10T00:00:00Z",
  },
];

describe("AdminChatMutesPage", () => {
  beforeEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("renders the page with mutes table", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        mutes: mockMutes,
        total: 2,
        page: 1,
        per_page: 25,
      }),
    } as Response);

    render(
      <MemoryRouter>
        <AdminChatMutesPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("spammer")).toBeInTheDocument();
    });

    expect(screen.getByText("Spamming chat")).toBeInTheDocument();
    expect(screen.getByText("admin")).toBeInTheDocument();
    expect(screen.getByText("troll")).toBeInTheDocument();
    expect(screen.getByText("System")).toBeInTheDocument();
    expect(screen.getAllByText("Unmute")).toHaveLength(2);
  });

  it("shows empty state when no mutes", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({
        mutes: [],
        total: 0,
        page: 1,
        per_page: 25,
      }),
    } as Response);

    render(
      <MemoryRouter>
        <AdminChatMutesPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("No active chat mutes.")).toBeInTheDocument();
    });
  });

  it("calls unmute endpoint and shows toast on success", async () => {
    const user = userEvent.setup();
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          mutes: [mockMutes[0]],
          total: 1,
          page: 1,
          per_page: 25,
        }),
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        status: 204,
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          mutes: [],
          total: 0,
          page: 1,
          per_page: 25,
        }),
      } as Response);

    render(
      <MemoryRouter>
        <AdminChatMutesPage />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText("spammer")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Unmute"));

    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/admin/chat/users/10/mute",
        expect.objectContaining({ method: "DELETE" }),
      );
    });

    expect(mockToast.success).toHaveBeenCalledWith("spammer has been unmuted");
  });
});
