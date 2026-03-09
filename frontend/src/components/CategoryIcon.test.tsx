import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, test } from "vitest";
import { CategoryIcon } from "@/components/CategoryIcon";

afterEach(cleanup);

describe("CategoryIcon", () => {
  test("renders placeholder with first letter when no imageUrl", () => {
    render(<CategoryIcon name="Movies" />);
    expect(screen.getByText("M")).toBeInTheDocument();
  });

  test("renders image when imageUrl is provided", () => {
    render(
      <CategoryIcon name="Games" imageUrl="https://example.com/icon.png" />,
    );
    const img = screen.getByRole("img", { name: "Games" });
    expect(img).toBeInTheDocument();
    expect(img).toHaveAttribute("src", "https://example.com/icon.png");
    expect(img).toHaveAttribute("loading", "lazy");
  });

  test("renders placeholder when imageUrl is null", () => {
    render(<CategoryIcon name="TV Shows" imageUrl={null} />);
    expect(screen.getByText("T")).toBeInTheDocument();
  });

  test("falls back to placeholder when image fails to load", () => {
    render(
      <CategoryIcon name="Broken" imageUrl="https://example.com/broken.png" />,
    );
    const img = screen.getByRole("img", { name: "Broken" });
    fireEvent.error(img);
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.getByText("B")).toBeInTheDocument();
  });

  test("applies size class", () => {
    const { container } = render(<CategoryIcon name="Music" size="lg" />);
    const el = container.querySelector(".category-icon--lg");
    expect(el).toBeInTheDocument();
  });

  test("applies custom className", () => {
    const { container } = render(
      <CategoryIcon name="Software" className="my-custom" />,
    );
    const el = container.querySelector(".my-custom");
    expect(el).toBeInTheDocument();
  });
});
