import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { BonusStorePage } from "@/pages/BonusStorePage";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

const mockRefreshUser = vi.fn();
let mockBalance = 6000;

vi.mock("@/features/auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/auth")>();
  return {
    ...actual,
    useAuth: () => ({
      user: { id: 1, username: "buyer", bonus_points: mockBalance },
      isAuthenticated: true,
      isLoading: false,
      refreshUser: mockRefreshUser,
    }),
  };
});

const items = [
  {
    id: 1,
    slug: "invite-1",
    kind: "invite",
    name: "1 Invite",
    description: "One invitation to bring a friend aboard.",
    price: 5000,
    reward_amount: 1,
  },
  {
    id: 2,
    slug: "upload-5gib",
    kind: "upload_credit",
    name: "5 GiB Upload Credit",
    description: "Adds 5 GiB to your uploaded total.",
    price: 3000,
    reward_amount: 5368709120,
  },
];

interface FetchInit {
  method?: string;
}

function mockApi(options?: { enabled?: boolean; purchaseError?: string }) {
  const calls: { method: string; url: string }[] = [];
  mockFetch.mockImplementation((url: string, init?: FetchInit) => {
    const method = init?.method ?? "GET";
    calls.push({ method, url });
    if (method === "GET") {
      return Promise.resolve({
        ok: true,
        json: async () => ({
          enabled: options?.enabled ?? true,
          items: options?.enabled === false ? [] : items,
        }),
      });
    }
    if (options?.purchaseError) {
      return Promise.resolve({
        ok: false,
        json: async () => ({
          error: { code: "conflict", message: options.purchaseError },
        }),
      });
    }
    return Promise.resolve({
      ok: true,
      json: async () => ({ item_id: 1, slug: "invite-1", new_balance: 1000 }),
    });
  });
  return calls;
}

function renderPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <BonusStorePage />
      </ToastProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  mockFetch.mockReset();
  mockRefreshUser.mockReset();
  mockBalance = 6000;
});
afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

describe("BonusStorePage", () => {
  test("renders the balance and store items", async () => {
    mockApi();
    renderPage();
    expect(await screen.findByText("1 Invite")).toBeInTheDocument();
    expect(screen.getByText("5 GiB Upload Credit")).toBeInTheDocument();
    expect(screen.getByText("6,000")).toBeInTheDocument();
  });

  test("shows the disabled message when the store is off", async () => {
    mockApi({ enabled: false });
    renderPage();
    expect(
      await screen.findByText("The bonus store is currently disabled."),
    ).toBeInTheDocument();
    expect(screen.queryByText("1 Invite")).not.toBeInTheDocument();
  });

  test("purchases an item and refreshes the auth balance", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("1 Invite");
    const buyButtons = screen.getAllByRole("button", { name: "Buy" });
    await user.click(buyButtons[0]);

    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.method === "POST" && c.url.endsWith("/store/purchase/1"),
        ),
      ).toBe(true);
    });
    expect(mockRefreshUser).toHaveBeenCalled();
  });

  test("surfaces the server error message on a failed purchase", async () => {
    mockApi({ purchaseError: "not enough bonus points" });
    const user = userEvent.setup();
    renderPage();

    await screen.findByText("1 Invite");
    await user.click(screen.getAllByRole("button", { name: "Buy" })[0]);

    expect(
      await screen.findByText("not enough bonus points"),
    ).toBeInTheDocument();
    expect(mockRefreshUser).not.toHaveBeenCalled();
  });

  test("disables Buy when the balance cannot cover the price", async () => {
    mockBalance = 100;
    mockApi();
    renderPage();

    await screen.findByText("1 Invite");
    for (const btn of screen.getAllByRole("button", { name: "Buy" })) {
      expect(btn).toBeDisabled();
    }
  });
});
