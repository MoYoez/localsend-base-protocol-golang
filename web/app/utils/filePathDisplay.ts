/**
 * Splits a full path into display fileName and directory path.
 * e.g. "Arknight_Endfield/Hypergryph Launcher/1.0.0/res/web/version.json"
 *   -> { fileName: "version.json", dirPath: "Arknight_Endfield/Hypergryph Launcher/1.0.0/res/web" }
 */
export function splitFilePath(fullPath: string): {
  fileName: string;
  dirPath: string;
} {
  const trimmed = fullPath.trim();
  const lastSlash = trimmed.lastIndexOf("/");
  if (lastSlash === -1) {
    return { fileName: trimmed, dirPath: "" };
  }
  return {
    fileName: trimmed.slice(lastSlash + 1),
    dirPath: trimmed.slice(0, lastSlash),
  };
}
