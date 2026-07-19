import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ConfirmModal } from "@/components/modal/ConfirmModal";

describe("ConfirmModal", () => {
  afterEach(() => {
    cleanup();
    document.body.innerHTML = "";
  });

  it("renders the title and message when open", () => {
    render(
      <ConfirmModal
        isOpen={true}
        title="Delete Thing"
        message="Are you sure?"
        onConfirm={() => {}}
        onCancel={() => {}}
      />,
    );

    expect(screen.getByText("Delete Thing")).toBeInTheDocument();
    expect(screen.getByText("Are you sure?")).toBeInTheDocument();
  });

  it("calls onConfirm when the confirm button is clicked", () => {
    const onConfirm = vi.fn();
    render(
      <ConfirmModal
        isOpen={true}
        title="Delete Thing"
        message="Are you sure?"
        confirmLabel="Delete"
        onConfirm={onConfirm}
        onCancel={() => {}}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("calls onCancel when the cancel button is clicked", () => {
    const onCancel = vi.fn();
    render(
      <ConfirmModal
        isOpen={true}
        title="Delete Thing"
        message="Are you sure?"
        onConfirm={() => {}}
        onCancel={onCancel}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("calls onCancel when Escape is pressed", () => {
    // ConfirmModal always passes through Modal's default closeOnEscape
    // behavior, so a delete/confirmation dialog should still close on
    // Escape without any extra wiring.
    const onCancel = vi.fn();
    render(
      <ConfirmModal
        isOpen={true}
        title="Delete Thing"
        message="Are you sure?"
        onConfirm={() => {}}
        onCancel={onCancel}
      />,
    );

    fireEvent.keyDown(document, { key: "Escape" });
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("disables both buttons and shows a loading label while loading", () => {
    render(
      <ConfirmModal
        isOpen={true}
        title="Delete Thing"
        message="Are you sure?"
        confirmLabel="Delete"
        loading={true}
        onConfirm={() => {}}
        onCancel={() => {}}
      />,
    );

    const confirmBtn = screen.getByRole("button", { name: "Deleting..." });
    const cancelBtn = screen.getByRole("button", { name: "Cancel" });
    expect(confirmBtn).toBeDisabled();
    expect(cancelBtn).toBeDisabled();
  });

  it("does not render when isOpen is false", () => {
    render(
      <ConfirmModal
        isOpen={false}
        title="Delete Thing"
        message="Are you sure?"
        onConfirm={() => {}}
        onCancel={() => {}}
      />,
    );

    expect(screen.queryByText("Delete Thing")).not.toBeInTheDocument();
  });
});
