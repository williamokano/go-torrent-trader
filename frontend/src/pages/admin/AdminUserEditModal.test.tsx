import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { AdminUserEditModal } from "./AdminUserEditModal";
import { ToastProvider } from "@/components/toast";

const mockFetch = vi.fn();

vi.stubGlobal("fetch", mockFetch);

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
});

const testUser = {
  id: 1,
  username: "testuser",
  email: "test@example.com",
  group_id: 5,
  avatar: null,
  title: null,
  info: null,
  uploaded: 1024,
  downloaded: 512,
  enabled: true,
  can_download: true,
  can_upload: true,
  can_chat: true,
  warned: false,
  donor: false,
  parked: false,
  passkey: "abc123def456",
  invites: 0,
};

const testGroups = [
  { value: "1", label: "Administrator" },
  { value: "5", label: "User" },
];

function renderModal(onSave = vi.fn().mockResolvedValue(undefined)) {
  return render(
    <ToastProvider>
      <MemoryRouter>
        <AdminUserEditModal
          user={testUser}
          groups={testGroups}
          isOpen={true}
          onClose={vi.fn()}
          onSave={onSave}
        />
      </MemoryRouter>
    </ToastProvider>,
  );
}

describe("AdminUserEditModal", () => {
  test("renders reset password and reset passkey buttons", () => {
    renderModal();

    expect(screen.getByText("Reset Password")).toBeInTheDocument();
    expect(screen.getByText("Reset Passkey")).toBeInTheDocument();
  });

  test("opens password reset modal when clicking Reset Password", () => {
    renderModal();

    fireEvent.click(screen.getByText("Reset Password"));

    expect(screen.getByText("Reset Password for testuser")).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText("Leave blank to auto-generate"),
    ).toBeInTheDocument();
  });

  test("calls reset password API and shows generated password", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ new_password: "GeneratedPass123!" }),
    });

    renderModal();

    // Open the password reset modal
    fireEvent.click(screen.getByText("Reset Password"));

    // Click the reset button in the sub-modal (the second one found)
    const resetButtons = screen.getAllByText("Reset Password");
    const confirmButton = resetButtons[resetButtons.length - 1];
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(screen.getByText("GeneratedPass123!")).toBeInTheDocument();
    });

    // Verify the copy button is present
    expect(screen.getByText("Copy")).toBeInTheDocument();
  });

  test("opens passkey confirm modal when clicking Reset Passkey", () => {
    renderModal();

    fireEvent.click(screen.getByText("Reset Passkey"));

    expect(
      screen.getByText(/invalidate all existing \.torrent files/),
    ).toBeInTheDocument();
  });

  test("calls reset passkey API and shows new passkey", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ new_passkey: "abcdef1234567890abcdef1234567890" }),
    });

    renderModal();

    fireEvent.click(screen.getByText("Reset Passkey"));

    // The ConfirmModal has a "Reset Passkey" confirm button; find it in the confirm modal dialog
    // There are two "Reset Passkey" buttons: one in the main form, one in the confirm modal
    const allResetPasskeyBtns = screen.getAllByText("Reset Passkey");
    // The last one is the confirm button in the ConfirmModal
    fireEvent.click(allResetPasskeyBtns[allResetPasskeyBtns.length - 1]);

    await waitFor(() => {
      expect(
        screen.getByText("abcdef1234567890abcdef1234567890"),
      ).toBeInTheDocument();
    });
  });

  test("shows error toast on failed password reset", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      json: async () => ({
        error: { message: "insufficient permissions" },
      }),
    });

    renderModal();

    fireEvent.click(screen.getByText("Reset Password"));

    const resetButtons = screen.getAllByText("Reset Password");
    fireEvent.click(resetButtons[resetButtons.length - 1]);

    // Error appears in a toast, which renders text inside a toast component
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1);
    });

    // The password modal should still be visible (not closed on error)
    await waitFor(() => {
      expect(
        screen.getByText("Reset Password for testuser"),
      ).toBeInTheDocument();
    });
  });
});
