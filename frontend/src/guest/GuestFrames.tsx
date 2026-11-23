import { formatTranslation, photoCount, translate } from "../i18n";
import { useEffect, useRef, useState } from "react";
import { fitFor, explainFit, photoCounts, swatch, type Fit } from "../frameInfo";
import { Icon } from "../icons";
import type { Frame } from "../types";
import { FrameThumb, Fit as FitImage, LayoutBadge, PhotoDots, PrimaryAction, SecondaryAction } from "./parts";
import { useMoreBelow } from "./useMoreBelow";

export interface GuestFramesProps {
  language?: string; frames: Frame[]; categories: string[]; shown: Frame[];
  category: string; setCategory: (c: string) => void; count: number; setCount: (n: number) => void;
  picked?: Frame; pickId: string; select: (f: Frame) => void;
  variantId: string; setVariantId: (id: string) => void; caption: string; setCaption: (c: string) => void;
  fit: Fit; takenPhotos?: number; busy?: boolean;
  title: string; confirmLabel?: string; backLabel?: string; onBack?: () => void;
  confirm: (d: { useFirst?: boolean; restart?: boolean }) => void;
  thumbUrl: (f: Frame) => string; bigUrl: (f: Frame) => string; portrait: boolean;
}

/** The guest frame gallery. Landscape: gallery | preview + details. Portrait: gallery above a compact summary.
 *  Continue / Back are a persistent bar at the bottom in both. */
export function GuestFrames(p: GuestFramesProps) {
  const { picked, frames } = p;
  const t = (key: Parameters<typeof translate>[1]) => translate(p.language, key);
  const scroller = useMoreBelow([p.shown.length]);
  const [typing, setTyping] = useState(false);
  const blurTimer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(blurTimer.current), []);

  // after the list changes (filter, first load) the selected frame stays in view
  useEffect(() => {
    scroller.ref.current?.querySelector<HTMLElement>('[aria-checked="true"]')?.scrollIntoView?.({ block: "nearest" });
  }, [p.category, p.count, frames.length]); // eslint-disable-line react-hooks/exhaustive-deps

  const counts = photoCounts(frames);
  const variants = picked?.variants ?? [];
  const activeVariant = variants.find((v) => v.id === p.variantId) ?? variants[0];
  const label = p.fit.kind === "ok" ? (p.confirmLabel ?? t("continueLabel")) : p.fit.kind === "fewer" ? formatTranslation(p.language, "firstPhotos", { count: p.fit.need }) : t("restartFrame");
  const decision = p.fit.kind === "fewer" ? { useFirst: true } : p.fit.kind === "more" ? { restart: true } : {};

  return (
    <div className="g-frames" data-typing={typing ? "true" : "false"}>
      <div className="g-frames-main">
        <section className="g-gallery" aria-label="Frames">
          <div className="g-titlebar">
            <h1>{p.title}</h1>
            <p>{t("tapDesign")}</p>
          </div>
          <div className="g-filters">
            <div className="g-chips" role="group" aria-label="Frame categories">
              {["All", ...p.categories].map((c) => (
                <button key={c} type="button" className={`g-chip ${p.category === c ? "on" : ""}`} aria-pressed={p.category === c} onClick={() => p.setCategory(c)}>{c === "All" ? t("all") : c}</button>
              ))}
            </div>
            {counts.length > 1 && (
              <div className="g-chips" role="group" aria-label="Number of photos">
                <button type="button" className={`g-chip ${p.count === 0 ? "on" : ""}`} aria-pressed={p.count === 0} onClick={() => p.setCount(0)}>{t("anyPhotos")}</button>
                {counts.map((n) => (
                  <button key={n} type="button" className={`g-chip ${p.count === n ? "on" : ""}`} aria-pressed={p.count === n} onClick={() => p.setCount(n)}>{photoCount(p.language, n)}</button>
                ))}
              </div>
            )}
          </div>
          <div className={`g-scroll ${scroller.more ? "more" : ""}`}>
            <div ref={scroller.ref} className="g-scroll-area" onScroll={scroller.onScroll}>
              {p.shown.length === 0 && <p className="g-empty">{t("noFramesMatch")}</p>}
              <div className="g-grid" role="radiogroup" aria-label="Frames">
                {p.shown.map((f) => {
                  const mis = fitFor(f, p.takenPhotos).kind !== "ok";
                  return <FrameThumb key={f.id} frame={f} src={p.thumbUrl(f)} selected={f.id === p.pickId} mismatch={mis} note={formatTranslation(p.language, "needsPhotos", { count: f.photos })} onSelect={() => p.select(f)} />;
                })}
              </div>
            </div>
            {scroller.more && <span className="g-more" aria-hidden="true"><Icon name="chevdown" size={22} />{t("moreFrames")}</span>}
          </div>
        </section>

        <aside className="g-detail" aria-label="Selected frame">
          {picked ? (
            <>
              <FitImage className="g-detail-img" src={p.bigUrl(picked)} alt={`${picked.name} preview`} />
              <div className="g-detail-info">
                <h2>{picked.name}</h2>
                <p className="g-detail-meta"><LayoutBadge format={picked.format} /><PhotoDots n={picked.photos} /><span>{photoCount(p.language, picked.photos)}</span></p>
                {variants.length > 1 && (
                  <div className="g-colors">
                    <div className="g-swatches" role="radiogroup" aria-label="Colour">
                      {variants.map((v, i) => {
                        const on = p.variantId === v.id || (!p.variantId && i === 0);
                        return (
                          <button key={v.id} type="button" role="radio" aria-checked={on} aria-label={v.name} title={v.name} className={`g-swatch ${on ? "on" : ""}`}
                            style={{ background: i === 0 ? picked.background : swatch(v, picked) }} onClick={() => p.setVariantId(i === 0 ? "" : v.id)}>
                          </button>
                        );
                      })}
                    </div>
                    <span className="g-swatch-name">{activeVariant?.name}</span>
                  </div>
                )}
                {picked.captionEnabled && (
                  <label className="g-caption">{t("caption")}
                    <input value={p.caption} maxLength={40} placeholder="e.g. Mai & Nam" aria-label="Caption" enterKeyHint="done"
                      onChange={(e) => p.setCaption(e.target.value)}
                      onFocus={(e) => { window.clearTimeout(blurTimer.current); setTyping(true); const el = e.currentTarget; window.setTimeout(() => el.scrollIntoView?.({ block: "center" }), 250); }}
                      onBlur={() => { blurTimer.current = window.setTimeout(() => setTyping(false), 400); }}
                      onKeyDown={(e) => { if (e.key === "Enter") e.currentTarget.blur(); }} />
                  </label>
                )}
                {p.fit.kind !== "ok" && <div className="g-message warn" role="status"><div className="g-message-body"><div>{explainFit(p.fit, picked.name)}</div></div></div>}
              </div>
            </>
          ) : <p className="g-empty">{t("selectFrame")}</p>}
        </aside>
      </div>

      <div className="g-actions">
        {p.onBack && <SecondaryAction icon="back" className="g-back" disabled={p.busy} onClick={p.onBack}>{p.backLabel ?? t("back")}</SecondaryAction>}
        <PrimaryAction icon="arrow" busy={p.busy} disabled={p.busy || !picked} onClick={() => p.confirm(decision)}>{p.busy ? t("pleaseWait") : label}</PrimaryAction>
      </div>
    </div>
  );
}
