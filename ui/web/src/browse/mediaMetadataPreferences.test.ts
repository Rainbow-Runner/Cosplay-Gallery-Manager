import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { defaultMediaMetadataVisibleFields, readMediaMetadataVisibleFields, writeMediaMetadataVisibleFields } from "./mediaMetadataPreferences";

beforeEach(() => {
  const values = new Map<string, string>();
  Object.defineProperty(window, "localStorage", { configurable: true, value: {
    clear: () => values.clear(),
    getItem: (key: string) => values.get(key) ?? null,
    removeItem: (key: string) => values.delete(key),
    setItem: (key: string, value: string) => values.set(key, value),
  } });
});
afterEach(() => window.localStorage.clear());

describe("media metadata visibility preferences", () => {
  it("keeps sensitive metadata disabled by default", () => {
    const values = readMediaMetadataVisibleFields();
    expect(values).toEqual(defaultMediaMetadataVisibleFields);
    expect(values).not.toContain("sensitive.gps");
    expect(values).not.toContain("sensitive.device_identifiers");
  });

  it("round-trips a valid per-browser selection", () => {
    writeMediaMetadataVisibleFields(["file.type", "descriptive.author", "sensitive.gps"]);
    expect(readMediaMetadataVisibleFields()).toEqual(["file.type", "descriptive.author", "sensitive.gps"]);
  });

  it("falls back safely when stored JSON is malformed", () => {
    window.localStorage.setItem("cgm.mediaMetadataVisibleFields.v1", "not-json");
    expect(readMediaMetadataVisibleFields()).toEqual(defaultMediaMetadataVisibleFields);
  });
});
