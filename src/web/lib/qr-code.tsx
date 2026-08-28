"use client";

import { qrMatrix } from "@/lib/qr";

export function QrCode({ value, label }: { value: string; label?: string }) {
  let modules: number[][];
  try {
    modules = qrMatrix(value);
  } catch {
    return <p className="muted">Could not draw a QR code. Enter the secret manually.</p>;
  }
  const n = modules.length;
  const dim = n + 8;
  const cells = modules
    .flatMap((row, y) => row.map((v, x) => (v ? `<rect x="${x + 4}" y="${y + 4}" width="1" height="1"/>` : "")))
    .join("");
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${dim} ${dim}" shape-rendering="crispEdges"><rect width="${dim}" height="${dim}" fill="#fff"/>${cells}</svg>`;
  return (
    <img
      className="qr-img"
      alt={label ?? "QR code"}
      width={200}
      height={200}
      src={`data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`}
    />
  );
}
