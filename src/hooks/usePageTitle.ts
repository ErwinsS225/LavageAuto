/**
 * Titre d'onglet : « Page · WashGuard ».
 */
import { useEffect } from "react";

export function usePageTitle(title: string) {
  useEffect(() => {
    document.title = title ? `${title} · WashGuard` : "WashGuard";
  }, [title]);
}
