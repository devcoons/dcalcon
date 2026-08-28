import type { NextConfig } from "next";

// Rewrites are evaluated at `next build` for `next start` / standalone.
// `next dev` re-reads this file on startup. Default matches `make -C src/svc run` (:8080).
const api = process.env.API_URL || "http://127.0.0.1:8080";

const nextConfig: NextConfig = {
  output: "standalone",
  agentRules: false,
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${api}/api/:path*` },
      { source: "/healthz", destination: `${api}/healthz` },
      { source: "/readyz", destination: `${api}/readyz` },
      { source: "/version", destination: `${api}/version` },
      { source: "/webcal/:path*", destination: `${api}/webcal/:path*` },
    ];
  },
};

export default nextConfig;
