import { useState } from "react";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { MetadataFieldsTable } from "@/components/MetadataFieldsTable";
import type { MetadataField } from "@/utils/metadata";

function Harness({ initial = [] }: { initial?: MetadataField[] }) {
  const [value, setValue] = useState<MetadataField[]>(initial);
  return <MetadataFieldsTable value={value} onChange={setValue} />;
}

afterEach(cleanup);

describe("MetadataFieldsTable", () => {
  it("shows an empty message and opens the add-field modal", () => {
    render(<Harness />);
    expect(screen.getByText(/no custom fields/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Add Field" }));

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Add Field", { selector: ".modal-title" }));
  });

  it("adds a field that then appears as a row", () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole("button", { name: "Add Field" }));

    fireEvent.change(screen.getByLabelText("Key"), {
      target: { value: "year" },
    });
    fireEvent.change(screen.getByLabelText("Label"), {
      target: { value: "Year" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save Field" }));

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    const row = screen.getByTestId("field-row");
    expect(row).toHaveTextContent("Year");
    expect(row).toHaveTextContent("year");
    expect(screen.queryByText(/no custom fields/i)).not.toBeInTheDocument();
  });

  it("edits an existing field via the modal", () => {
    render(
      <Harness initial={[{ key: "year", label: "Year", type: "number" }]} />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    const labelInput = screen.getByLabelText("Label");
    expect(labelInput).toHaveValue("Year");
    fireEvent.change(labelInput, { target: { value: "Release Year" } });
    fireEvent.click(screen.getByRole("button", { name: "Save Field" }));

    expect(screen.getByTestId("field-row")).toHaveTextContent("Release Year");
  });

  it("removes a field", () => {
    render(
      <Harness initial={[{ key: "year", label: "Year", type: "number" }]} />,
    );
    expect(screen.getByTestId("field-row")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Remove" }));

    expect(screen.queryByTestId("field-row")).not.toBeInTheDocument();
    expect(screen.getByText(/no custom fields/i)).toBeInTheDocument();
  });
});
