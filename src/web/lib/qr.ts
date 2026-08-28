/** QR Code Model 2, versions 1–6, ECC L, byte mode. Enough for an otpauth URL. */

const CAPACITY = [0, 19, 34, 55, 80, 108, 136];
const EC_COUNT = [0, 7, 10, 15, 20, 26, 18];
const EC_BLOCKS = [0, 1, 1, 1, 1, 1, 2];
const REMAINDER = [0, 0, 7, 7, 7, 7, 7];
const ALIGN = [[], [], [18], [22], [26], [30], [34]];

const EXP = new Uint8Array(512);
const LOG = new Uint8Array(256);
(() => {
  let x = 1;
  for (let i = 0; i < 255; i++) {
    EXP[i] = x;
    LOG[x] = i;
    x <<= 1;
    if (x & 0x100) x ^= 0x11d;
  }
  for (let i = 255; i < 512; i++) EXP[i] = EXP[i - 255];
})();

function gfMul(a: number, b: number) {
  if (!a || !b) return 0;
  return EXP[LOG[a] + LOG[b]];
}

function rsGenerator(n: number) {
  let poly = new Uint8Array([1]);
  for (let i = 0; i < n; i++) {
    const next = new Uint8Array(poly.length + 1);
    for (let j = 0; j < poly.length; j++) {
      next[j] ^= poly[j];
      next[j + 1] ^= gfMul(poly[j], EXP[i]);
    }
    poly = next;
  }
  return poly;
}

function rsEncode(data: Uint8Array, n: number) {
  const gen = rsGenerator(n);
  const rest = new Uint8Array(n);
  for (const b of data) {
    const factor = b ^ rest[0];
    rest.copyWithin(0, 1);
    rest[n - 1] = 0;
    if (!factor) continue;
    for (let i = 0; i < n; i++) rest[i] ^= gfMul(gen[i + 1], factor);
  }
  return rest;
}

function bitBuffer() {
  const bytes: number[] = [];
  let acc = 0;
  let bits = 0;
  const put = (value: number, len: number) => {
    for (let i = len - 1; i >= 0; i--) {
      acc = (acc << 1) | ((value >> i) & 1);
      bits++;
      if (bits === 8) {
        bytes.push(acc);
        acc = 0;
        bits = 0;
      }
    }
  };
  const padTo = (n: number) => {
    if (bits) {
      acc <<= 8 - bits;
      bytes.push(acc);
      acc = 0;
      bits = 0;
    }
    const pads = [0xec, 0x11];
    let i = 0;
    while (bytes.length < n) bytes.push(pads[i++ % 2]);
  };
  return { put, padTo, bytes: () => bytes };
}

function interleave(version: number, data: number[]) {
  const blocks = EC_BLOCKS[version];
  const ecEach = EC_COUNT[version];
  const dataEach = Math.floor(CAPACITY[version] / blocks);
  const groups: { data: Uint8Array; ec: Uint8Array }[] = [];
  let offset = 0;
  for (let i = 0; i < blocks; i++) {
    const chunk = Uint8Array.from(data.slice(offset, offset + dataEach));
    offset += dataEach;
    groups.push({ data: chunk, ec: rsEncode(chunk, ecEach) });
  }
  const out: number[] = [];
  for (let i = 0; i < dataEach; i++) {
    for (const g of groups) out.push(g.data[i]);
  }
  for (let i = 0; i < ecEach; i++) {
    for (const g of groups) out.push(g.ec[i]);
  }
  return out;
}

function finder(mod: number[][], r: number, c: number) {
  for (let y = -1; y <= 7; y++) {
    for (let x = -1; x <= 7; x++) {
      const rr = r + y;
      const cc = c + x;
      if (rr < 0 || cc < 0 || rr >= mod.length || cc >= mod.length) continue;
      const dark =
        (x >= 0 && x <= 6 && (y === 0 || y === 6)) ||
        (y >= 0 && y <= 6 && (x === 0 || x === 6)) ||
        (x >= 2 && x <= 4 && y >= 2 && y <= 4);
      mod[rr][cc] = dark ? 1 : 0;
    }
  }
}

function alignment(mod: number[][], r: number, c: number) {
  for (let y = -2; y <= 2; y++) {
    for (let x = -2; x <= 2; x++) {
      mod[r + y][c + x] = x === 0 && y === 0 || Math.abs(x) === 2 || Math.abs(y) === 2 ? 1 : 0;
    }
  }
}

function reserved(size: number, version: number) {
  const r = Array.from({ length: size }, () => Array(size).fill(false));
  const mark = (y: number, x: number) => {
    if (y >= 0 && x >= 0 && y < size && x < size) r[y][x] = true;
  };
  for (let y = 0; y < 9; y++) for (let x = 0; x < 9; x++) mark(y, x);
  for (let y = 0; y < 9; y++) for (let x = size - 8; x < size; x++) mark(y, x);
  for (let y = size - 8; y < size; y++) for (let x = 0; x < 9; x++) mark(y, x);
  for (let i = 0; i < size; i++) {
    mark(6, i);
    mark(i, 6);
  }
  for (const a of ALIGN[version]) {
    for (const b of [6, ...ALIGN[version]]) {
      if ((a === 6 && b === 6) || (a === 6 && b === size - 7) || (a === size - 7 && b === 6)) continue;
      for (let y = -2; y <= 2; y++) for (let x = -2; x <= 2; x++) mark(a + y, b + x);
    }
  }
  return r;
}

function maskAt(mask: number, y: number, x: number) {
  switch (mask) {
    case 0:
      return (y + x) % 2 === 0;
    case 1:
      return y % 2 === 0;
    case 2:
      return x % 3 === 0;
    case 3:
      return (y + x) % 3 === 0;
    case 4:
      return (Math.floor(y / 2) + Math.floor(x / 3)) % 2 === 0;
    case 5:
      return ((y * x) % 2) + ((y * x) % 3) === 0;
    case 6:
      return (((y * x) % 2) + ((y * x) % 3)) % 2 === 0;
    default:
      return (((y + x) % 2) + ((y * x) % 3)) % 2 === 0;
  }
}

function formatBits(mask: number) {
  let bits = (0b01 << 3) | mask;
  let d = bits << 10;
  const gen = 0b10100110111;
  for (let i = 14; i >= 10; i--) {
    if ((d >> i) & 1) d ^= gen << (i - 10);
  }
  return (bits << 10 | d) ^ 0b101010000010010;
}

function placeFormat(mod: number[][], mask: number) {
  const bits = formatBits(mask);
  const size = mod.length;
  for (let i = 0; i < 15; i++) {
    const bit = (bits >> i) & 1;
    if (i < 6) mod[i][8] = bit;
    else if (i < 8) mod[i + 1][8] = bit;
    else mod[size - 15 + i][8] = bit;
    if (i < 8) mod[8][size - 1 - i] = bit;
    else if (i === 8) mod[8][7] = bit;
    else mod[8][14 - i] = bit;
  }
  mod[size - 8][8] = 1;
}

function penalty(mod: number[][]) {
  const n = mod.length;
  let s = 0;
  for (let y = 0; y < n; y++) {
    let run = 1;
    for (let x = 1; x < n; x++) {
      if (mod[y][x] === mod[y][x - 1]) run++;
      else {
        if (run >= 5) s += run - 2;
        run = 1;
      }
    }
    if (run >= 5) s += run - 2;
  }
  for (let x = 0; x < n; x++) {
    let run = 1;
    for (let y = 1; y < n; y++) {
      if (mod[y][x] === mod[y - 1][x]) run++;
      else {
        if (run >= 5) s += run - 2;
        run = 1;
      }
    }
    if (run >= 5) s += run - 2;
  }
  for (let y = 0; y < n - 1; y++) {
    for (let x = 0; x < n - 1; x++) {
      const v = mod[y][x];
      if (v === mod[y][x + 1] && v === mod[y + 1][x] && v === mod[y + 1][x + 1]) s += 3;
    }
  }
  let dark = 0;
  for (let y = 0; y < n; y++) for (let x = 0; x < n; x++) dark += mod[y][x];
  s += Math.floor(Math.abs(dark * 20 - n * n * 10) / (n * n)) * 10;
  return s;
}

export function qrMatrix(text: string): number[][] {
  const data = new TextEncoder().encode(text);
  let version = 1;
  while (version <= 6 && data.length + 2 > CAPACITY[version] - 2) version++;
  if (version > 6) throw new Error("QR payload is too long");
  const cap = CAPACITY[version];
  const buf = bitBuffer();
  buf.put(0b0100, 4);
  buf.put(data.length, 8);
  for (const b of data) buf.put(b, 8);
  buf.put(0, Math.min(4, cap * 8 - (4 + 8 + data.length * 8)));
  buf.padTo(cap);
  const code = interleave(version, buf.bytes());
  const size = version * 4 + 17;
  const base = Array.from({ length: size }, () => Array(size).fill(0));
  finder(base, 0, 0);
  finder(base, 0, size - 7);
  finder(base, size - 7, 0);
  for (let i = 8; i < size - 8; i++) {
    base[6][i] = i % 2 === 0 ? 1 : 0;
    base[i][6] = i % 2 === 0 ? 1 : 0;
  }
  for (const a of ALIGN[version]) {
    for (const b of [6, ...ALIGN[version]]) {
      if ((a <= 8 && b <= 8) || (a <= 8 && b >= size - 8) || (a >= size - 8 && b <= 8)) continue;
      alignment(base, a, b);
    }
  }
  const res = reserved(size, version);
  const bits: number[] = [];
  for (const b of code) for (let i = 7; i >= 0; i--) bits.push((b >> i) & 1);
  for (let i = 0; i < REMAINDER[version]; i++) bits.push(0);

  let best: number[][] | null = null;
  let bestScore = Infinity;
  for (let mask = 0; mask < 8; mask++) {
    const mod = base.map((row) => row.slice());
    let bi = 0;
    let dir = -1;
    for (let x = size - 1; x > 0; x -= 2) {
      if (x === 6) x--;
      for (let i = 0; i < size; i++) {
        const y = dir < 0 ? size - 1 - i : i;
        for (const dx of [0, -1]) {
          const xx = x + dx;
          if (res[y][xx]) continue;
          let bit = bits[bi++] || 0;
          if (maskAt(mask, y, xx)) bit ^= 1;
          mod[y][xx] = bit;
        }
      }
      dir *= -1;
    }
    placeFormat(mod, mask);
    const score = penalty(mod);
    if (score < bestScore) {
      bestScore = score;
      best = mod;
    }
  }
  return best!;
}
