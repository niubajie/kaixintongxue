import { create } from "zustand";
import { tokenStore } from "../api/client";
import { bridge } from "../platform";

const ROLE_KEY = "circle_admin_role";
const NAME_KEY = "circle_admin_name";

interface AuthState {
  token: string | null;
  role: string | null;
  username: string | null;
  login: (token: string, role: string, username: string) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  token: tokenStore.get(),
  role: bridge.secureStorage.get(ROLE_KEY),
  username: bridge.secureStorage.get(NAME_KEY),
  login: (token, role, username) => {
    tokenStore.set(token);
    bridge.secureStorage.set(ROLE_KEY, role);
    bridge.secureStorage.set(NAME_KEY, username);
    set({ token, role, username });
  },
  logout: () => {
    tokenStore.clear();
    bridge.secureStorage.remove(ROLE_KEY);
    bridge.secureStorage.remove(NAME_KEY);
    set({ token: null, role: null, username: null });
  }
}));

export function hasRole(...allowed: string[]): boolean {
  const role = useAuthStore.getState().role;
  if (!role) return false;
  if (role === "super_admin") return true;
  return allowed.includes(role);
}
