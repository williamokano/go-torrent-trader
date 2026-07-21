import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MetadataFieldModal } from "@/components/MetadataFieldModal";
import type { MetadataField } from "@/utils/metadata";

afterEach(cleanup);

function renderModal(
  overrides: Partial<React.ComponentProps<typeof MetadataFieldModal>> = {},
) {
  const onSave = vi.fn();
  const onClose = vi.fn();
  render(
    <MetadataFieldModal
      initial={null}
      existingKeys={[]}
      onSave={onSave}
      onClose={onClose}
      {...overrides}
    />,
  );
  return { onSave, onClose };
}

describe("MetadataFieldModal", () => {
  it("adds a select field with parsed options", () => {
    const { onSave } = renderModal();

    fireEvent.change(screen.getByLabelText("Key"), {
      target: { value: "codec" },
    });
    fireEvent.change(screen.getByLabelText("Label"), {
      target: { value: "Codec" },
    });
    fireEvent.change(screen.getByLabelText("Type"), {
      target: { value: "select" },
    });
    fireEvent.change(screen.getByLabelText("Options (comma separated)"), {
      target: { value: "x264, x265" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Save Field" }));

    expect(onSave).toHaveBeenCalledWith({
      key: "codec",
      label: "Codec",
      type: "select",
      options: ["x264", "x265"],
    });
  });

  it("rejects an invalid key", () => {
    const { onSave } = renderModal();
    fireEvent.change(screen.getByLabelText("Key"), {
      target: { value: "Bad Key" },
    });
    fireEvent.change(screen.getByLabelText("Label"), {
      target: { value: "Label" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Save Field" }));

    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByText(/key must start with/i)).toBeInTheDocument();
  });

  it("rejects a duplicate key", () => {
    const { onSave } = renderModal({ existingKeys: ["year"] });
    fireEvent.change(screen.getByLabelText("Key"), {
      target: { value: "year" },
    });
    fireEvent.change(screen.getByLabelText("Label"), {
      target: { value: "Year" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Save Field" }));

    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByText(/already exists/i)).toBeInTheDocument();
  });

  it("requires at least one option for a select", () => {
    const { onSave } = renderModal();
    fireEvent.change(screen.getByLabelText("Key"), {
      target: { value: "codec" },
    });
    fireEvent.change(screen.getByLabelText("Label"), {
      target: { value: "Codec" },
    });
    fireEvent.change(screen.getByLabelText("Type"), {
      target: { value: "select" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Save Field" }));

    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByText(/add at least one option/i)).toBeInTheDocument();
  });

  it("rejects a number field whose min exceeds max", () => {
    const { onSave } = renderModal();
    fireEvent.change(screen.getByLabelText("Key"), {
      target: { value: "year" },
    });
    fireEvent.change(screen.getByLabelText("Label"), {
      target: { value: "Year" },
    });
    fireEvent.change(screen.getByLabelText("Type"), {
      target: { value: "number" },
    });
    fireEvent.change(screen.getByLabelText("Min"), { target: { value: "10" } });
    fireEvent.change(screen.getByLabelText("Max"), { target: { value: "5" } });

    fireEvent.click(screen.getByRole("button", { name: "Save Field" }));

    expect(onSave).not.toHaveBeenCalled();
    expect(
      screen.getByText(/min must be less than or equal/i),
    ).toBeInTheDocument();
  });

  it("pre-fills from an existing field and saves number constraints", () => {
    const initial: MetadataField = {
      key: "year",
      label: "Year",
      type: "number",
      min: 1900,
      integer: true,
    };
    const { onSave } = renderModal({ initial, existingKeys: [] });

    // Pre-filled values are present.
    expect(screen.getByLabelText("Key")).toHaveValue("year");
    expect(screen.getByLabelText("Min")).toHaveValue(1900);

    fireEvent.change(screen.getByLabelText("Max"), {
      target: { value: "2030" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save Field" }));

    expect(onSave).toHaveBeenCalledWith({
      key: "year",
      label: "Year",
      type: "number",
      min: 1900,
      max: 2030,
      integer: true,
    });
  });
});
