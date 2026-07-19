import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MetadataFields } from "@/components/MetadataFields";
import type { MetadataField } from "@/utils/metadata";

const schema: MetadataField[] = [
  { key: "year", label: "Year", type: "number" },
  { key: "codec", label: "Codec", type: "select", options: ["x264", "x265"] },
  {
    key: "audio",
    label: "Audio",
    type: "multiselect",
    options: ["FLAC", "AC3"],
  },
  { key: "hdr", label: "HDR", type: "boolean" },
  { key: "note", label: "Note", type: "text", required: true },
];

describe("MetadataFields", () => {
  afterEach(cleanup);

  it("renders nothing for an empty schema", () => {
    const { container } = render(
      <MetadataFields schema={[]} values={{}} onChange={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders a control per field and marks required fields", () => {
    render(<MetadataFields schema={schema} values={{}} onChange={vi.fn()} />);
    expect(screen.getByLabelText("Year")).toBeInTheDocument();
    expect(screen.getByLabelText("Codec")).toBeInTheDocument();
    expect(screen.getByLabelText("HDR")).toBeInTheDocument();
    // required field gets a trailing asterisk
    expect(screen.getByLabelText("Note *")).toBeInTheDocument();
  });

  it("emits updated values when a text/number field changes", () => {
    const onChange = vi.fn();
    render(<MetadataFields schema={schema} values={{}} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText("Year"), {
      target: { value: "2024" },
    });
    expect(onChange).toHaveBeenCalledWith({ year: "2024" });
  });

  it("emits the selected option for a select field", () => {
    const onChange = vi.fn();
    render(<MetadataFields schema={schema} values={{}} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText("Codec"), {
      target: { value: "x265" },
    });
    expect(onChange).toHaveBeenCalledWith({ codec: "x265" });
  });

  it("toggles multiselect options on and off", () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <MetadataFields schema={schema} values={{}} onChange={onChange} />,
    );
    fireEvent.click(screen.getByLabelText("FLAC"));
    expect(onChange).toHaveBeenLastCalledWith({ audio: ["FLAC"] });

    rerender(
      <MetadataFields
        schema={schema}
        values={{ audio: ["FLAC"] }}
        onChange={onChange}
      />,
    );
    fireEvent.click(screen.getByLabelText("FLAC"));
    expect(onChange).toHaveBeenLastCalledWith({ audio: [] });
  });

  it("emits boolean changes", () => {
    const onChange = vi.fn();
    render(<MetadataFields schema={schema} values={{}} onChange={onChange} />);
    fireEvent.click(screen.getByLabelText("HDR"));
    expect(onChange).toHaveBeenCalledWith({ hdr: true });
  });
});
