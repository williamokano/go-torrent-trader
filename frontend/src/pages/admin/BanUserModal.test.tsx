import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, test, vi } from "vitest";
import { BanUserModal } from "@/pages/admin/BanUserModal";

afterEach(cleanup);

describe("BanUserModal", () => {
  test("renders modal when open", () => {
    render(
      <BanUserModal
        isOpen={true}
        username="testuser"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByText("Ban testuser")).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText("Why is this user being banned?"),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Also ban IP address")).toBeInTheDocument();
    expect(screen.getByLabelText("Also ban email domain")).toBeInTheDocument();
    expect(screen.getByText("Ban User")).toBeInTheDocument();
    expect(screen.getByText("Cancel")).toBeInTheDocument();
  });

  test("does not render when closed", () => {
    render(
      <BanUserModal
        isOpen={false}
        username="testuser"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.queryByText("Ban testuser")).not.toBeInTheDocument();
  });

  test("confirm button is disabled when reason is empty", () => {
    render(
      <BanUserModal
        isOpen={true}
        username="testuser"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    const confirmBtn = screen.getByText("Ban User");
    expect(confirmBtn).toBeDisabled();
  });

  test("confirm button is enabled when reason is provided", () => {
    render(
      <BanUserModal
        isOpen={true}
        username="testuser"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    const textarea = screen.getByPlaceholderText(
      "Why is this user being banned?",
    );
    fireEvent.change(textarea, { target: { value: "Spamming" } });

    const confirmBtn = screen.getByText("Ban User");
    expect(confirmBtn).not.toBeDisabled();
  });

  test("calls onConfirm with correct data", () => {
    const onConfirm = vi.fn();
    render(
      <BanUserModal
        isOpen={true}
        username="testuser"
        onConfirm={onConfirm}
        onCancel={vi.fn()}
      />,
    );

    const textarea = screen.getByPlaceholderText(
      "Why is this user being banned?",
    );
    fireEvent.change(textarea, { target: { value: "Bad behavior" } });

    fireEvent.click(screen.getByLabelText("Also ban IP address"));

    const confirmBtn = screen.getByText("Ban User");
    fireEvent.click(confirmBtn);

    expect(onConfirm).toHaveBeenCalledWith({
      reason: "Bad behavior",
      ban_ip: true,
      ban_email: false,
      duration_days: null,
    });
  });

  test("calls onConfirm with duration_days when provided", () => {
    const onConfirm = vi.fn();
    render(
      <BanUserModal
        isOpen={true}
        username="testuser"
        onConfirm={onConfirm}
        onCancel={vi.fn()}
      />,
    );

    const textarea = screen.getByPlaceholderText(
      "Why is this user being banned?",
    );
    fireEvent.change(textarea, { target: { value: "Temp ban" } });

    const durationInput = screen.getByPlaceholderText(
      "Leave empty for permanent ban",
    );
    fireEvent.change(durationInput, { target: { value: "7" } });

    fireEvent.click(screen.getByText("Ban User"));

    expect(onConfirm).toHaveBeenCalledWith({
      reason: "Temp ban",
      ban_ip: false,
      ban_email: false,
      duration_days: 7,
    });
  });

  test("shows loading state", () => {
    render(
      <BanUserModal
        isOpen={true}
        username="testuser"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
        loading={true}
      />,
    );

    expect(screen.getByText("Banning...")).toBeInTheDocument();
    expect(screen.getByText("Banning...")).toBeDisabled();
  });

  test("calls onCancel when cancel button is clicked", () => {
    const onCancel = vi.fn();
    render(
      <BanUserModal
        isOpen={true}
        username="testuser"
        onConfirm={vi.fn()}
        onCancel={onCancel}
      />,
    );

    fireEvent.click(screen.getByText("Cancel"));
    expect(onCancel).toHaveBeenCalled();
  });

  test("resets state when isOpen changes to false", () => {
    const onConfirm = vi.fn();
    const { rerender } = render(
      <BanUserModal
        isOpen={true}
        username="testuser"
        onConfirm={onConfirm}
        onCancel={vi.fn()}
      />,
    );

    // Fill in some state
    const textarea = screen.getByPlaceholderText(
      "Why is this user being banned?",
    );
    fireEvent.change(textarea, { target: { value: "Bad behavior" } });
    fireEvent.click(screen.getByLabelText("Also ban IP address"));

    // Close the modal
    rerender(
      <BanUserModal
        isOpen={false}
        username="testuser"
        onConfirm={onConfirm}
        onCancel={vi.fn()}
      />,
    );

    // Reopen the modal
    rerender(
      <BanUserModal
        isOpen={true}
        username="testuser"
        onConfirm={onConfirm}
        onCancel={vi.fn()}
      />,
    );

    // State should be reset
    const newTextarea = screen.getByPlaceholderText(
      "Why is this user being banned?",
    );
    expect(newTextarea).toHaveValue("");
    expect(screen.getByLabelText("Also ban IP address")).not.toBeChecked();
    expect(screen.getByText("Ban User")).toBeDisabled();
  });

  test("shows domain warning when ban email is checked", () => {
    render(
      <BanUserModal
        isOpen={true}
        username="testuser"
        email="baduser@evil.com"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByText(/Also ban email domain/));

    expect(
      screen.getByText(/will block all future registrations from \*@evil\.com/),
    ).toBeInTheDocument();
  });
});
