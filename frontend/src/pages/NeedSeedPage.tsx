import { useMemo } from "react";
import { FilteredTorrentsPage } from "./FilteredTorrentsPage";

export function NeedSeedPage() {
  const extraParams = useMemo(
    () => ({
      max_seeders: "0",
      sort: "created_at",
      order: "desc",
    }),
    [],
  );

  return (
    <FilteredTorrentsPage
      title="Need Seed"
      extraParams={extraParams}
      emptyMessage="All torrents currently have seeders."
    />
  );
}
