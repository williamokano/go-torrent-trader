import createClient from "openapi-fetch";
import type { paths } from "@/api/schema";
import { getConfig } from "@/config";

export const api = createClient<paths>({
  baseUrl: getConfig().API_URL,
});
