import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, test, expect, vi } from "vitest";
import { RatingWidget } from "@/components/RatingWidget";
import { ToastProvider } from "@/components/toast";
import { AuthContext } from "@/features/auth/AuthContextDef";
import type { AuthContextValue, User } from "@/features/auth/AuthContextDef";

vi.mock("@/features/auth/token", () => ({
  getAccessToken: () => "fake-token",
}));

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

const FAKE_RATING = {
  average: 4.2,
  count: 15,
  user_rating: 4,
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
    isStaff: false,
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

function renderRating(
  torrentId = "1",
  authContext: AuthContextValue = makeAuthContext(),
) {
  return render(
    <AuthContext.Provider value={authContext}>
      <ToastProvider>
        <RatingWidget torrentId={torrentId} />
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
    new Response(JSON.stringify(FAKE_RATING), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
});

describe("RatingWidget", () => {
  test("shows loading state initially", () => {
    vi.spyOn(globalThis, "fetch").mockReturnValue(new Promise(() => {}));
    renderRating();
    expect(screen.getByText("Loading rating...")).toBeInTheDocument();
  });

  test("renders average and count after loading", async () => {
    renderRating();
    await waitFor(() => {
      expect(screen.getByText("4.2")).toBeInTheDocument();
    });
    expect(screen.getByText("(15 votes)")).toBeInTheDocument();
  });

  test("renders 5 star buttons", async () => {
    renderRating();
    await waitFor(() => {
      expect(screen.getByText("4.2")).toBeInTheDocument();
    });
    const stars = screen.getAllByRole("button", { name: /Rate \d star/ });
    expect(stars).toHaveLength(5);
  });

  test("shows N/A when no ratings", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ average: 0, count: 0 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    renderRating();
    await waitFor(() => {
      expect(screen.getByText("N/A")).toBeInTheDocument();
    });
    expect(screen.getByText("(0 votes)")).toBeInTheDocument();
  });

  test("shows singular 'vote' for count of 1", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ average: 5, count: 1 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    renderRating();
    await waitFor(() => {
      expect(screen.getByText("(1 vote)")).toBeInTheDocument();
    });
  });

  test("star buttons disabled for unauthenticated users", async () => {
    renderRating("1", makeAuthContext(null));
    await waitFor(() => {
      const stars = screen.getAllByRole("button", { name: /Rate \d star/ });
      stars.forEach((star) => expect(star).toBeDisabled());
    });
  });

  test("clicking a star submits rating via POST", async () => {
    const mockFetch = vi.spyOn(globalThis, "fetch");
    mockFetch
      .mockResolvedValueOnce(
        new Response(JSON.stringify(FAKE_RATING), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(new Response(null, { status: 201 }))
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ average: 4.3, count: 16, user_rating: 5 }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );

    renderRating();

    await waitFor(() => {
      expect(screen.getByText("4.2")).toBeInTheDocument();
    });

    const star5 = screen.getByRole("button", { name: "Rate 5 stars" });
    fireEvent.click(star5);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/torrents/1/rating",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ rating: 5 }),
        }),
      );
    });
  });

  test("fetches rating with authorization header", async () => {
    const mockFetch = vi.spyOn(globalThis, "fetch");
    mockFetch.mockResolvedValue(
      new Response(JSON.stringify(FAKE_RATING), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    renderRating();

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/torrents/1/rating",
        expect.objectContaining({
          headers: { Authorization: "Bearer fake-token" },
        }),
      );
    });
  });
});
