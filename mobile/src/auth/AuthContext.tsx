import React, { createContext, useContext, useEffect, useMemo, useState } from "react";
import * as SecureStore from "expo-secure-store";
import * as Auth from "../data/AuthRepository";
import type { AuthUser } from "../data/AuthRepository";

const TOKEN_KEY = "auth.token";
const USER_KEY = "auth.user";

type AuthContextValue = {
  user: AuthUser | null;
  isLoggedIn: boolean;
  isLoading: boolean;
  register: (email: string, password: string) => Promise<void>;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  updateProfile: (changes: { email?: string; password?: string }) => Promise<void>;
  deleteAccount: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

async function persistSession(token: string, user: AuthUser): Promise<void> {
  await SecureStore.setItemAsync(TOKEN_KEY, token);
  await SecureStore.setItemAsync(USER_KEY, JSON.stringify(user));
}

async function clearPersistedSession(): Promise<void> {
  await SecureStore.deleteItemAsync(TOKEN_KEY);
  await SecureStore.deleteItemAsync(USER_KEY);
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [token, setToken] = useState<string | null>(null);
  const [user, setUser] = useState<AuthUser | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  // Read whatever session was last stored, without a network round-trip —
  // a JWT is stateless (24h TTL, internal/adapters/jwt/issuer.go), so there
  // is nothing to "verify" that works offline anyway. This app must open
  // straight to a plausible logged-in state with no connection at all;
  // an expired/invalid token is only discovered reactively, the next time
  // an authenticated call (updateProfile/deleteAccount below) gets a 401 —
  // at that point the session is cleared and the UI falls back to logged out.
  useEffect(() => {
    (async () => {
      const [storedToken, storedUser] = await Promise.all([
        SecureStore.getItemAsync(TOKEN_KEY),
        SecureStore.getItemAsync(USER_KEY),
      ]);
      if (storedToken && storedUser) {
        setToken(storedToken);
        try {
          setUser(JSON.parse(storedUser) as AuthUser);
        } catch {
          // Stored value was corrupt — treat as logged out rather than crash.
          await clearPersistedSession();
        }
      }
      setIsLoading(false);
    })();
  }, []);

  async function handleAuthError(err: unknown): Promise<never> {
    if (err instanceof Auth.AuthApiError && err.status === 401) {
      await clearPersistedSession();
      setToken(null);
      setUser(null);
    }
    throw err;
  }

  async function registerFn(email: string, password: string) {
    const newUser = await Auth.register(email, password);
    const newToken = await Auth.login(email, password);
    await persistSession(newToken, newUser);
    setToken(newToken);
    setUser(newUser);
  }

  async function loginFn(email: string, password: string) {
    const newToken = await Auth.login(email, password);
    // /login only returns a token, not the profile — /me isn't a GET route
    // today (only PATCH/DELETE), so the profile shown right after login is
    // built from what the user just typed rather than a server round-trip.
    const loggedInUser: AuthUser = { id: "", email, role: "user" };
    await persistSession(newToken, loggedInUser);
    setToken(newToken);
    setUser(loggedInUser);
  }

  async function logoutFn() {
    if (token) {
      try {
        await Auth.logout(token);
      } catch {
        // Stateless JWT — server-side logout has nothing to invalidate
        // anyway (see internal/adapters/http/user_handler.go's own comment
        // on this route). Clear the local session regardless of the result.
      }
    }
    await clearPersistedSession();
    setToken(null);
    setUser(null);
  }

  async function updateProfileFn(changes: { email?: string; password?: string }) {
    if (!token) throw new Error("not logged in");
    try {
      const updated = await Auth.updateProfile(token, changes);
      const merged = { ...updated };
      await persistSession(token, merged);
      setUser(merged);
    } catch (err) {
      await handleAuthError(err);
    }
  }

  async function deleteAccountFn() {
    if (!token) throw new Error("not logged in");
    try {
      await Auth.deleteAccount(token);
      await clearPersistedSession();
      setToken(null);
      setUser(null);
    } catch (err) {
      await handleAuthError(err);
    }
  }

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      isLoggedIn: user !== null,
      isLoading,
      register: registerFn,
      login: loginFn,
      logout: logoutFn,
      updateProfile: updateProfileFn,
      deleteAccount: deleteAccountFn,
    }),
    [user, token, isLoading],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return ctx;
}
