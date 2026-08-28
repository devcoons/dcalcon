#!/usr/bin/env node
/**
 * Runtime reverse proxy so API_URL can change after `next build`.
 * Next standalone rewrites are baked at compile time; this process listens on
 * PORT and forwards /api, /healthz, /readyz, /version, /webcal to API_URL,
 * and everything else to the Next server on 127.0.0.1:3001.
 */
import http from "node:http";
import { spawn } from "node:child_process";
import { setTimeout as sleep } from "node:timers/promises";

const publicPort = Number(process.env.PORT || 3000);
const nextPort = Number(process.env.NEXT_INTERNAL_PORT || 3001);
const api = (process.env.API_URL || "http://127.0.0.1:8080").replace(/\/$/, "");
const hostname = process.env.HOSTNAME || "0.0.0.0";

process.env.PORT = String(nextPort);
process.env.HOSTNAME = "127.0.0.1";

const child = spawn(process.execPath, ["server.js"], {
  stdio: "inherit",
  env: process.env,
});
child.on("exit", (code) => process.exit(code ?? 1));

function shouldProxy(url) {
  return (
    url.startsWith("/api") ||
    url.startsWith("/healthz") ||
    url.startsWith("/readyz") ||
    url.startsWith("/version") ||
    url.startsWith("/webcal")
  );
}

function clientIP(req) {
  const addr = req.socket?.remoteAddress || "";
  return addr.startsWith("::ffff:") ? addr.slice(7) : addr;
}

function isTrustedPeer(ip) {
  return (
    ip === "127.0.0.1" ||
    ip === "::1" ||
    ip.startsWith("10.") ||
    ip.startsWith("192.168.") ||
    /^172\.(1[6-9]|2\d|3[0-1])\./.test(ip)
  );
}

function forwardedHeaders(req, destHost) {
  const headers = { ...req.headers, host: destHost };
  const incoming = headers["x-forwarded-for"];
  const protoIn = headers["x-forwarded-proto"];
  delete headers["x-forwarded-for"];
  delete headers["x-forwarded-host"];
  delete headers["x-forwarded-proto"];
  delete headers["x-real-ip"];
  const peer = clientIP(req);
  if (isTrustedPeer(peer) && incoming) {
    const left = String(incoming).split(",")[0].trim();
    headers["x-forwarded-for"] = left ? `${left}, ${peer}` : peer;
  } else {
    headers["x-forwarded-for"] = peer;
  }
  if (isTrustedPeer(peer) && protoIn) {
    headers["x-forwarded-proto"] = String(protoIn).split(",")[0].trim() || "http";
  }
  return headers;
}

function proxy(req, res, targetBase) {
  const dest = new URL(req.url || "/", targetBase.endsWith("/") ? targetBase : targetBase + "/");
  const headers = forwardedHeaders(req, dest.host);
  const hop = http.request(
    dest,
    { method: req.method, headers },
    (up) => {
      res.writeHead(up.statusCode || 502, up.headers);
      up.pipe(res);
    },
  );
  hop.on("error", (err) => {
    if (!res.headersSent) {
      res.writeHead(502, { "content-type": "text/plain" });
    }
    res.end("upstream error: " + err.message);
  });
  req.pipe(hop);
}

const server = http.createServer((req, res) => {
  const url = req.url || "/";
  if (shouldProxy(url)) {
    proxy(req, res, api);
    return;
  }
  proxy(req, res, `http://127.0.0.1:${nextPort}`);
});

async function listen() {
  for (let i = 0; i < 50; i++) {
    try {
      await new Promise((resolve, reject) => {
        const probe = http.get(`http://127.0.0.1:${nextPort}/`, (r) => {
          r.resume();
          resolve();
        });
        probe.on("error", reject);
      });
      break;
    } catch {
      await sleep(100);
    }
  }
  server.listen(publicPort, hostname, () => {
    console.log(`web proxy listening on ${hostname}:${publicPort} → next :${nextPort}, api ${api}`);
  });
}

listen().catch((err) => {
  console.error(err);
  process.exit(1);
});
