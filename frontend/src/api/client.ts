import createClient from "openapi-fetch";
import type { paths } from "@/api/schema";

const API_BASE = import.meta.env.VITE_API_URL || "http://localhost:8080";

export const api = createClient<paths>({
  baseUrl: API_BASE,
});
