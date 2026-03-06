import {
  cleanup,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { UploadPage } from "@/pages/UploadPage";
import { ToastProvider } from "@/components/toast";
import { clearTokens } from "@/features/auth/token";

const mockGET = vi.fn();

vi.mock("@/api", () => ({
  api: {
    GET: (...args: unknown[]) => mockGET(...args),
  },
}));

const FAKE_CATEGORIES = [
  { id: 1, name: "Movies", parent_id: null, sort_order: 1 },
  { id: 2, name: "TV", parent_id: null, sort_order: 2 },
  { id: 3, name: "Music", parent_id: null, sort_order: 3 },
  { id: 4, name: "Games", parent_id: null, sort_order: 4 },
  { id: 5, name: "Software", parent_id: null, sort_order: 5 },
  { id: 6, name: "Anime", parent_id: null, sort_order: 6 },
  { id: 7, name: "Books", parent_id: null, sort_order: 7 },
  { id: 8, name: "Other", parent_id: null, sort_order: 8 },
];

const mockNavigate = vi.fn();

vi.mock("react-router-dom", async () => {
  const actual =
    await vi.importActual<typeof import("react-router-dom")>(
      "react-router-dom",
    );
  return { ...actual, useNavigate: () => mockNavigate };
});

vi.mock("@/features/auth/token", async () => {
  const actual = await vi.importActual<typeof import("@/features/auth/token")>(
    "@/features/auth/token",
  );
  return { ...actual, getAccessToken: () => "fake-token" };
});

vi.mock("@/config", () => ({
  getConfig: () => ({ API_URL: "http://localhost:8080", SITE_NAME: "Test" }),
}));

afterEach(cleanup);

beforeEach(() => {
  clearTokens();
  localStorage.clear();
  vi.clearAllMocks();
  vi.restoreAllMocks();
  mockGET.mockResolvedValue({
    data: { categories: FAKE_CATEGORIES },
    error: undefined,
  });
});

function renderUploadPage() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={["/upload"]}>
        <UploadPage />
      </MemoryRouter>
    </ToastProvider>,
  );
}

function createTorrentFile(name = "test.torrent") {
  return new File(["fake-torrent-content"], name, {
    type: "application/x-bittorrent",
  });
}

describe("UploadPage", () => {
  test("renders all form fields", () => {
    renderUploadPage();
    expect(
      screen.getByText("Drop .torrent file here or click to browse"),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Category")).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toBeInTheDocument();
    expect(screen.getByLabelText("Description")).toBeInTheDocument();
    expect(screen.getByLabelText("Upload anonymously")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Upload" })).toBeInTheDocument();
  });

  test("renders all category options", async () => {
    renderUploadPage();
    await waitFor(() => {
      const select = screen.getByLabelText("Category") as HTMLSelectElement;
      const options = Array.from(select.options).map((o) => o.text);
      expect(options).toEqual([
        "Select a category",
        "Movies",
        "TV",
        "Music",
        "Games",
        "Software",
        "Anime",
        "Books",
        "Other",
      ]);
    });
  });

  test("shows file name after selecting a .torrent file", () => {
    renderUploadPage();
    const input = screen.getByTestId("file-input") as HTMLInputElement;
    const file = createTorrentFile("my-movie.torrent");
    fireEvent.change(input, { target: { files: [file] } });

    expect(screen.getByText("my-movie.torrent")).toBeInTheDocument();
  });

  test("auto-fills name from torrent filename", () => {
    renderUploadPage();
    const input = screen.getByTestId("file-input") as HTMLInputElement;
    const file = createTorrentFile("awesome-linux-iso.torrent");
    fireEvent.change(input, { target: { files: [file] } });

    const nameInput = screen.getByLabelText("Name") as HTMLInputElement;
    expect(nameInput.value).toBe("awesome-linux-iso");
  });

  test("shows error for non-.torrent file", () => {
    renderUploadPage();
    const input = screen.getByTestId("file-input") as HTMLInputElement;
    const file = new File(["content"], "readme.txt", { type: "text/plain" });
    fireEvent.change(input, { target: { files: [file] } });

    expect(
      screen.getByText("Please select a .torrent file"),
    ).toBeInTheDocument();
  });

  test("shows file error when submitting without a file", async () => {
    renderUploadPage();

    // Select a category so that's not the blocker
    fireEvent.change(screen.getByLabelText("Category"), {
      target: { value: "1" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Upload" }));

    await waitFor(() => {
      expect(
        screen.getByText("A .torrent file is required"),
      ).toBeInTheDocument();
    });
  });

  test("shows toast error when submitting without a category", async () => {
    renderUploadPage();

    // Add a file but no category
    const input = screen.getByTestId("file-input") as HTMLInputElement;
    fireEvent.change(input, {
      target: { files: [createTorrentFile()] },
    });

    fireEvent.click(screen.getByRole("button", { name: "Upload" }));

    await waitFor(() => {
      expect(screen.getByText("Please select a category")).toBeInTheDocument();
    });
  });

  test("submits form data and navigates on success", async () => {
    const mockFetch = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ torrent: { id: 42 } }), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      }),
    );

    renderUploadPage();

    // Wait for categories to load
    await waitFor(() => {
      const select = screen.getByLabelText("Category") as HTMLSelectElement;
      expect(select.options.length).toBeGreaterThan(1);
    });

    // Fill in the form
    const fileInput = screen.getByTestId("file-input") as HTMLInputElement;
    fireEvent.change(fileInput, {
      target: { files: [createTorrentFile()] },
    });
    fireEvent.change(screen.getByLabelText("Category"), {
      target: { value: "1" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Upload" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1);
    });

    const [url, options] = mockFetch.mock.calls[0];
    expect(url).toBe("http://localhost:8080/api/v1/torrents");
    expect(options?.method).toBe("POST");
    expect((options?.headers as Record<string, string>)["Authorization"]).toBe(
      "Bearer fake-token",
    );
    expect(options?.body).toBeInstanceOf(FormData);

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/torrent/42");
    });
  });

  test("shows error toast on API failure", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({ error: { message: "Duplicate torrent" } }),
        { status: 409, headers: { "Content-Type": "application/json" } },
      ),
    );

    renderUploadPage();

    // Wait for categories to load
    await waitFor(() => {
      const select = screen.getByLabelText("Category") as HTMLSelectElement;
      expect(select.options.length).toBeGreaterThan(1);
    });

    const fileInput = screen.getByTestId("file-input") as HTMLInputElement;
    fireEvent.change(fileInput, {
      target: { files: [createTorrentFile()] },
    });
    fireEvent.change(screen.getByLabelText("Category"), {
      target: { value: "1" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Upload" }));

    await waitFor(() => {
      expect(screen.getByText("Duplicate torrent")).toBeInTheDocument();
    });
  });

  test("shows loading state while submitting", async () => {
    let resolveUpload: (value: Response) => void;
    vi.spyOn(globalThis, "fetch").mockReturnValueOnce(
      new Promise<Response>((resolve) => {
        resolveUpload = resolve;
      }),
    );

    renderUploadPage();

    // Wait for categories to load
    await waitFor(() => {
      const select = screen.getByLabelText("Category") as HTMLSelectElement;
      expect(select.options.length).toBeGreaterThan(1);
    });

    const fileInput = screen.getByTestId("file-input") as HTMLInputElement;
    fireEvent.change(fileInput, {
      target: { files: [createTorrentFile()] },
    });
    fireEvent.change(screen.getByLabelText("Category"), {
      target: { value: "1" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Upload" }));

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Uploading..." }),
      ).toBeDisabled();
    });

    resolveUpload!(
      new Response(JSON.stringify({ torrent: { id: 1 } }), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Upload" })).not.toBeDisabled();
    });
  });

  test("navigates to /browse when response has no torrent id", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({}), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      }),
    );

    renderUploadPage();

    // Wait for categories to load
    await waitFor(() => {
      const select = screen.getByLabelText("Category") as HTMLSelectElement;
      expect(select.options.length).toBeGreaterThan(1);
    });

    const fileInput = screen.getByTestId("file-input") as HTMLInputElement;
    fireEvent.change(fileInput, {
      target: { files: [createTorrentFile()] },
    });
    fireEvent.change(screen.getByLabelText("Category"), {
      target: { value: "1" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Upload" }));

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/browse");
    });
  });
});
