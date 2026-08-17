import { apiFetch } from "./client";
import type { AuthResponse, User } from "@/types/auth";

export function register(email: string, password: string) {
  return apiFetch<AuthResponse>("/api/auth/register", {
    method: "POST",
    body: { email, password },
  });
}

export function login(email: string, password: string) {
  return apiFetch<AuthResponse>("/api/auth/login", {
    method: "POST",
    body: { email, password },
  });
}

export function loginWithGoogle(idToken: string) {
  return apiFetch<AuthResponse>("/api/auth/google", {
    method: "POST",
    body: { id_token: idToken },
  });
}

/** Relies solely on the HttpOnly refresh cookie; no body needed. */
export function refresh() {
  return apiFetch<AuthResponse>("/api/auth/refresh", { method: "POST" });
}

export function logout() {
  return apiFetch<void>("/api/auth/logout", { method: "POST" });
}

export function me(accessToken: string) {
  return apiFetch<User>("/api/auth/me", { accessToken });
}
