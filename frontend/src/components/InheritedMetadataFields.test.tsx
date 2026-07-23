import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { InheritedMetadataFields } from "@/components/InheritedMetadataFields";
import type { MetadataField } from "@/utils/metadata";

afterEach(cleanup);

describe("InheritedMetadataFields", () => {
  it("renders nothing when there are no inherited fields", () => {
    const { container } = render(<InheritedMetadataFields fields={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders inherited fields read-only with their details", () => {
    const fields: MetadataField[] = [
      { key: "year", label: "Year", type: "number", min: 1900, required: true },
      {
        key: "codec",
        label: "Codec",
        type: "select",
        options: ["x264", "x265"],
      },
    ];
    render(<InheritedMetadataFields fields={fields} />);

    expect(screen.getByTestId("inherited-fields")).toBeInTheDocument();
    expect(screen.getAllByTestId("inherited-field-row")).toHaveLength(2);
    expect(screen.getByText("Year")).toBeInTheDocument();
    expect(screen.getByText("x264, x265")).toBeInTheDocument();
    // Read-only: no edit/remove controls.
    expect(
      screen.queryByRole("button", { name: /edit|remove/i }),
    ).not.toBeInTheDocument();
  });
});
