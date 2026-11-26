import { useState, type ReactNode } from "react";
import { formatTranslation } from "../../../shared/i18n";
import { Icon } from "../../../shared/icons";
import { MediaImage } from "../../../shared/MediaImage";
import type { Shot } from "../../../shared/types";
import { selectGridShape, type PhotoSelection } from "../photoSelection";

/**
 * Every captured photograph, each one a single large touch target: tap to select, tap again to unselect. A selected photo
 * shows a clear border and its number in the selection (1 = first in the frame). The tile takes the photograph's real
 * aspect ratio, so nothing is cropped in the review; the crop each frame window will use is applied by the compositor.
 * No camera call is made from here.
 */
export function PhotoSelectGrid({ candidates, selection, mediaUrl, language, disabled, renderExtra, onImageStatus }:
  { candidates: readonly Shot[]; selection: PhotoSelection; mediaUrl: (id: string, kind: "preview") => string; language?: string; disabled?: boolean; renderExtra?: (shot: Shot) => ReactNode; onImageStatus?: (id: string, ready: boolean) => void }) {
  const ordered = [...candidates].sort((a, b) => a.slot - b.slot);
  const landscape = selectGridShape(ordered.length, false), portrait = selectGridShape(ordered.length, true);
  const vars = { ["--cl" as string]: landscape.cols, ["--rl" as string]: landscape.rows, ["--cp" as string]: portrait.cols, ["--rp" as string]: portrait.rows };
  return (
    <div className="g-pick-grid" style={vars} data-count={ordered.length}>
      {ordered.map((shot) => <PickTile key={shot.mediaId} shot={shot} src={mediaUrl(shot.mediaId, "preview")} order={selection.orderOf(shot.mediaId)} language={language}
        disabled={!!disabled} onToggle={() => selection.toggle(shot.mediaId)} onImageStatus={onImageStatus} extra={renderExtra?.(shot)} />)}
    </div>
  );
}

function PickTile({ shot, src, order, language, disabled, onToggle, onImageStatus, extra }: { shot: Shot; src: string; order: number; language?: string; disabled: boolean; onToggle: () => void; onImageStatus?: (id: string, ready: boolean) => void; extra?: ReactNode }) {
  const [ar, setAr] = useState(1.5);
  const selected = order > 0;
  const label = selected ? formatTranslation(language, "photoSelectedAria", { number: shot.slot + 1, order }) : formatTranslation(language, "photoUnselectedAria", { number: shot.slot + 1 });
  return (
    <div className={`g-pick ${selected ? "on" : ""}`}>
      <button type="button" className="g-pick-tile" aria-pressed={selected} aria-label={label} disabled={disabled} onClick={onToggle} data-slot={shot.slot}>
        <span className="g-pick-img" style={{ ["--ar" as string]: ar }}>
          <MediaImage src={src} alt="" diagnosticId={shot.mediaId} onStatus={(ready) => onImageStatus?.(shot.mediaId, ready)} onDecoded={(w, h) => setAr(w / h)} className="g-pick-media"
            revision={shot.mediaId} />
          <span className="g-pick-badge" aria-hidden="true">{selected ? order : <Icon name="check" size={18} />}</span>
        </span>
      </button>
      {extra}
    </div>
  );
}
