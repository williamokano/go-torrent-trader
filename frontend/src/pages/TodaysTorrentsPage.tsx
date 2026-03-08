import { useMemo } from "react";
import { FilteredTorrentsPage } from "./FilteredTorrentsPage";

export function TodaysTorrentsPage() {
  const extraParams = useMemo(() => {
    const since = new Date();
    since.setHours(since.getHours() - 24);
    return {
      created_after: since.toISOString(),
      sort: "created_at",
      order: "desc",
    };
  }, []);

  return (
    <FilteredTorrentsPage
      title="Today's Torrents"
      extraParams={extraParams}
      emptyMessage="No torrents uploaded in the last 24 hours."
    />
  );
}
