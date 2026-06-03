import { useCallback, useEffect, useLayoutEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { useI18n } from "../../lib/i18n";

export interface OnboardingTourStep {
  target: string;
  title: string;
  body: string;
}

interface Rect {
  top: number;
  left: number;
  width: number;
  height: number;
}

interface Props {
  open: boolean;
  steps: OnboardingTourStep[];
  onFinish: () => void;
  onSkip: () => void;
}

const CARD_WIDTH = 340;
const CARD_HEIGHT = 228;
const GAP = 14;

function findAnchor(target: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-tour="${target}"]`);
}

function paddedRect(el: HTMLElement): Rect {
  const r = el.getBoundingClientRect();
  const pad = 8;
  return {
    top: Math.max(8, r.top - pad),
    left: Math.max(8, r.left - pad),
    width: Math.max(28, r.width + pad * 2),
    height: Math.max(28, r.height + pad * 2),
  };
}

function clamp(n: number, min: number, max: number) {
  return Math.min(Math.max(n, min), max);
}

export function OnboardingTour({ open, steps, onFinish, onSkip }: Props) {
  const { t } = useI18n();
  const [index, setIndex] = useState(0);
  const [rect, setRect] = useState<Rect | null>(null);

  useEffect(() => {
    if (open) setIndex(0);
  }, [open, steps]);

  const current = steps[index];
  const measure = useCallback(() => {
    if (!current) return;
    const el = findAnchor(current.target);
    if (!el) return;
    setRect(paddedRect(el));
  }, [current]);

  useLayoutEffect(() => {
    if (!open || !current) {
      setRect(null);
      return;
    }
    const el = findAnchor(current.target);
    if (!el) {
      if (index >= steps.length - 1) onFinish();
      else setIndex((i) => i + 1);
      return;
    }
    setRect(null);
    el.scrollIntoView?.({ block: "center", inline: "nearest", behavior: "auto" });
    measure();
    let raf = 0;
    const scheduleMeasure = () => {
      window.cancelAnimationFrame(raf);
      raf = window.requestAnimationFrame(measure);
    };
    scheduleMeasure();
    window.addEventListener("resize", scheduleMeasure);
    window.addEventListener("scroll", scheduleMeasure, true);
    return () => {
      window.cancelAnimationFrame(raf);
      window.removeEventListener("resize", scheduleMeasure);
      window.removeEventListener("scroll", scheduleMeasure, true);
    };
  }, [current, index, measure, onFinish, open, steps.length]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onSkip();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onSkip, open]);

  const cardPos = useMemo(() => {
    const fallback = { left: 24, top: 24 };
    if (!rect) return fallback;
    const vw = window.innerWidth || 1024;
    const vh = window.innerHeight || 768;
    const below = rect.top + rect.height + GAP;
    const above = rect.top - CARD_HEIGHT - GAP;
    const top = below + CARD_HEIGHT < vh ? below : Math.max(24, above);
    return {
      left: clamp(rect.left, 18, Math.max(18, vw - CARD_WIDTH - 18)),
      top: clamp(top, 18, Math.max(18, vh - CARD_HEIGHT - 18)),
    };
  }, [rect]);

  if (!open || !current || !rect) return null;

  const atFirst = index === 0;
  const atLast = index >= steps.length - 1;

  return createPortal(
    <div className="tour-layer" role="dialog" aria-modal="true" aria-label={t("onboarding.ariaLabel")}>
      <div
        className="tour-spotlight"
        style={{
          transform: `translate(${rect.left}px, ${rect.top}px)`,
          width: rect.width,
          height: rect.height,
        }}
      />
      <section
        className="tour-card"
        style={{
          transform: `translate(${cardPos.left}px, ${cardPos.top}px)`,
          width: CARD_WIDTH,
        }}
      >
        <div className="tour-count">
          {t("onboarding.progress", { current: index + 1, total: steps.length })}
        </div>
        <h2>{current.title}</h2>
        <p>{current.body}</p>
        <div className="tour-actions">
          <button type="button" className="btn ghost sm" onClick={onSkip}>
            {t("onboarding.skip")}
          </button>
          <span className="tour-spacer" />
          <button
            type="button"
            className="btn ghost sm"
            onClick={() => setIndex((i) => Math.max(0, i - 1))}
            disabled={atFirst}
          >
            {t("onboarding.previous")}
          </button>
          <button
            type="button"
            className="btn accent sm"
            onClick={() => {
              if (atLast) onFinish();
              else setIndex((i) => i + 1);
            }}
          >
            {atLast ? t("onboarding.finish") : t("onboarding.next")}
          </button>
        </div>
      </section>
    </div>,
    document.body,
  );
}
