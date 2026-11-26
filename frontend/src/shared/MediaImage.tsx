import { useEffect, useRef, useState } from "react";
import { translate } from "./i18n";
import { useLocale } from "./locale";

type State = "loading" | "ready" | "retrying" | "failed";

/** Saved media with bounded recovery. Retry URLs are deterministic for a given attempt and never include URLs in diagnostics. */
export function MediaImage({ src, alt, className = "", diagnosticId, onStatus, revision, onDecoded, manualRetry = true }: {
  src: string; alt: string; className?: string; diagnosticId?: string; revision?: string;
  onStatus?: (ready: boolean) => void; onDecoded?: (width: number, height: number) => void; manualRetry?: boolean;
}) {
  const language = useLocale();
  const [attempt, setAttempt] = useState(0);
  const [automaticFailures, setAutomaticFailures] = useState(0);
  const [state, setState] = useState<State>("loading");
  const timer = useRef<number | undefined>(undefined);
  const loadTimer = useRef<number | undefined>(undefined);
  const status = useRef(onStatus); status.current = onStatus;
  useEffect(() => {
    window.clearTimeout(timer.current); window.clearTimeout(loadTimer.current);
    setAttempt(0); setAutomaticFailures(0); setState("loading");
    return () => { window.clearTimeout(timer.current); window.clearTimeout(loadTimer.current); };
  }, [src, revision]);
  const fail = (reason: "image_error" | "load_timeout" | "decode_dimensions") => {
    window.clearTimeout(loadTimer.current);
    if (automaticFailures < 2) {
      setAutomaticFailures((n) => n + 1);
      setState("retrying");
      timer.current = window.setTimeout(() => { setState("loading"); setAttempt((n) => n + 1); }, automaticFailures === 0 ? 500 : 1400);
      return;
    }
    setState("failed"); status.current?.(false);
    // The identifier is a non-sensitive media ID; deliberately never log the URL/token or filesystem path.
    console.warn("saved media image failed", { mediaId: diagnosticId ?? "unknown", reason, attempt: attempt + 1 });
  };
  const retry = () => { window.clearTimeout(timer.current); setAutomaticFailures(0); setState("loading"); setAttempt((n) => n + 1); };
  const retrySrc = attempt === 0 ? src : `${src}${src.includes("?") ? "&" : "?"}retry=${attempt}`;
  useEffect(() => {
    if (state !== "loading") return;
    loadTimer.current = window.setTimeout(() => fail("load_timeout"), 10000);
    return () => window.clearTimeout(loadTimer.current);
  }, [retrySrc, state]);
  return <span className={`media-image ${className}`} data-state={state}>
    <img src={retrySrc} alt={alt} decoding="async" draggable={false}
      onLoad={(event) => {
        const image = event.currentTarget;
        if (!image.naturalWidth || !image.naturalHeight) { fail("decode_dimensions"); return; }
        window.clearTimeout(timer.current); window.clearTimeout(loadTimer.current); setState("ready"); status.current?.(true); onDecoded?.(image.naturalWidth, image.naturalHeight);
      }}
      onError={() => fail("image_error")} />
    {state !== "ready" && <span className="media-image-message" role={state === "failed" ? "alert" : "status"}>
      {state === "loading" ? translate(language, "loadingPhoto") : state === "retrying" ? translate(language, "preparingPhotos") : <>
        <span>{translate(language, "couldNotLoadPhoto")}</span>
        {manualRetry && <button type="button" onClick={retry}>{translate(language, "retry")}</button>}
      </>}
    </span>}
  </span>;
}
