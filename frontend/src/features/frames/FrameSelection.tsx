import { formatTranslation, photoCount, translate } from "../../shared/i18n";
import { useCallback, useEffect, useMemo, useState } from "react";
import { api, toAppError } from "../../shared/api";
import { choiceOf, explainFit, filterFrames, fitFor, categoryLabel, frameDisplayName, variantDisplayName, layoutLabel, photoCounts, swatch } from "../../shared/frameInfo";
import { GuestFrames } from "../booth/guest/GuestFrames";
import { LoadingState, GuestMessage, SecondaryAction } from "../booth/guest/parts";
import { useLocale } from "../../shared/locale";
import { useApp } from "../../app/state";
import type { Frame, FrameChoice } from "../../shared/types";
import { useOrientation } from "../../shared/useOrientation";

export interface FrameDecision { useFirst: boolean; restart: boolean }

interface Props {
  /** "booth": large guest UI. "studio": the same screen inside the operator pages. */
  variant: "booth" | "studio";
  initial?: { id: string; variantId: string; caption?: string };
  /** After capture: how many photos exist. Frames for another number are explained, never silently swapped in. */
  takenPhotos?: number;
  title?: string;
  confirmLabel?: string;
  backLabel?: string;
  busy?: boolean;
  onConfirm: (c: FrameChoice, d: FrameDecision) => void;
  onBack?: () => void;
}

/** Frame selection: browse, preview, pick a colour, confirm. Used before the camera starts. */
export function FrameSelection({ variant, initial, takenPhotos, title, confirmLabel, backLabel, busy, onConfirm, onBack }: Props) {
  const { settings, frameUrl } = useApp();
  const language = useLocale(settings.language);
  const t = (key: Parameters<typeof translate>[1]) => translate(language, key);
  const [frames, setFrames] = useState<Frame[] | null>(null);
  const [categories, setCategories] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [category, setCategory] = useState("All");
  const [count, setCount] = useState(0);
  const [pickId, setPickId] = useState(initial?.id ?? "");
  const [variantId, setVariantId] = useState(initial?.variantId ?? "");
  const [caption, setCaption] = useState(initial?.caption ?? "");

  const load = useCallback(() => {
    setError(""); setFrames(null);
    api.listFrames(false).then((r) => {
      setFrames(r.frames); setCategories(r.categories);
      setPickId((cur) => {
        if (cur && r.frames.some((f) => f.id === cur)) return cur;
        const pre = r.frames.find((f) => f.id === (settings.templateId || r.default)) ?? r.frames.find((f) => takenPhotos ? f.photos === takenPhotos : true);
        return pre?.id ?? "";
      });
    }).catch((e) => setError(toAppError(e).message));
  }, [settings.templateId, takenPhotos]);
  useEffect(load, [load]);

  const shown = useMemo(() => filterFrames(frames ?? [], { category, photos: count }), [frames, category, count]);
  const picked = frames?.find((f) => f.id === pickId);
  const fit = picked ? fitFor(picked, takenPhotos) : { kind: "ok" as const };
  const booth = variant === "booth";
  const portrait = useOrientation() === "portrait";

  const select = (f: Frame) => {
    if (f.id !== pickId) { setPickId(f.id); setVariantId(""); }
  };
  const confirm = (d: Partial<FrameDecision>) => picked && onConfirm(choiceOf(picked, variantId, caption), { useFirst: false, restart: false, ...d });

  if (booth) {
    return (
      <div className="g-screen" data-testid="frame-selection">
        {error && (
          <GuestMessage kind="err" action={<button type="button" className="g-btn g-secondary" onClick={load}>{translate(language, "tryAgain")}</button>}>
            <strong>{translate(language, "frameError")}</strong>
          </GuestMessage>
        )}
        {!error && !frames && <LoadingState>{translate(language, "loadingFrames")}</LoadingState>}
        {frames && frames.length === 0 && <GuestMessage kind="warn">{translate(language, "noFrames")}</GuestMessage>}
        {!(frames && frames.length > 0) && (
          <div className="g-actions">
            {onBack && <SecondaryAction icon="back" className="g-back" disabled={busy} onClick={onBack}>{backLabel ?? translate(language, "back")}</SecondaryAction>}
          </div>
        )}
        {frames && frames.length > 0 && (
          <GuestFrames language={language} frames={frames} categories={categories} shown={shown} category={category} setCategory={setCategory} count={count} setCount={setCount}
            picked={picked} pickId={pickId} select={select} variantId={variantId} setVariantId={setVariantId} caption={caption} setCaption={setCaption}
            fit={fit} takenPhotos={takenPhotos} busy={busy} title={title ?? translate(language, "framesTitle")} confirmLabel={confirmLabel} backLabel={backLabel} onBack={onBack}
            confirm={confirm} portrait={portrait}
            thumbUrl={(f) => frameUrl(f.id, "", 420)}
            bigUrl={(f) => frameUrl(f.id, variantId, portrait ? 320 : 720, f.captionEnabled ? caption.trim() : "")} />
        )}
      </div>
    );
  }

  return (
    <div className={`frames ${booth ? "booth" : "studio"}`} data-testid="frame-selection">
      <header className="frames-head">
        {onBack && <button className="back" onClick={onBack} disabled={busy}>← {backLabel ?? t("back")}</button>}
        <h1>{title ?? t("framesTitle")}</h1>
      </header>

      {error && (
        <div className="note err" role="alert"><div><strong>{t("frameError")}</strong><div className="action">{error}</div></div><button onClick={load}>{t("tryAgain")}</button></div>
      )}
      {!error && !frames && <div className="center-msg" role="status"><div className="spinner" />{t("loadingFrames")}</div>}
      {frames && frames.length === 0 && (
        <div className="note warn" role="status"><div>{t("noFramesStudio")}</div></div>
      )}

      {frames && frames.length > 0 && (
        <div className="frames-body">
          <div className="frames-list">
            <div className="filters" role="group" aria-label={t("frameCategories")}>
              {["All", ...categories].map((c) => (
                <button key={c} className={`chip ${category === c ? "on" : ""}`} aria-pressed={category === c} onClick={() => setCategory(c)}>{c === "All" ? t("all") : categoryLabel(c, language)}</button>
              ))}
            </div>
            {photoCounts(frames).length > 1 && (
              <div className="filters" role="group" aria-label={t("numberPhotos")}>
                <button className={`chip ${count === 0 ? "on" : ""}`} aria-pressed={count === 0} onClick={() => setCount(0)}>{t("anyPhotos")}</button>
                {photoCounts(frames).map((n) => (
                  <button key={n} className={`chip ${count === n ? "on" : ""}`} aria-pressed={count === n} onClick={() => setCount(n)}>{photoCount(language, n)}</button>
                ))}
              </div>
            )}
            {shown.length === 0 && <p className="muted">{t("noFramesMatch")}</p>}
            <div className="frame-grid" role="radiogroup" aria-label={t("frames")}>
              {shown.map((f) => {
                const mis = fitFor(f, takenPhotos).kind !== "ok";
                return (
                  <button key={f.id} role="radio" aria-checked={f.id === pickId} className={`frame-card ${f.id === pickId ? "on" : ""} ${mis ? "mismatch" : ""}`} onClick={() => select(f)}>
                    <img src={frameUrl(f.id, "", 360)} alt={formatTranslation(language, "frameAlt", { name: frameDisplayName(f, language) })} loading="lazy" />
                    <span className="fname">{frameDisplayName(f, language)}</span>
                    <span className="fmeta">{layoutLabel(f.format, language)} · {photoCount(language, f.photos)}</span>
                    {mis && <span className="fwarn">{formatTranslation(language, "needsPhotos", { count: f.photos })}</span>}
                  </button>
                );
              })}
            </div>
          </div>

          <aside className="frame-detail" aria-label={t("selectedFrame")}>
            {picked ? (
              <>
                <div className="big-preview"><img src={frameUrl(picked.id, variantId, 640, picked.captionEnabled ? caption.trim() : "")} alt={formatTranslation(language, "framePreview", { name: frameDisplayName(picked, language) })} /></div>
                <h2>{frameDisplayName(picked, language)}</h2>
                <p className="muted">{layoutLabel(picked.format, language)} · {photoCount(language, picked.photos)} · {picked.pixelW}×{picked.pixelH} px</p>
                {picked.variants && picked.variants.length > 1 && (
                  <div className="swatches" role="radiogroup" aria-label={t("chooseColor")}>
                    {picked.variants.map((v, i) => {
                      const on = variantId === v.id || (!variantId && i === 0);
                      return (
                        <button key={v.id} role="radio" aria-checked={on} aria-label={variantDisplayName(picked, v, language)} title={variantDisplayName(picked, v, language)} className={`swatch ${on ? "on" : ""}`}
                          style={{ background: i === 0 ? picked.background : swatch(v, picked) }} onClick={() => setVariantId(i === 0 ? "" : v.id)} />
                      );
                    })}
                  </div>
                )}
                {picked.captionEnabled && (
                  <label className="caption-line">{t("caption")}
                    <input value={caption} maxLength={40} placeholder="e.g. Mai & Nam" onChange={(e) => setCaption(e.target.value)} aria-label={t("captionAria")} />
                  </label>
                )}
                {fit.kind !== "ok" && <div className="note warn" role="status"><div>{explainFit(fit, frameDisplayName(picked, language), language)}</div></div>}
                <div className="frame-actions">
                  {fit.kind === "ok" && (
                    <button className="primary big" disabled={busy} onClick={() => confirm({})}>{busy ? t("pleaseWait") : (confirmLabel ?? t("continueLabel"))}</button>
                  )}
                  {fit.kind === "fewer" && <button className="primary big" disabled={busy} onClick={() => confirm({ useFirst: true })}>{formatTranslation(language, "firstPhotos", { count: fit.need })}</button>}
                  {fit.kind === "more" && <button className="primary big" disabled={busy} onClick={() => confirm({ restart: true })}>{t("restartFrame")}</button>}
                </div>
              </>
            ) : <p className="muted">{t("selectFrame")}</p>}
          </aside>
        </div>
      )}
    </div>
  );
}
