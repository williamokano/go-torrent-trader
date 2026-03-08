import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { StaffPage } from "@/pages/StaffPage";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const FAKE_STAFF = [
  {
    id: 1,
    username: "superadmin",
    group_id: 1,
    group_name: "Administrator",
    title: "Site Owner",
  },
  {
    id: 2,
    username: "admin2",
    group_id: 1,
    group_name: "Administrator",
    title: null,
  },
  {
    id: 3,
    username: "moduser",
    group_id: 2,
    group_name: "Moderator",
    title: "Forum Mod",
  },
];

const mockFetch = vi.fn();

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mockFetch.mockResolvedValue({
    ok: true,
    json: () => Promise.resolve({ staff: FAKE_STAFF }),
  });
  vi.stubGlobal("fetch", mockFetch);
});

function renderStaffPage() {
  return render(
    <MemoryRouter initialEntries={["/staff"]}>
      <StaffPage />
    </MemoryRouter>,
  );
}

describe("StaffPage", () => {
  test("renders page title", () => {
    renderStaffPage();
    expect(screen.getByText("Staff")).toBeInTheDocument();
  });

  test("shows loading state initially", () => {
    mockFetch.mockReturnValue(new Promise(() => {}));
    renderStaffPage();
    expect(screen.getByText("Loading staff...")).toBeInTheDocument();
  });

  test("renders staff members grouped by role", async () => {
    renderStaffPage();
    await waitFor(() => {
      expect(screen.getByText("superadmin")).toBeInTheDocument();
    });
    expect(screen.getByText("admin2")).toBeInTheDocument();
    expect(screen.getByText("moduser")).toBeInTheDocument();
  });

  test("renders group section headers", async () => {
    renderStaffPage();
    await waitFor(() => {
      expect(screen.getByText("Administrators")).toBeInTheDocument();
    });
    expect(screen.getByText("Moderators")).toBeInTheDocument();
  });

  test("renders staff title when present", async () => {
    renderStaffPage();
    await waitFor(() => {
      expect(screen.getByText("Site Owner")).toBeInTheDocument();
    });
    expect(screen.getByText("Forum Mod")).toBeInTheDocument();
  });

  test("does not render title for staff without one", async () => {
    renderStaffPage();
    await waitFor(() => {
      expect(screen.getByText("admin2")).toBeInTheDocument();
    });
    // admin2 has no title, so it should only show the username
    const card = screen.getByText("admin2").closest(".staff__card");
    expect(card?.querySelector(".staff__card-title")).toBeNull();
  });

  test("shows empty state when no staff", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ staff: [] }),
    });
    renderStaffPage();
    await waitFor(() => {
      expect(screen.getByText("No staff members found.")).toBeInTheDocument();
    });
  });

  test("shows error state on API failure", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({ error: { message: "Server error" } }),
    });
    renderStaffPage();
    await waitFor(() => {
      expect(screen.getByText("Server error")).toBeInTheDocument();
    });
  });

  test("passes authorization header to fetch", async () => {
    renderStaffPage();
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/users/staff",
        expect.objectContaining({
          headers: { Authorization: "Bearer fake-token" },
        }),
      );
    });
  });
});
