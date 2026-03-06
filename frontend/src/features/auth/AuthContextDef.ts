import { createContext } from "react";

export interface User {
  id: number;
  username: string;
  email: string;
  group_id: number;
  avatar: string;
  title: string;
  info: string;
  uploaded: number;
  downloaded: number;
  ratio: number;
  passkey: string;
  invites: number;
  warned: boolean;
  donor: boolean;
  enabled: boolean;
  created_at: string;
  last_login: string;
  isAdmin: boolean;
}

export interface RegisterData {
  username: string;
  email: string;
  password: string;
}

export interface AuthContextValue {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  register: (data: RegisterData) => Promise<void>;
  refreshUser: () => Promise<void>;
}

export const AuthContext = createContext<AuthContextValue | null>(null);
