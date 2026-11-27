// 生成 Tauri updater 使用的 latest.json。
// 用法：
//   node scripts/make-latest-json.mjs --notes "修复若干问题"
//   KAIXIN_UPDATE_BASE_URL=https://your-server/desktop node scripts/make-latest-json.mjs
//
// 前置：npm run tauri build 已生成
//   src-tauri/target/release/bundle/nsis/KaixinAssistant_<version>_x64-setup.exe
//   src-tauri/target/release/bundle/nsis/KaixinAssistant_<version>_x64-setup.exe.sig
// 输出：src-tauri/target/release/bundle/nsis/latest.json

import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const projectRoot = resolve(here, "..");
const pkg = JSON.parse(readFileSync(join(projectRoot, "package.json"), "utf8"));
const version = pkg.version;
const bundleDir = join(projectRoot, "src-tauri", "target", "release", "bundle", "nsis");
const installer = join(bundleDir, `KaixinAssistant_${version}_x64-setup.exe`);
const signatureFile = `${installer}.sig`;

const argv = process.argv.slice(2);
const noteIndex = argv.indexOf("--notes");
const notes = noteIndex >= 0 ? (argv[noteIndex + 1] ?? "") : `开心同学 AI 助手 ${version}`;
const baseUrl = (process.env.KAIXIN_UPDATE_BASE_URL || "https://update.kaixin.example.com/desktop").replace(/\/$/, "");

for (const file of [installer, signatureFile]) {
  if (!existsSync(file)) {
    console.error(`缺少文件：${file}`);
    console.error("请先运行 npm run tauri build，并确认 tauri.conf.json 里 createUpdaterArtifacts=true。");
    process.exit(1);
  }
}

const signature = readFileSync(signatureFile, "utf8").trim();
const fileName = installer.split(/[\\/]/).pop();
const latest = {
  version,
  notes,
  pub_date: new Date().toISOString(),
  platforms: {
    "windows-x86_64": {
      signature,
      url: `${baseUrl}/${fileName}`
    }
  }
};

const out = join(bundleDir, "latest.json");
writeFileSync(out, JSON.stringify(latest, null, 2) + "\n", "utf8");
console.log(`已生成 ${out}`);
console.log(`安装包 URL：${latest.platforms["windows-x86_64"].url}`);
