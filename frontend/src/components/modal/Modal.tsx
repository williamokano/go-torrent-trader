import { useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import "@/components/modal/modal.css";

interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title?: string;
  children: React.ReactNode;
  /**
   * Whether pressing Escape closes the modal. Defaults to true.
   * Set to false for modals with free-text/multi-field inputs the user may
   * be actively composing (edit/create forms), so a stray Escape press
   * doesn't silently discard in-progress input. Confirmation modals and
   * static-content modals should keep the default.
   */
  closeOnEscape?: boolean;
  /**
   * Whether clicking the overlay backdrop or the "x" close button closes
   * the modal. Defaults to true. Set to false for the same free-text/
   * multi-field edit and create forms that opt out of closeOnEscape, so a
   * stray click outside the form (or a reflexive click on "x") doesn't
   * silently discard in-progress input. Kept as a separate flag from
   * closeOnEscape rather than folded into it: pointer-driven dismissal and
   * the Escape key are different enough interactions that a future modal
   * could plausibly want to allow one but not the other. When false, the
   * "x" button is rendered visibly disabled (dimmed, aria-disabled) rather
   * than left looking active while silently doing nothing — every modal
   * that uses this flag still exposes an explicit in-form Cancel button.
   */
  closeOnDismissClick?: boolean;
}

export function Modal({
  isOpen,
  onClose,
  title,
  children,
  closeOnEscape = true,
  closeOnDismissClick = true,
}: ModalProps) {
  const contentRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isOpen || !closeOnEscape) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onClose();
      }
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, onClose, closeOnEscape]);

  useEffect(() => {
    if (!isOpen) return;

    const focusableSelector =
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';
    const el = contentRef.current;
    if (el) {
      const firstFocusable = el.querySelector<HTMLElement>(focusableSelector);
      firstFocusable?.focus();
    }
  }, [isOpen]);

  if (!isOpen) return null;

  return createPortal(
    <div
      className="modal-overlay"
      onClick={(e) => {
        if (closeOnDismissClick && e.target === e.currentTarget) {
          onClose();
        }
      }}
      role="dialog"
      aria-modal="true"
      aria-label={title}
    >
      <div className="modal-content" ref={contentRef}>
        {title && (
          <div className="modal-header">
            <h2 className="modal-title">{title}</h2>
            <button
              className="modal-close"
              onClick={() => {
                if (closeOnDismissClick) onClose();
              }}
              aria-label="Close modal"
              aria-disabled={!closeOnDismissClick}
            >
              &times;
            </button>
          </div>
        )}
        {children}
      </div>
    </div>,
    document.body,
  );
}
