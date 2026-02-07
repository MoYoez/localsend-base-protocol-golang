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
      className="fixed inset-0 z-50 flex items-end justify-center bg-black/50 p-0 sm:items-center sm:p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="file-detail-title"
      onClick={onClose}
    >
      <div
        className="w-full max-h-[85vh] overflow-y-auto rounded-t-2xl border border-b-0 border-zinc-200 bg-white p-5 shadow-lg dark:border-zinc-800 dark:border-b-0 dark:bg-zinc-900 sm:max-h-none sm:rounded-2xl sm:border-b sm:p-6 sm:shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2
          id="file-detail-title"
          className="mb-4 text-lg font-semibold text-zinc-900 dark:text-zinc-100 sm:text-xl"
        >
          {t("files.viewDetail")}
        </h2>
        <div className="space-y-3 text-sm">
          <p className="break-words font-medium text-zinc-800 dark:text-zinc-200">
            {fileName}
          </p>
          {dirPath ? (
            <p className="break-all text-zinc-600 dark:text-zinc-400">
              {t("files.fromPath", { path: dirPath })}
            </p>
          ) : null}
          <p className="text-zinc-500 dark:text-zinc-400">
            {formatFileSize(file.size)}
            {file.fileType ? ` • ${file.fileType}` : ""}
          </p>
        </div>
        <div className="mt-6 flex justify-end sm:mt-6">
          <button
            type="button"
            onClick={onClose}
            className="min-h-[48px] min-w-[120px] rounded-xl bg-zinc-900 px-5 py-3 text-base font-medium text-white transition-colors hover:bg-zinc-800 active:bg-zinc-700 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200 dark:active:bg-zinc-300 sm:min-h-0 sm:min-w-0 sm:rounded-lg sm:px-4 sm:py-2 sm:text-sm"
          >
            {t("files.close")}
          </button>
        </div>
      </div>
    </div>
  );
}
