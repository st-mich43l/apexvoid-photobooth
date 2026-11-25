import { useState } from "react";
import { api, toAppError, type AppError } from "../../../shared/api";
import { useApp } from "../../../app/state";
import type { Settings } from "../../../shared/types";

/** Change one or two settings from any page. Only the named keys are sent (PatchSettings merges them into the
 *  backend's *current* settings), so a page holding an older copy can never roll back what another page changed. */
export function usePatchSettings() {
  const { settings, setSettings, toast } = useApp();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<AppError | null>(null);
  const patch = async (p: Partial<Settings>, message = "Saved"): Promise<boolean> => {
    setBusy(true); setError(null);
    try {
      const r = await api.patchSettings(p);
      setSettings(r.settings ?? { ...settings, ...p });
      toast(message);
      return true;
    } catch (e) { setError(toAppError(e)); return false; } finally { setBusy(false); }
  };
  return { patch, busy, error, clear: () => setError(null) };
}
