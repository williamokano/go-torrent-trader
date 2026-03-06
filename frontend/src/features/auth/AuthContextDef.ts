import { createContext } from "react";

export interface User {
  id: number;
  username: string;
  email: string;
  groupId: number;
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
}

export const AuthContext = createContext<AuthContextValue | null>(null);
