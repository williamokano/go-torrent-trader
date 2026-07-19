import { useState } from "react";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MetadataSchemaEditor } from "@/components/MetadataSchemaEditor";
import type { MetadataField } from "@/utils/metadata";

function Harness({ spy }: { spy?: (f: MetadataField[]) => void }) {
  const [value, setValue] = useState<MetadataField[]>([]);
  return (
    <MetadataSchemaEditor
      value={value}
      onChange={(f) => {
        setValue(f);
        spy?.(f);
      }}
    />
  );
}

describe("MetadataSchemaEditor", () => {
  afterEach(cleanup);

  it("shows an empty message and adds a field", () => {
    render(<Harness />);
    expect(screen.getByText(/no custom fields/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Add Field" }));

    expect(screen.getByTestId("schema-row")).toBeInTheDocument();
    expect(screen.queryByText(/no custom fields/i)).not.toBeInTheDocument();
  });

  it("edits a field and reveals options after switching to select", () => {
    const spy = vi.fn();
    render(<Harness spy={spy} />);
    fireEvent.click(screen.getByRole("button", { name: "Add Field" }));

    fireEvent.change(screen.getByLabelText("Key"), {
      target: { value: "codec" },
    });
    fireEvent.change(screen.getByLabelText("Label"), {
      target: { value: "Codec" },
    });
    fireEvent.change(screen.getByLabelText("Type"), {
      target: { value: "select" },
    });

    const optionsInput = screen.getByLabelText("Options (comma separated)");
    fireEvent.change(optionsInput, { target: { value: "x264, x265" } });

    expect(spy).toHaveBeenLastCalledWith([
      {
        key: "codec",
        label: "Codec",
        type: "select",
        options: ["x264", "x265"],
      },
    ]);
  });

  it("captures number constraints", () => {
    const spy = vi.fn();
    render(<Harness spy={spy} />);
    fireEvent.click(screen.getByRole("button", { name: "Add Field" }));
    fireEvent.change(screen.getByLabelText("Key"), {
      target: { value: "year" },
    });
    fireEvent.change(screen.getByLabelText("Type"), {
      target: { value: "number" },
    });
    fireEvent.change(screen.getByLabelText("Min"), {
      target: { value: "1900" },
    });
    fireEvent.click(screen.getByLabelText("Whole numbers only"));

    const last = spy.mock.calls[spy.mock.calls.length - 1][0][0];
    expect(last.type).toBe("number");
    expect(last.min).toBe(1900);
    expect(last.integer).toBe(true);
  });

  it("removes a field", () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole("button", { name: "Add Field" }));
    expect(screen.getByTestId("schema-row")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    expect(screen.queryByTestId("schema-row")).not.toBeInTheDocument();
  });
});
