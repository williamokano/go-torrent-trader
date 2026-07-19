import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AdminGroupsPage } from "@/pages/admin/AdminGroupsPage";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

interface FetchInit {
  method?: string;
  body?: string;
}

const vipGroup = {
  id: 7,
  name: "VIP",
  slug: "vip",
  level: 60,
  color: "#FFA500",
  can_upload: true,
  can_download: true,
  can_invite: true,
  can_comment: true,
  can_forum: true,
  is_admin: false,
  is_moderator: false,
  is_immune: false,
};

function mockApi() {
  const calls: { method: string; url: string; body?: string }[] = [];
  mockFetch.mockImplementation((url: string, init?: FetchInit) => {
    const method = init?.method ?? "GET";
    calls.push({ method, url, body: init?.body });
    if (method === "GET") {
      return Promise.resolve({
        ok: true,
        json: async () => ({ groups: [vipGroup] }),
      });
    }
    // POST / PUT / DELETE
    return Promise.resolve({ ok: true, json: async () => ({}) });
  });
  return calls;
}

function renderPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <AdminGroupsPage />
      </ToastProvider>
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  mockFetch.mockReset();
  vi.restoreAllMocks();
});

beforeEach(() => {
  mockFetch.mockReset();
});

describe("AdminGroupsPage", () => {
  test("lists groups from the API", async () => {
    mockApi();
    renderPage();
    expect(await screen.findByText("VIP")).toBeInTheDocument();
  });

  test("creates a group through the New Group modal", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("VIP");
    await user.click(screen.getByRole("button", { name: "New group" }));

    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByLabelText("Name"), "Seedbox");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === "POST" && c.url.endsWith("/admin/groups"),
        ),
      ).toBe(true);
    });
    const post = calls.find((c) => c.method === "POST");
    expect(JSON.parse(post!.body!)).toMatchObject({ name: "Seedbox" });
  });

  test("does not close the group form on Escape, preserving in-progress input", async () => {
    mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("VIP");
    await user.click(screen.getByRole("button", { name: "New group" }));

    const dialog = await screen.findByRole("dialog");
    const nameInput = within(dialog).getByLabelText("Name");
    await user.type(nameInput, "Draft group name");

    await user.keyboard("{Escape}");

    // The edit/create form is a real-data-entry modal, so it opts out of
    // Escape-to-close (unlike the delete ConfirmModal above) — a stray
    // Escape press must not discard what the admin was typing.
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(nameInput).toHaveValue("Draft group name");
  });

  test("deletes a group after confirmation", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("VIP");
    await user.click(screen.getByRole("button", { name: "Delete" }));

    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(/Delete the "VIP" group\?/),
    ).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === "DELETE" && c.url.endsWith("/admin/groups/7"),
        ),
      ).toBe(true);
    });
  });

  test("does not delete when confirmation is declined", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("VIP");
    await user.click(screen.getByRole("button", { name: "Delete" }));

    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(calls.some((c) => c.method === "DELETE")).toBe(false);
  });

  test("does not delete when confirmation is dismissed via Escape", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("VIP");
    await user.click(screen.getByRole("button", { name: "Delete" }));

    await screen.findByRole("dialog");
    await user.keyboard("{Escape}");

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(calls.some((c) => c.method === "DELETE")).toBe(false);
  });
});
