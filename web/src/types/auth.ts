export interface User {
  user_id: string;
  email: string;
}

export interface AuthResponse {
  access_token: string;
  access_token_expires_at: string;
}

export type AuthStatus = "loading" | "authenticated" | "unauthenticated";
