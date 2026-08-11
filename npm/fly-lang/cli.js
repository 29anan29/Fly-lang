#!/usr/bin/env node
"use strict";

const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");

const BIN_MAP = {
  linux: { x64: "fly-linux-amd64", arm64: "fly-linux-arm64" },
  darwin: { x64: "fly-darwin-amd64", arm64: "fly-darwin-arm64" },
  win32: { x64: "fly-windows-amd64.exe", arm64: "fly-windows-arm64.exe" },
};

function binaryPath() {
  const binDir = path.join(__dirname, "bin");
  const sys = process.platform;
  const arch = process.arch;
  const name = BIN_MAP[sys] && BIN_MAP[sys][arch];
  if (name) {
    const p = path.join(binDir, name);
    if (fs.existsSync(p)) {
      return p;
    }
  }
  const fallbacks = [path.join(binDir, "fly"), path.join(__dirname, "..", "..", "fly")];
  for (const c of fallbacks) {
    if (fs.existsSync(c)) {
      return c;
    }
  }
  console.error(`未找到 ${sys}/${arch} 的 fly 二进制，请检查安装是否完整（bin/）`);
  process.exit(1);
}

function main() {
  const binary = binaryPath();
  if (process.platform !== "win32") {
    fs.chmodSync(binary, 0o755);
  }
  const r = spawnSync(binary, process.argv.slice(2), {
    stdio: "inherit",
    windowsHide: false,
  });
  if (r.error) {
    console.error(r.error.message);
    process.exit(1);
  }
  process.exit(r.status === null ? 1 : r.status);
}

main();
