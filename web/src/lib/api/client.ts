const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/**
 * Every request includes credentials so the browser attaches/receives the
 * HttpOnly refresh-token cookie. The access token, when present, is sent
 * as a Bearer header rather than a cookie so it is never persisted by
 * the browser.
 */
export async function apiFetch<T>(
  path: string,
  options: { method?: string; body?: unknown; accessToken?: string } = {},
): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, {
    method: options.method ?? "GET",
    credentials: "include",
    headers: {
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      ...(options.accessToken ? { Authorization: `Bearer ${options.accessToken}` } : {}),
    },
    body: options.body ? JSON.stringify(options.body) : undefined,
  });

  if (res.status === 204) {
    return undefined as T;
  }

  const data = await res.json().catch(() => null);

  if (!res.ok) {
    const message = (data && typeof data === "object" && "error" in data ? String(data.error) : null) ?? res.statusText;
    throw new ApiError(res.status, message);
  }

  return data as T;
}
