"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useCallback, useEffect, useState } from "react";
import { FileDetailModal } from "./components/FileDetailModal";
import { LanguageSwitcher } from "./components/LanguageSwitcher";
import { useLanguage } from "./context/LanguageContext";
import { splitFilePath } from "./utils/filePathDisplay";
import { LuSendToBack } from "react-icons/lu";

interface FileInfo {
  id: string;
  fileName: string;
  size: number;
  fileType: string;
  sha256?: string;
  preview?: string;
}

interface PrepareDownloadResponse {
  info: {
    alias: string;
    version: string;
    deviceModel?: string;
    deviceType?: string;
    fingerprint: string;
    download?: boolean;
  };
  sessionId: string;
  files: Record<string, FileInfo>;
}

const FILES_PAGE_SIZE = 10;

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function DownloadContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { t } = useLanguage();
  const sessionId = searchParams.get("session") ?? searchParams.get("sessionId") ?? "";

  const [pin, setPin] = useState("");
  const [pinInput, setPinInput] = useState("");
  const [sessionInput, setSessionInput] = useState("");
  const [data, setData] = useState<PrepareDownloadResponse | null>(null);
  const [loading, setLoading] = useState(!!sessionId);
  const [error, setError] = useState<string | null>(null);
  const [needsPin, setNeedsPin] = useState(false);
  const [filePage, setFilePage] = useState(1);
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedFile, setSelectedFile] = useState<{
    fileId: string;
    file: FileInfo;
  } | null>(null);

  const fetchFileList = useCallback(
    async (pinValue?: string) => {
      if (!sessionId) {
        setError(t("error.missingSession"));
        setLoading(false);
        return;
      }

      setLoading(true);
      setError(null);
      setNeedsPin(false);

      try {
        let url: URL;
        if (process.env.NODE_ENV === "development") {
          url = new URL("/api/localsend/v2/prepare-download", "http://localhost:53317");
          } else {
          url = new URL("/api/localsend/v2/prepare-download", window.location.origin);
        }
        url.searchParams.set("sessionId", sessionId);
        if (pinValue) {
          url.searchParams.set("pin", pinValue);
        }
        

        const res = await fetch(url.toString(), { method: "GET" });
        const text = await res.text();

        if (res.status === 401) {
          const body = text ? JSON.parse(text) : {};
          const msg = body?.error ?? t("error.pinRequired");
          setNeedsPin(true);
          setError(msg);
          return;
        }

        if (!res.ok) {
          const body = text ? JSON.parse(text) : {};
          setError(body?.error ?? `${t("error.requestFailed")}: ${res.status}`);
          return;
        }

        const json: PrepareDownloadResponse = JSON.parse(text);
        setData(json);
        setPin(pinValue ?? "");
        setFilePage(1);
      } catch (err) {
        setError(err instanceof Error ? err.message : t("error.requestFailed"));
      } finally {
        setLoading(false);
      }
    },
    [sessionId, t]
  );

  useEffect(() => {
    if (sessionId) fetchFileList();
    else setLoading(false);
  }, [sessionId, fetchFileList]);

  useEffect(() => {
    setFilePage(1);
  }, [searchQuery]);

  const handlePinSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    fetchFileList(pinInput);
  };

  const getDownloadUrl = (fileId: string) => {
    if (!data?.sessionId) return "#";
    const url = new URL("/api/localsend/v2/download", window.location.origin);
    url.searchParams.set("sessionId", data.sessionId);
    url.searchParams.set("fileId", fileId);
    return url.toString();
  };

  if (!sessionId) {
    const handleSessionSubmit = (e: React.FormEvent) => {
      e.preventDefault();
      const value = sessionInput.trim();
      if (!value) return;
      const params = new URLSearchParams(searchParams.toString());
      params.set("session", value);
      router.replace(`/?${params.toString()}`);
    };

    return (
      <main className="flex min-h-screen flex-col items-center justify-center p-8">
        <div className="absolute right-6 top-6">
          <LanguageSwitcher />
        </div>

        <div className="Header text-2xl font-semibold my-10">
          <div className="flex flex-col items-center justify-center">
            <LuSendToBack
              className="w-18 h-18 cursor-pointer mb-2"
              onClick={() => router.push("/")}
            />
            <p className="text-center">Decky - Localsend Go Downloader</p>
          </div>
        </div>
        <div className="w-full max-w-md rounded-lg border border-zinc-200 bg-white p-8 dark:border-zinc-800 dark:bg-zinc-900">
          <h1 className="mb-2 text-xl font-semibold">{t("session.title")}</h1>
          <p className="mb-6 text-sm text-zinc-600 dark:text-zinc-400">
            {t("session.desc")}
          </p>
          <form onSubmit={handleSessionSubmit} className="flex flex-col gap-4">
            <input
              type="text"
              value={sessionInput}
              onChange={(e) => setSessionInput(e.target.value)}
              placeholder={t("session.placeholder")}
              className="rounded border border-zinc-300 px-4 py-2 dark:border-zinc-700 dark:bg-zinc-800 dark:text-white"
              autoFocus
            />
            <button
              type="submit"
              disabled={!sessionInput.trim()}
              className="rounded bg-zinc-900 px-4 py-2 text-white hover:bg-zinc-800 disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
            >
              {t("session.continue")}
            </button>
          </form>
        </div>
      </main>
    );
  }

  if (loading && !needsPin) {
    return (
      <main className="flex min-h-screen flex-col items-center justify-center p-8">
        <div className="absolute right-6 top-6">
          <LanguageSwitcher />
        </div>
        <div className="text-zinc-600 dark:text-zinc-400">{t("loading")}</div>
      </main>
    );
  }

  if (needsPin) {
    return (
      <main className="flex min-h-screen flex-col items-center justify-center p-8">
        <div className="absolute right-6 top-6">
          <LanguageSwitcher />
        </div>
        <div className="w-full max-w-md rounded-lg border border-zinc-200 bg-white p-8 dark:border-zinc-800 dark:bg-zinc-900">
          <h1 className="mb-4 text-xl font-semibold">{t("pin.title")}</h1>
          {error && (
            <p className="mb-4 text-sm text-red-600 dark:text-red-400">{error}</p>
          )}
          <form onSubmit={handlePinSubmit} className="flex flex-col gap-4">
            <input
              type="text"
              value={pinInput}
              onChange={(e) => setPinInput(e.target.value)}
              placeholder={t("pin.placeholder")}
              className="rounded border border-zinc-300 px-4 py-2 dark:border-zinc-700 dark:bg-zinc-800 dark:text-white"
              autoFocus
            />
            <button
              type="submit"
              className="rounded bg-zinc-900 px-4 py-2 text-white hover:bg-zinc-800 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
            >
              {t("pin.continue")}
            </button>
          </form>
        </div>
      </main>
    );
  }

  if (error && !data) {
    return (
      <main className="flex min-h-screen flex-col items-center justify-center p-8">
        <div className="absolute right-6 top-6">
          <LanguageSwitcher />
        </div>
        <div className="w-full max-w-md rounded-lg border border-zinc-200 bg-white p-8 dark:border-zinc-800 dark:bg-zinc-900">
          <h1 className="mb-4 text-xl font-semibold">{t("error.title")}</h1>
          <p className="text-red-600 dark:text-red-400">{error}</p>
          <button
            onClick={() => fetchFileList(pin || undefined)}
            className="mt-4 rounded bg-zinc-900 px-4 py-2 text-white hover:bg-zinc-800 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
          >
            {t("error.retry")}
          </button>
        </div>
      </main>
    );
  }

  if (!data) {
    return null;
  }

  const allFiles = Object.entries(data.files);
  const query = searchQuery.trim().toLowerCase();
  const filteredFiles =
    query === ""
      ? allFiles
      : allFiles.filter(([, file]) =>
          file.fileName.toLowerCase().includes(query)
        );
  const totalFiles = filteredFiles.length;
  const totalPages = Math.max(1, Math.ceil(totalFiles / FILES_PAGE_SIZE));
  const safePage = Math.min(Math.max(1, filePage), totalPages);
  const startIdx = (safePage - 1) * FILES_PAGE_SIZE;
  const pageFiles = filteredFiles.slice(
    startIdx,
    startIdx + FILES_PAGE_SIZE
  );
  const showPagination = totalFiles > FILES_PAGE_SIZE;
  const from = startIdx + 1;
  const to = Math.min(startIdx + FILES_PAGE_SIZE, totalFiles);

  return (
    <main className="flex min-h-screen flex-col items-center p-8">
      <div className="absolute right-6 top-6">
        <LanguageSwitcher />
      </div>
      <div className="w-full max-w-2xl">
        <h1 className="mb-2 text-2xl font-semibold">{t("files.title")}</h1>
        <p className="mb-4 text-sm text-zinc-600 dark:text-zinc-400">
          {t("files.from")} {data.info.alias}
        </p>

        <input
          type="text"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          placeholder={t("files.searchPlaceholder")}
          className="mb-4 w-full rounded border border-zinc-300 px-4 py-2 dark:border-zinc-700 dark:bg-zinc-800 dark:text-white"
          aria-label={t("files.searchPlaceholder")}
        />

        {showPagination && (
          <p className="mb-3 text-sm text-zinc-500 dark:text-zinc-400">
            {t("pagination.showing", {
              from,
              to,
              count: totalFiles,
            })}
          </p>
        )}

        <ul className="divide-y divide-zinc-200 dark:divide-zinc-700">
          {pageFiles.map(([fileId, file]) => {
            const { fileName: displayName } = splitFilePath(file.fileName);
            return (
              <li
                key={fileId}
                className="flex items-center justify-between gap-4 py-4"
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate font-medium">{displayName}</p>
                  <p className="text-sm text-zinc-500 dark:text-zinc-400">
                    {formatFileSize(file.size)}
                    {file.fileType && ` • ${file.fileType}`}
                  </p>
                </div>
                <div className="flex shrink-0 gap-2">
                  <button
                    type="button"
                    onClick={() => setSelectedFile({ fileId, file })}
                    className="rounded border border-zinc-300 px-3 py-1.5 text-sm text-zinc-700 hover:bg-zinc-50 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-300 dark:hover:bg-zinc-700"
                  >
                    {t("files.viewDetail")}
                  </button>
                  <a
                    href={getDownloadUrl(fileId)}
                    download={file.fileName}
                    className="rounded bg-zinc-900 px-4 py-2 text-sm text-white hover:bg-zinc-800 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
                  >
                    {t("files.download")}
                  </a>
                </div>
              </li>
            );
          })}
        </ul>

        {selectedFile && (
          <FileDetailModal
            file={selectedFile.file}
            onClose={() => setSelectedFile(null)}
          />
        )}

        {showPagination && (
          <div className="mt-6 flex flex-wrap items-center justify-between gap-3 border-t border-zinc-200 pt-4 dark:border-zinc-700">
            <span className="text-sm text-zinc-600 dark:text-zinc-400">
              {t("pagination.page", { page: safePage, total: totalPages })}
            </span>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => setFilePage((p) => Math.max(1, p - 1))}
                disabled={safePage <= 1}
                className="rounded border border-zinc-300 bg-white px-3 py-1.5 text-sm text-zinc-700 hover:bg-zinc-50 disabled:opacity-50 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-300 dark:hover:bg-zinc-700"
              >
                {t("pagination.prev")}
              </button>
              <button
                type="button"
                onClick={() =>
                  setFilePage((p) => Math.min(totalPages, p + 1))
                }
                disabled={safePage >= totalPages}
                className="rounded border border-zinc-300 bg-white px-3 py-1.5 text-sm text-zinc-700 hover:bg-zinc-50 disabled:opacity-50 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-300 dark:hover:bg-zinc-700"
              >
                {t("pagination.next")}
              </button>
            </div>
          </div>
        )}
      </div>
    </main>
  );
}

function LoadingFallback() {
  const { t } = useLanguage();
  return (
    <main className="flex min-h-screen flex-col items-center justify-center p-8">
      <div className="text-zinc-600 dark:text-zinc-400">{t("loading")}</div>
    </main>
  );
}

export default function Home() {
  return (
    <Suspense fallback={<LoadingFallback />}>
      <DownloadContent />
    </Suspense>
  );
}
