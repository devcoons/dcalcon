"use client";

import { useEffect, useRef, useState, type CSSProperties, type PointerEvent, type ReactNode } from "react";
import { Check, Copy, X } from "lucide-react";
import { copyText } from "@/lib/format";

export function PageHeader({
  title,
  lede,
  children,
}: {
  title: string;
  lede?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div className="toolbar">
      <div>
        <h1>{title}</h1>
        {lede ? <p className="lede">{lede}</p> : null}
      </div>
      {children}
    </div>
  );
}

export function Banner({ kind, children }: { kind: "err" | "ok" | "info"; children: ReactNode }) {
  return <div className={`banner ${kind}`}>{children}</div>;
}

export function errorMessage(ex: unknown, fallback: string): string {
  return ex instanceof Error && ex.message ? ex.message : fallback;
}

export function useNotice() {
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  return {
    err,
    ok,
    fail(msg: string) {
      setOk("");
      setErr(msg);
    },
    failFrom(ex: unknown, fallback: string) {
      setOk("");
      setErr(errorMessage(ex, fallback));
    },
    done(msg: string) {
      setErr("");
      setOk(msg);
    },
  };
}

export function Notices({ notice }: { notice: ReturnType<typeof useNotice> }) {
  return (
    <>
      {notice.err ? <Banner kind="err">{notice.err}</Banner> : null}
      {notice.ok ? <Banner kind="ok">{notice.ok}</Banner> : null}
    </>
  );
}

export function Fold({ title, children, defaultOpen }: { title: string; children: ReactNode; defaultOpen?: boolean }) {
  return (
    <details className="fold" open={defaultOpen}>
      <summary>{title}</summary>
      <div className="fold-body">{children}</div>
    </details>
  );
}

export function CopyField({ label, value }: { label: string; value: string }) {
  const [done, setDone] = useState(false);
  return (
    <div className="field">
      <span>{label}</span>
      <div className="copy-row">
        <input readOnly value={value} />
        <button
          className="btn secondary sm"
          type="button"
          onClick={() => copyText(value).then(() => setDone(true))}
        >
          {done ? <Check size={14} aria-hidden /> : <Copy size={14} aria-hidden />}
          {done ? "Copied" : "Copy"}
        </button>
      </div>
    </div>
  );
}

export function IconBtn({
  label,
  onClick,
  children,
  danger,
}: {
  label: string;
  onClick?: () => void;
  children: ReactNode;
  danger?: boolean;
}) {
  return (
    <button
      className={`btn ghost sm icon-btn${danger ? " danger" : ""}`}
      type="button"
      aria-label={label}
      onClick={(e) => {
        e.stopPropagation();
        onClick?.();
      }}
    >
      {children}
    </button>
  );
}

export function Modal({
  title,
  lede,
  icon,
  size = "md",
  className,
  style,
  titleId = "modal-title",
  onClose,
  footer,
  children,
}: {
  title: string;
  lede?: ReactNode;
  icon?: ReactNode;
  size?: "sm" | "md" | "lg";
  className?: string;
  style?: CSSProperties;
  titleId?: string;
  onClose: () => void;
  footer?: ReactNode;
  children: ReactNode;
}) {
  const boxRef = useRef<HTMLDivElement>(null);
  const drag = useRef<{ dx: number; dy: number; w: number; h: number } | null>(null);
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null);
  const [dragging, setDragging] = useState(false);
  const undocked = pos !== null;

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = prev;
    };
  }, [onClose]);

  useEffect(() => {
    if (!undocked) return;
    function onResize() {
      const el = boxRef.current;
      if (!el) return;
      const r = el.getBoundingClientRect();
      setPos((cur) => (cur ? clampPos(cur.x, cur.y, r.width, r.height) : cur));
    }
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, [undocked]);

  function onHeadPointerDown(e: PointerEvent<HTMLDivElement>) {
    if (e.button !== 0) return;
    if ((e.target as HTMLElement).closest("button, a, input")) return;
    const el = boxRef.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    drag.current = { dx: e.clientX - r.left, dy: e.clientY - r.top, w: r.width, h: r.height };
    setPos(clampPos(r.left, r.top, r.width, r.height));
    setDragging(true);
    e.currentTarget.setPointerCapture(e.pointerId);
    e.preventDefault();
  }

  function onHeadPointerMove(e: PointerEvent<HTMLDivElement>) {
    const d = drag.current;
    if (!d) return;
    setPos(clampPos(e.clientX - d.dx, e.clientY - d.dy, d.w, d.h));
  }

  function onHeadPointerUp(e: PointerEvent<HTMLDivElement>) {
    drag.current = null;
    setDragging(false);
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId);
  }

  return (
    <div className="modal-backdrop" onClick={onClose} role="presentation">
      <div
        ref={boxRef}
        className={`modal modal-${size}${className ? ` ${className}` : ""}`}
        style={{
          ...style,
          ...(pos ? { position: "fixed", left: pos.x, top: pos.y, right: "auto", margin: 0 } : undefined),
        }}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
      >
        <div
          className={`modal-head${dragging ? " dragging" : ""}`}
          onPointerDown={onHeadPointerDown}
          onPointerMove={onHeadPointerMove}
          onPointerUp={onHeadPointerUp}
          onPointerCancel={onHeadPointerUp}
        >
          <div className="modal-title">
            {icon}
            <div>
              <h2 id={titleId}>{title}</h2>
              {lede ? <p className="muted">{lede}</p> : null}
            </div>
          </div>
          <button className="btn ghost sm icon-btn" type="button" onClick={onClose} aria-label="Close">
            <X size={18} />
          </button>
        </div>
        <div className="modal-body">{children}</div>
        {footer ? <div className="modal-foot">{footer}</div> : null}
      </div>
    </div>
  );
}

function clampPos(x: number, y: number, w: number, h: number) {
  const pad = 8;
  const maxX = Math.max(pad, window.innerWidth - w - pad);
  const maxY = Math.max(pad, window.innerHeight - h - pad);
  return {
    x: Math.min(Math.max(pad, x), maxX),
    y: Math.min(Math.max(pad, y), maxY),
  };
}
