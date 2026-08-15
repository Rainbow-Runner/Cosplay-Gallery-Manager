#!/usr/bin/env node

import { readFileSync, readdirSync, realpathSync, statSync, writeFileSync } from "node:fs";
import { basename, dirname, join } from "node:path";
import { spawnSync } from "node:child_process";

const root = new URL("../", import.meta.url).pathname;
const go = process.env.CGM_GO || "go";

function run(command, args, cwd = root) {
  const result = spawnSync(command, args, { cwd, encoding: "utf8", env: { ...process.env, GOTOOLCHAIN: "local" } });
  if (result.status !== 0) {
    process.stderr.write(result.stderr || result.stdout);
    process.exit(result.status || 1);
  }
  return result.stdout.trim();
}

function detectLicense(directory) {
  let names = [];
  try {
    names = readdirSync(directory).filter((name) => /^(licen[cs]e|copying)(\..*)?$/i.test(name));
  } catch {
    return "NOASSERTION";
  }
  const text = names.map((name) => {
    try {
      const path = join(directory, name);
      return statSync(path).isFile() ? readFileSync(path, "utf8").slice(0, 32000) : "";
    } catch {
      return "";
    }
  }).join("\n").toLowerCase();
  if (text.includes("apache license") && text.includes("version 2.0")) return "Apache-2.0";
  if (text.includes("gnu affero general public license")) return text.includes("either version 3") ? "AGPL-3.0-or-later" : "AGPL-3.0-only";
  if (text.includes("gnu lesser general public license")) return "LGPL-3.0-or-later";
  if (text.includes("mozilla public license") && text.includes("2.0")) return "MPL-2.0";
  if (text.includes("cc0 1.0 universal") || text.includes("public domain with cc0 1.0")) return "CC0-1.0";
  if (text.includes("permission is hereby granted, free of charge")) return "MIT";
  if (text.includes("redistribution and use in source and binary forms")) {
    return text.includes("neither the name") ? "BSD-3-Clause" : "BSD-2-Clause";
  }
  if (text.includes("permission to use, copy, modify, and/or distribute")) return "ISC";
  if (text.includes("the unlicense")) return "Unlicense";
  return "NOASSERTION";
}

const goRows = run(go, [
  "list", "-deps", "-tags", "cgm_web_embed,cgm_galleryepic",
  "-f", "{{if and .Module .Module.Version}}{{.Module.Path}}\t{{.Module.Version}}\t{{.Module.Dir}}{{end}}",
  "./cmd/cgm",
]).split("\n").filter(Boolean);
const goPackages = [...new Map(goRows.map((row) => {
  const [name, version, directory] = row.split("\t");
  return [`go:${name}@${version}`, {
    ecosystem: "Go", name, version, license: detectLicense(directory),
    download: `https://proxy.golang.org/${name}/@v/${version}.zip`,
  }];
})).values()];

const nodePackagesByID = new Map();
function visitNodePackage(directory) {
  const manifestPath = join(directory, "package.json");
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  if (manifest.name !== "@cosplay-gallery-manager/web") {
    const key = `${manifest.name}@${manifest.version}`;
    if (nodePackagesByID.has(key)) return;
    nodePackagesByID.set(key, {
      ecosystem: "npm", name: manifest.name, version: manifest.version,
      license: typeof manifest.license === "string" ? manifest.license : "NOASSERTION",
      download: `https://registry.npmjs.org/${manifest.name}/-/${basename(manifest.name)}-${manifest.version}.tgz`,
      homepage: typeof manifest.homepage === "string" ? manifest.homepage : undefined,
    });
  }
  for (const dependency of Object.keys(manifest.dependencies || {})) {
    let modulesDirectory = join(directory, "node_modules");
    let cursor = directory;
    while (basename(cursor) !== "node_modules" && dirname(cursor) !== cursor) cursor = dirname(cursor);
    if (basename(cursor) === "node_modules") modulesDirectory = cursor;
    const dependencyManifest = realpathSync(join(modulesDirectory, dependency, "package.json"));
    visitNodePackage(dirname(dependencyManifest));
  }
}
visitNodePackage(join(root, "ui/web"));
const nodePackages = [...nodePackagesByID.values()];
const dependencies = [...goPackages, ...nodePackages].sort((left, right) =>
  left.ecosystem.localeCompare(right.ecosystem) || left.name.localeCompare(right.name) || left.version.localeCompare(right.version));

const sourceRevision = process.env.CGM_SOURCE_REVISION || "source";
const created = process.env.CGM_SBOM_CREATED || "1970-01-01T00:00:00Z";
const productID = "SPDXRef-Package-Cosplay-Gallery-Manager";
const spdxID = (dependency, index) => `SPDXRef-Package-${dependency.ecosystem}-${index + 1}`;
const sbom = {
  spdxVersion: "SPDX-2.3",
  dataLicense: "CC0-1.0",
  SPDXID: "SPDXRef-DOCUMENT",
  name: "Cosplay Gallery Manager application dependency SBOM",
  documentNamespace: "https://github.com/Rainbow-Runner/Cosplay-Gallery-Manager/sbom/application-dependencies/v1",
  creationInfo: { created, creators: ["Tool: scripts/generate-cgm-legal-metadata.mjs"] },
  packages: [{
    name: "Cosplay Gallery Manager", SPDXID: productID, versionInfo: "1.5.0-dev",
    downloadLocation: "NOASSERTION", filesAnalyzed: false,
    licenseConcluded: "AGPL-3.0-or-later", licenseDeclared: "AGPL-3.0-or-later",
    supplier: "Organization: Cosplay Gallery Manager contributors",
    externalRefs: [{ referenceCategory: "PACKAGE-MANAGER", referenceType: "purl", referenceLocator:
      `pkg:github/Rainbow-Runner/Cosplay-Gallery-Manager${sourceRevision === "source" ? "" : `@${sourceRevision}`}` }],
  }, ...dependencies.map((dependency, index) => ({
    name: dependency.name, SPDXID: spdxID(dependency, index), versionInfo: dependency.version,
    downloadLocation: dependency.download, filesAnalyzed: false,
    licenseConcluded: dependency.license, licenseDeclared: dependency.license,
    homepage: dependency.homepage,
    externalRefs: [{ referenceCategory: "PACKAGE-MANAGER", referenceType: "purl",
      referenceLocator: `pkg:${dependency.ecosystem === "Go" ? "golang" : "npm"}/${dependency.name}@${dependency.version}` }],
  }))],
  relationships: [
    { spdxElementId: "SPDXRef-DOCUMENT", relationshipType: "DESCRIBES", relatedSpdxElement: productID },
    ...dependencies.map((dependency, index) => ({
      spdxElementId: productID, relationshipType: "DEPENDS_ON", relatedSpdxElement: spdxID(dependency, index),
    })),
  ],
};

const notice = `# Third-Party Notices

Cosplay Gallery Manager is distributed under the GNU Affero General Public License,
version 3 or later. The complete terms are in \`LICENSE\`.

This project is derived from the [Stash](https://github.com/stashapp/stash) project.
Stash code and contributions remain under the GNU Affero General Public License;
the repository history preserves the corresponding authorship and copyright record.

The application dependency inventory below is generated from the Go packages compiled
for \`cmd/cgm\` and the production web dependency graph. License identifiers are
informational classifications; the dependency's own license file is authoritative.
The machine-readable companion is \`docs/legal/cgm.spdx.json\`.

| Ecosystem | Package | Version | Declared/detected licence |
| --- | --- | --- | --- |
${dependencies.map((dependency) => `| ${dependency.ecosystem} | \`${dependency.name}\` | \`${dependency.version}\` | ${dependency.license} |`).join("\n")}

The Docker image also contains Debian, FFmpeg and dcraw/LibRaw packages. Their exact
versions and notices are image-platform-specific; release images must additionally
publish the BuildKit image SBOM/attestation. This application SBOM does not substitute
for that container operating-system inventory.
`;

writeFileSync(join(root, "THIRD_PARTY_NOTICES.md"), notice);
writeFileSync(join(root, "docs/legal/cgm.spdx.json"), `${JSON.stringify(sbom, null, 2)}\n`);
process.stdout.write(`wrote ${dependencies.length} application dependencies\n`);
