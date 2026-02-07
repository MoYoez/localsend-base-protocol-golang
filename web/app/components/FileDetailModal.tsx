"use client";

import { useLanguage } from "../context/LanguageContext";
import { splitFilePath } from "../utils/filePathDisplay";

export interface FileInfoForDetail {
  fileName: string;
  size: number;
  fileType: string;
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

interface FileDetailModalProps {
  file: FileInfoForDetail;
  onClose: () => void;
}

export function FileDetailModal({ file, onClose }: FileDetailModalProps) {
  const { t } = useLanguage();
  const { fileName, dirPath } = splitFilePath(file.fileName);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="file-detail-title"
      onClick={onClose}
    >
      <div
        className="w-full max-w-md rounded-lg border border-zinc-200 bg-white p-6 dark:border-zinc-800 dark:bg-zinc-900"
        onClick={(e) => e.stopPropagation()}
      >
        <h2
          id="file-detail-title"
          className="mb-4 text-xl font-semibold text-zinc-900 dark:text-zinc-100"
        >
          {t("files.viewDetail")}
        </h2>
        <div className="space-y-3 text-sm">
          <p className="font-medium text-zinc-800 dark:text-zinc-200">
            {fileName}
          </p>
          {dirPath ? (
            <p className="text-zinc-600 dark:text-zinc-400">
              {t("files.fromPath", { path: dirPath })}
            </p>
          ) : null}
          <p className="text-zinc-500 dark:text-zinc-400">
            {formatFileSize(file.size)}
            {file.fileType ? ` • ${file.fileType}` : ""}
          </p>
        </div>
        <div className="mt-6 flex justify-end">
          <button
            type="button"
            onClick={onClose}
            className="rounded bg-zinc-900 px-4 py-2 text-white hover:bg-zinc-800 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
          >
            {t("files.close")}
          </button>
        </div>
      </div>
    </div>
  );
}
