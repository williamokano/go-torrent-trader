import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AdminUsersPage } from "@/pages/admin/AdminUsersPage";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

const users = [
  {
    id: 1,
    username: "activeuser",
    email: "active@example.test",
    group_id: 5,
    group_name: "User",
    uploaded: 1073741824,
    downloaded: 536870912,
    enabled: true,
    warned: false,
    created_at: new Date().toISOString(),
    last_access: new Date().toISOString(),
  },
  {
    id: 2,
    username: "banneduser",
    email: "banned@example.test",
    group_id: 5,
    group_name: "User",
    uploaded: 0,
    downloaded: 0,
    enabled: false,
    warned: false,
    created_at: new Date().toISOString(),
    last_access: null,
  },
  {
    id: 3,
    username: "warneduser",
    email: "warned@example.test",
    group_id: 5,
    group_name: "User",
    uploaded: 10,
    downloaded: 10,
    enabled: true,
    warned: true,
    created_at: new Date().toISOString(),
    last_access: null,
  },
];

function mockApi(list = users) {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/admin/groups")) {
      return Promise.resolve({
        ok: true,
        json: async () => ({ groups: [{ id: 5, name: "User" }] }),
      });
    }
    return Promise.resolve({
      ok: true,
      json: async () => ({ users: list, total: list.length }),
    });
  });
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/admin/users"]}>
      <AdminUsersPage />
    </MemoryRouter>,
  );
}

beforeEach(() => mockFetch.mockReset());
afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

describe("AdminUsersPage", () => {
  test("lists users with links to their detail page", async () => {
    mockApi();
    renderPage();
    const link = await screen.findByRole("link", { name: /activeuser/ });
    expect(link).toHaveAttribute("href", "/admin/users/1");
    expect(screen.getByText("active@example.test")).toBeInTheDocument();
  });

  test("shows a status badge per account state", async () => {
    mockApi();
    renderPage();
    // Scope to the table so the Status filter's <option>s don't collide.
    const table = await screen.findByRole("table");
    expect(within(table).getByText("Active")).toBeInTheDocument();
    expect(within(table).getByText("Disabled")).toBeInTheDocument();
    expect(within(table).getByText("Warned")).toBeInTheDocument();
  });

  test("shows an empty state when no users match", async () => {
    mockApi([]);
    renderPage();
    await waitFor(() => {
      expect(
        screen.getByText(/No users match these filters/),
      ).toBeInTheDocument();
    });
  });
});
