"use client";

import { useRef, useState } from "react";
import { FileText, Paperclip, X } from "lucide-react";
import { api, type Attachment } from "@/lib/api";
import { formatBytes } from "@/lib/format";

const MAX_FILES = 20;
const MAX_FILE_BYTES = 8 * 1024 * 1024;

export function AttachmentEditor({
  saved,
  pending,
  canEdit,
  onPending,
  onRemoveSaved,
  onRemovePending,
  onDownload,
}: {
  saved: Attachment[];
  pending: File[];
  canEdit: boolean;
  onPending: (files: File[]) => void;
  onRemoveSaved: (id: string) => void;
  onRemovePending: (index: number) => void;
  onDownload?: (a: Attachment) => void;
}) {
  const input = useRef<HTMLInputElement>(null);
  const [skipHint, setSkipHint] = useState("");
  const count = saved.length + pending.length;

  function pick(list: FileList | null) {
    if (!list?.length) return;
    const next: File[] = [];
    let skipped = 0;
    for (const f of Array.from(list)) {
      if (f.size > MAX_FILE_BYTES) {
        skipped++;
        continue;
      }
      next.push(f);
    }
    const room = Math.max(0, MAX_FILES - count);
    const kept = next.slice(0, room);
    onPending([...pending, ...kept]);
    if (skipped > 0 || kept.length < next.length) {
      setSkipHint("Some files were skipped (over 8 MB or the 20-file limit).");
    } else {
      setSkipHint("");
    }
    if (input.current) input.current.value = "";
  }

  return (
    <div className="attach-box">
      <span>Files</span>
      {saved.length === 0 && pending.length === 0 ? (
        <p className="muted">{canEdit ? "PDFs, images, or other files. Up to 20, 8 MB each." : "No files."}</p>
      ) : null}
      <ul className="attach-list">
        {saved.map((a) => (
          <li key={a.id} className="attach-row">
            <FileText size={16} aria-hidden />
            <button type="button" className="attach-name" onClick={() => onDownload?.(a)} disabled={!onDownload}>
              {a.filename}
            </button>
            <span className="muted">{formatBytes(a.size)}</span>
            {canEdit ? (
              <button type="button" className="btn ghost sm icon-btn" aria-label={`Remove ${a.filename}`} onClick={() => onRemoveSaved(a.id)}>
                <X size={14} />
              </button>
            ) : null}
          </li>
        ))}
        {pending.map((f, i) => (
          <li key={`p-${f.name}-${i}`} className="attach-row">
            <Paperclip size={16} aria-hidden />
            <span className="attach-name">{f.name}</span>
            <span className="muted">{formatBytes(f.size)} · new</span>
            {canEdit ? (
              <button type="button" className="btn ghost sm icon-btn" aria-label={`Remove ${f.name}`} onClick={() => onRemovePending(i)}>
                <X size={14} />
              </button>
            ) : null}
          </li>
        ))}
      </ul>
      {skipHint ? <p className="muted">{skipHint}</p> : null}
      {canEdit ? (
        <>
          <input
            ref={input}
            type="file"
            multiple
            hidden
            onChange={(e) => pick(e.target.files)}
          />
          <button
            type="button"
            className="btn secondary sm"
            onClick={() => input.current?.click()}
            disabled={count >= MAX_FILES}
          >
            <Paperclip size={14} aria-hidden />
            Add files
          </button>
        </>
      ) : null}
    </div>
  );
}

export async function saveAttachments(kind: "event" | "task", calendarId: number, href: string, pending: File[], removed: string[]) {
  for (const id of removed) {
    if (kind === "event") await api.deleteEventAttachment(calendarId, href, id);
    else await api.deleteTaskAttachment(calendarId, href, id);
  }
  for (const file of pending) {
    if (kind === "event") await api.uploadEventAttachment(calendarId, href, file);
    else await api.uploadTaskAttachment(calendarId, href, file);
  }
}
