import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ChatModMenu } from "@/components/ChatModMenu";

const mockChat = {
  deleteUserMessages: vi.fn(),
  muteUser: vi.fn(),
  unmuteUser: vi.fn(),
};

vi.mock("@/lib/useChat", () => ({
  useChat: () => mockChat,
}));

function renderMenu() {
  return render(
    <MemoryRouter>
      <ChatModMenu userId={2} username="bob" />
      {/* Sibling element to click as an "outside" target */}
      <div data-testid="outside">outside</div>
    </MemoryRouter>,
  );
}

function openMenu() {
  fireEvent.click(screen.getByTitle("Moderation actions"));
}

function openMuteForm() {
  fireEvent.click(screen.getByText("Mute user"));
}

describe("ChatModMenu", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("renders the username as the trigger and keeps the dropdown closed initially", () => {
    renderMenu();
    expect(screen.getByTitle("Moderation actions").textContent).toBe("bob");
    expect(screen.queryByText("Delete all messages")).not.toBeInTheDocument();
  });

  it("opens the dropdown on trigger click", () => {
    renderMenu();
    openMenu();
    expect(screen.getByText("View profile")).toBeInTheDocument();
    expect(screen.getByText("Delete all messages")).toBeInTheDocument();
    expect(screen.getByText("Mute user")).toBeInTheDocument();
    expect(screen.getByText("Unmute user")).toBeInTheDocument();
  });

  it("closes the dropdown on Escape when the mute form is not open", () => {
    renderMenu();
    openMenu();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByText("Delete all messages")).not.toBeInTheDocument();
  });

  it("closes the dropdown on outside click when the mute form is not open", () => {
    renderMenu();
    openMenu();
    fireEvent.mouseDown(screen.getByTestId("outside"));
    expect(screen.queryByText("Delete all messages")).not.toBeInTheDocument();
  });

  it("shows the mute duration/reason inputs when Mute user is clicked", () => {
    renderMenu();
    openMenu();
    openMuteForm();
    expect(screen.getByPlaceholderText("e.g. 10")).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText("Reason (optional)"),
    ).toBeInTheDocument();
  });

  it("does not close the menu or discard the mute form on a stray Escape press", () => {
    renderMenu();
    openMenu();
    openMuteForm();

    fireEvent.change(screen.getByPlaceholderText("e.g. 10"), {
      target: { value: "60" },
    });
    fireEvent.change(screen.getByPlaceholderText("Reason (optional)"), {
      target: { value: "spamming links" },
    });

    fireEvent.keyDown(document, { key: "Escape" });

    // Menu and form are still open, and the in-progress values survived.
    expect(screen.getByText("Delete all messages")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("e.g. 10")).toHaveValue(60);
    expect(screen.getByPlaceholderText("Reason (optional)")).toHaveValue(
      "spamming links",
    );
  });

  it("does not close the menu or discard the mute form on an outside click", () => {
    renderMenu();
    openMenu();
    openMuteForm();

    fireEvent.change(screen.getByPlaceholderText("Reason (optional)"), {
      target: { value: "in progress reason" },
    });

    fireEvent.mouseDown(screen.getByTestId("outside"));

    expect(screen.getByText("Delete all messages")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Reason (optional)")).toHaveValue(
      "in progress reason",
    );
  });

  it("still allows closing the mute form explicitly by toggling Mute user again", () => {
    renderMenu();
    openMenu();
    openMuteForm();
    expect(screen.getByPlaceholderText("e.g. 10")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Mute user"));

    expect(screen.queryByPlaceholderText("e.g. 10")).not.toBeInTheDocument();
    // The rest of the menu is still open.
    expect(screen.getByText("Delete all messages")).toBeInTheDocument();
  });

  it("can never get stuck open: re-clicking the trigger always closes everything, even mid-mute-form", () => {
    renderMenu();
    openMenu();
    openMuteForm();

    fireEvent.change(screen.getByPlaceholderText("Reason (optional)"), {
      target: { value: "will be abandoned" },
    });

    // The trigger's own onClick is unconditional — it is the guaranteed
    // escape valve even while Escape/outside-click are suppressed by the
    // open mute form.
    fireEvent.click(screen.getByTitle("Moderation actions"));

    expect(screen.queryByText("Delete all messages")).not.toBeInTheDocument();
    expect(
      screen.queryByPlaceholderText("Reason (optional)"),
    ).not.toBeInTheDocument();
  });

  it("submits the mute and closes the whole menu on Confirm mute", async () => {
    renderMenu();
    openMenu();
    openMuteForm();

    fireEvent.change(screen.getByPlaceholderText("e.g. 10"), {
      target: { value: "30" },
    });
    fireEvent.click(screen.getByText("Confirm mute"));

    expect(mockChat.muteUser).toHaveBeenCalledWith(2, 30, "");
    await vi.waitFor(() => {
      expect(screen.queryByText("Delete all messages")).not.toBeInTheDocument();
    });
  });

  it("opens a confirmation modal for Delete all messages and calls deleteUserMessages on confirm", async () => {
    renderMenu();
    openMenu();
    fireEvent.click(screen.getByText("Delete all messages"));

    expect(
      screen.getByText("Delete all chat messages from bob?"),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByText("Delete All"));

    await vi.waitFor(() => {
      expect(mockChat.deleteUserMessages).toHaveBeenCalledWith(2);
    });
  });
});
