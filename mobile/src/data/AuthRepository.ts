import { API_BASE_URL } from "../config";

// Shapes returned by the backend's real auth routes (internal/adapters/http
// on the `backend` branch, commit bbae160) — kept in sync by hand like
// PlacesRepository, there's no shared schema between the two codebases yet.
// Email + password only: the backend has no Apple/Google OAuth wired up
// (no provider/apple_sub/google_sub field on User), so this repository
// doesn't offer those either — building UI for a login method the backend
// can't handle would just be a dead end.
export type AuthUser = {
  id: string;
  email: string;
  role: string;
};

export class AuthApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}

async function authFetch<T>(
  path: string,
  options: { method: string; body?: unknown; token?: string },
): Promise<T | null> {
  const headers: Record<string, string> = {};
  if (options.body !== undefined) headers["Content-Type"] = "application/json";
  if (options.token) headers["Authorization"] = `Bearer ${options.token}`;

  const res = await fetch(`${API_BASE_URL}${path}`, {
    method: options.method,
    headers,
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
  });

  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    body = null;
  }

  if (!res.ok) {
    const message =
      body && typeof body === "object" && "error" in body ? String((body as { error: unknown }).error) : "request failed";
    throw new AuthApiError(message, res.status);
  }

  return body as T | null;
}

export async function register(email: string, password: string): Promise<AuthUser> {
  const user = await authFetch<AuthUser>("/register", { method: "POST", body: { email, password } });
  if (!user) throw new AuthApiError("empty response", 500);
  return user;
}

export async function login(email: string, password: string): Promise<string> {
  const body = await authFetch<{ token: string }>("/login", { method: "POST", body: { email, password } });
  if (!body) throw new AuthApiError("empty response", 500);
  return body.token;
}

export async function logout(token: string): Promise<void> {
  await authFetch<null>("/logout", { method: "POST", token });
}

export async function updateProfile(
  token: string,
  changes: { email?: string; password?: string },
): Promise<AuthUser> {
  const user = await authFetch<AuthUser>("/me", { method: "PATCH", body: changes, token });
  if (!user) throw new AuthApiError("empty response", 500);
  return user;
}

export async function deleteAccount(token: string): Promise<void> {
  await authFetch<null>("/me", { method: "DELETE", token });
}
