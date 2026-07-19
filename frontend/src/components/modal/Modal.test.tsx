import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Modal } from "@/components/modal/Modal";

describe("Modal", () => {
  afterEach(() => {
    cleanup();
    // Remove any portal elements that were appended to document.body
    document.body.innerHTML = "";
  });

  it("renders children when isOpen is true", () => {
    render(
      <Modal isOpen={true} onClose={() => {}}>
        <p>Modal content</p>
      </Modal>,
    );
    expect(screen.getByText("Modal content")).toBeInTheDocument();
  });

  it("does not render when isOpen is false", () => {
    render(
      <Modal isOpen={false} onClose={() => {}}>
        <p>Hidden content</p>
      </Modal>,
    );
    expect(screen.queryByText("Hidden content")).not.toBeInTheDocument();
  });

  it("calls onClose when Escape is pressed", () => {
    const onClose = vi.fn();
    render(
      <Modal isOpen={true} onClose={onClose}>
        <p>Escape test</p>
      </Modal>,
    );

    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does not call onClose on Escape when closeOnEscape is false", () => {
    const onClose = vi.fn();
    render(
      <Modal isOpen={true} onClose={onClose} closeOnEscape={false}>
        <p>Escape opt-out test</p>
      </Modal>,
    );

    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).not.toHaveBeenCalled();
  });

  it("calls onClose when overlay is clicked", () => {
    const onClose = vi.fn();
    render(
      <Modal isOpen={true} onClose={onClose} title="Overlay test">
        <p>Overlay content</p>
      </Modal>,
    );

    const overlay = screen.getByRole("dialog");
    fireEvent.click(overlay);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("calls onClose when the close button is clicked", () => {
    const onClose = vi.fn();
    render(
      <Modal isOpen={true} onClose={onClose} title="Close button test">
        <p>Content</p>
      </Modal>,
    );

    fireEvent.click(screen.getByLabelText("Close modal"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does not call onClose on overlay click when closeOnDismissClick is false", () => {
    const onClose = vi.fn();
    render(
      <Modal
        isOpen={true}
        onClose={onClose}
        title="Overlay opt-out test"
        closeOnDismissClick={false}
      >
        <p>Overlay opt-out content</p>
      </Modal>,
    );

    fireEvent.click(screen.getByRole("dialog"));
    expect(onClose).not.toHaveBeenCalled();
  });

  it("does not call onClose on close button click when closeOnDismissClick is false", () => {
    const onClose = vi.fn();
    render(
      <Modal
        isOpen={true}
        onClose={onClose}
        title="Close button opt-out test"
        closeOnDismissClick={false}
      >
        <p>Content</p>
      </Modal>,
    );

    fireEvent.click(screen.getByLabelText("Close modal"));
    expect(onClose).not.toHaveBeenCalled();
  });

  it("marks the close button aria-disabled when closeOnDismissClick is false", () => {
    render(
      <Modal
        isOpen={true}
        onClose={() => {}}
        title="Aria-disabled test"
        closeOnDismissClick={false}
      >
        <p>Content</p>
      </Modal>,
    );

    expect(screen.getByLabelText("Close modal")).toHaveAttribute(
      "aria-disabled",
      "true",
    );
  });

  it("does not mark the close button aria-disabled by default", () => {
    render(
      <Modal isOpen={true} onClose={() => {}} title="Aria-enabled test">
        <p>Content</p>
      </Modal>,
    );

    expect(screen.getByLabelText("Close modal")).toHaveAttribute(
      "aria-disabled",
      "false",
    );
  });

  it("still does not call onClose on content click when closeOnDismissClick is false", () => {
    const onClose = vi.fn();
    render(
      <Modal
        isOpen={true}
        onClose={onClose}
        title="Content click opt-out test"
        closeOnDismissClick={false}
      >
        <p>Click me</p>
      </Modal>,
    );

    fireEvent.click(screen.getByText("Click me"));
    expect(onClose).not.toHaveBeenCalled();
  });

  it("still calls onClose on Escape when only closeOnDismissClick is false", () => {
    const onClose = vi.fn();
    render(
      <Modal
        isOpen={true}
        onClose={onClose}
        title="Independent flags test"
        closeOnDismissClick={false}
      >
        <p>Content</p>
      </Modal>,
    );

    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("still calls onClose on overlay click when only closeOnEscape is false", () => {
    const onClose = vi.fn();
    render(
      <Modal
        isOpen={true}
        onClose={onClose}
        title="Independent flags test 2"
        closeOnEscape={false}
      >
        <p>Content</p>
      </Modal>,
    );

    fireEvent.click(screen.getByRole("dialog"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does not call onClose when content is clicked", () => {
    const onClose = vi.fn();
    render(
      <Modal isOpen={true} onClose={onClose}>
        <p>Click me</p>
      </Modal>,
    );

    fireEvent.click(screen.getByText("Click me"));
    expect(onClose).not.toHaveBeenCalled();
  });

  it("renders title when provided", () => {
    render(
      <Modal isOpen={true} onClose={() => {}} title="My Modal">
        <p>Title test</p>
      </Modal>,
    );
    expect(screen.getByText("My Modal")).toBeInTheDocument();
  });
});
