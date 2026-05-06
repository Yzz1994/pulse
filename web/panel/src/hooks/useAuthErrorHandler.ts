import { useCallback } from "react";
import { useNavigate } from "@tanstack/react-router";
import { AuthError } from "@/lib/api";
import { clearToken } from "@/lib/auth";

export function useAuthErrorHandler(): (err: unknown) => boolean {
  const navigate = useNavigate();
  return useCallback(
    (err: unknown): boolean => {
      if (err instanceof AuthError) {
        clearToken();
        navigate({ to: "/panel/login" });
        return true;
      }
      return false;
    },
    [navigate],
  );
}
