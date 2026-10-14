/**
 * useAuth : état de session applicative WashGuard (/api/v1/auth/me).
 *
 * Avec Clerk actif, la session s'ouvre via POST /auth/clerk (pont dans
 * src/lib/clerk.tsx) — la réponse de cet appel est le même payload que
 * /auth/me, et l'événement « washguard:auth-clerk » déclenche un refresh.
 */
import { useQuery, useQueryClient, useMutation } from "@tanstack/react-query";
import { useEffect } from "react";
import { api, ApiError } from "@/lib/api";
import { oublierJetonApercu } from "@/lib/session-preview";
import { surSessionExpiree } from "@/lib/session-signal";
import type { Role } from "@/types";
import type { Me } from "@/types";


export interface LoginInput {
  email: string;
  password: string;
}

export function useAuth() {
  const queryClient = useQueryClient();

  // Clerk : quand le backend a ouvert la session applicative, on recharge /me.
  useEffect(() => {
    const refresh = () =>
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    window.addEventListener("washguard:auth-clerk", refresh);
    return () => window.removeEventListener("washguard:auth-clerk", refresh);
  }, [queryClient]);

  // Session refusée par le serveur (expirée, révoquée, cookie perdu) : la
  // session connue est vidée, la garde de route ramène à la connexion et
  // explique le retour. On garde les données de l'entreprise en cache : elles
  // ne servent à rien sans session, et `queryClient.clear()` au moment de la
  // reconnexion s'en chargeait déjà autrement.
  useEffect(
    () =>
      surSessionExpiree(() => {
        queryClient.setQueryData(["me"], null);
      }),
    [queryClient],
  );

  const meQuery = useQuery<Me | null>({
    queryKey: ["me"],
    queryFn: async () => {
      try {
        return await api.get<Me>("/auth/me");
      } catch (caught) {
        if (caught instanceof ApiError && caught.status === 401) return null;
        throw caught;
      }
    },
    retry: false,
    staleTime: 60_000,
  });

  const login = useMutation({
    mutationFn: (input: LoginInput) =>
      api.post<Me>("/auth/login", input),
    onSuccess: (payload) => {
      queryClient.setQueryData(["me"], payload);
    },
    // Le jeton d'aperçu est retenu par le client API dès la réponse reçue ; une
    // connexion refusée l'a déjà oublié (voir src/lib/api.ts).
  });

  /** Inscription : organisation + station + propriétaire, session ouverte. */
  const registerMutation = useMutation({
    mutationFn: (input: Record<string, unknown>) =>
      api.post<Me>("/auth/register", input),
    onSuccess: (payload) => {
      queryClient.setQueryData(["me"], payload);
    },
  });

  const logout = useMutation({
    mutationFn: () => api.post("/auth/logout"),
    onSuccess: () => {
      queryClient.setQueryData(["me"], null);
      queryClient.clear();
      oublierJetonApercu();
    },
  });

  return {
    me: meQuery.data ?? null,
    loading: meQuery.isLoading,
    error: meQuery.error,
    refetch: meQuery.refetch,
    login: login.mutateAsync,
    loginPending: login.isPending,
    register: registerMutation.mutateAsync,
    registerPending: registerMutation.isPending,
    logout: logout.mutateAsync,
    /** Vérifie une permission serveur ; toujours vrai côté superadmin. */
    can: (permission: string) =>
      (meQuery.data?.permissions ?? []).includes(permission) ||
      meQuery.data?.user.role === "superadmin",
    role: (meQuery.data?.user.role ?? "") as Role | "",
    permissions: meQuery.data?.permissions ?? [],
  };
}
