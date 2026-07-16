import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import { ByteSizeInput } from "@/components/form/ByteSizeInput";

afterEach(cleanup);

describe("ByteSizeInput", () => {
  it("shows the value in the best-fit unit", () => {
    render(
      <ByteSizeInput label="Uploaded" value={5368709120} onChange={() => {}} />,
    );
    // 5 GiB exactly
    expect(screen.getByLabelText("Uploaded")).toHaveValue(5);
    expect(screen.getByLabelText("Uploaded unit")).toHaveValue("3");
  });

  it("keeps raw bytes for values under 1 KB", () => {
    render(<ByteSizeInput label="Uploaded" value={512} onChange={() => {}} />);
    expect(screen.getByLabelText("Uploaded")).toHaveValue(512);
    expect(screen.getByLabelText("Uploaded unit")).toHaveValue("0");
  });

  it("emits bytes when the amount is typed", () => {
    const onChange = vi.fn();
    render(<ByteSizeInput label="Uploaded" value={0} onChange={onChange} />);
    const unit = screen.getByLabelText("Uploaded unit");
    fireEvent.change(unit, { target: { value: "4" } }); // TB
    fireEvent.change(screen.getByLabelText("Uploaded"), {
      target: { value: "2" },
    });
    expect(onChange).toHaveBeenLastCalledWith(2 * 1024 ** 4);
  });

  it("converts the displayed amount when the unit changes without emitting", () => {
    const onChange = vi.fn();
    render(
      <ByteSizeInput
        label="Downloaded"
        value={1073741824}
        onChange={onChange}
      />,
    );
    // 1 GiB shown as GB; switch to MB → 1024
    fireEvent.change(screen.getByLabelText("Downloaded unit"), {
      target: { value: "2" },
    });
    expect(screen.getByLabelText("Downloaded")).toHaveValue(1024);
    expect(onChange).not.toHaveBeenCalled();
  });

  it("shows the exact byte count as a hint for non-byte units", () => {
    render(
      <ByteSizeInput
        label="Uploaded"
        value={1099511627776}
        onChange={() => {}}
      />,
    );
    expect(
      screen.getByText(`= ${(1099511627776).toLocaleString()} bytes`),
    ).toBeInTheDocument();
  });

  it("keeps the stored value while the field is emptied and restores it on blur", () => {
    const onChange = vi.fn();
    render(<ByteSizeInput label="Uploaded" value={1024} onChange={onChange} />);
    const input = screen.getByLabelText("Uploaded");
    fireEvent.change(input, { target: { value: "" } });
    // A transient empty field must not zero the stored bytes.
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.blur(input);
    expect(input).toHaveValue(1);
  });

  it("zeroes only on an explicit 0", () => {
    const onChange = vi.fn();
    render(<ByteSizeInput label="Uploaded" value={1024} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText("Uploaded"), {
      target: { value: "0" },
    });
    expect(onChange).toHaveBeenLastCalledWith(0);
  });

  it("clamps amounts beyond the safe integer range", () => {
    const onChange = vi.fn();
    render(<ByteSizeInput label="Uploaded" value={0} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText("Uploaded unit"), {
      target: { value: "4" },
    });
    fireEvent.change(screen.getByLabelText("Uploaded"), {
      target: { value: "99999999" },
    });
    expect(onChange).toHaveBeenLastCalledWith(Number.MAX_SAFE_INTEGER);
  });

  it("re-derives the display when the parent value changes", () => {
    function Harness() {
      const [bytes, setBytes] = useState(1024);
      return (
        <>
          <ByteSizeInput label="Uploaded" value={bytes} onChange={() => {}} />
          <button onClick={() => setBytes(2 * 1024 ** 3)}>reset</button>
        </>
      );
    }
    render(<Harness />);
    expect(screen.getByLabelText("Uploaded")).toHaveValue(1);
    fireEvent.click(screen.getByText("reset"));
    expect(screen.getByLabelText("Uploaded")).toHaveValue(2);
    expect(screen.getByLabelText("Uploaded unit")).toHaveValue("3");
  });
});
