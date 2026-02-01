"use client";

import { useSearchParams } from "next/navigation";
import { Suspense, useCallback, useEffect, useState } from "react";

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

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function DownloadContent() {
  const searchParams = useSearchParams();
  const sessionId = searchParams.get("session") ?? searchParams.get("sessionId") ?? "";

  const [pin, setPin] = useState("");
  const [pinInput, setPinInput] = useState("");
  const [data, setData] = useState<PrepareDownloadResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [needsPin, setNeedsPin] = useState(false);

  const fetchFileList = useCallback(
    async (pinValue?: string) => {
      if (!sessionId) {
        setError("Missing session ID. Please use the share link provided by the sender.");
        setLoading(false);
        return;
      }

      setLoading(true);
      setError(null);
      setNeedsPin(false);

      try {
        const url = new URL("/api/localsend/v2/prepare-download", window.location.origin);
        url.searchParams.set("sessionId", sessionId);
        if (pinValue) {
          url.searchParams.set("pin", pinValue);
        }

        const res = await fetch(url.toString(), { method: "POST" });
        const text = await res.text();

        if (res.status === 401) {
          const body = text ? JSON.parse(text) : {};
          const msg = body?.error ?? "PIN required";
          setNeedsPin(true);
          setError(msg);
          return;
        }

        if (!res.ok) {
          const body = text ? JSON.parse(text) : {};
          setError(body?.error ?? `Request failed: ${res.status}`);
          return;
        }

        const json: PrepareDownloadResponse = JSON.parse(text);
        setData(json);
        setPin(pinValue ?? "");
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to fetch file list");
      } finally {
        setLoading(false);
      }
    },
    [sessionId]
  );

  useEffect(() => {
    fetchFileList();
  }, [fetchFileList]);

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
    return (
      <main className="flex min-h-screen flex-col items-center justify-center p-8">
        <div className="w-full max-w-md rounded-lg border border-zinc-200 bg-white p-8 dark:border-zinc-800 dark:bg-zinc-900">
          <h1 className="mb-4 text-xl font-semibold">Download</h1>
          <p className="text-zinc-600 dark:text-zinc-400">
            Missing session ID. Please use the share link provided by the sender.
          </p>
        </div>
      </main>
    );
  }

  if (loading && !needsPin) {
    return (
      <main className="flex min-h-screen flex-col items-center justify-center p-8">
        <div className="text-zinc-600 dark:text-zinc-400">Loading...</div>
      </main>
    );
  }

  if (needsPin) {
    return (
      <main className="flex min-h-screen flex-col items-center justify-center p-8">
        <div className="w-full max-w-md rounded-lg border border-zinc-200 bg-white p-8 dark:border-zinc-800 dark:bg-zinc-900">
          <h1 className="mb-4 text-xl font-semibold">Enter PIN</h1>
          {error && (
            <p className="mb-4 text-sm text-red-600 dark:text-red-400">{error}</p>
          )}
          <form onSubmit={handlePinSubmit} className="flex flex-col gap-4">
            <input
              type="text"
              value={pinInput}
              onChange={(e) => setPinInput(e.target.value)}
              placeholder="PIN"
              className="rounded border border-zinc-300 px-4 py-2 dark:border-zinc-700 dark:bg-zinc-800 dark:text-white"
              autoFocus
            />
            <button
              type="submit"
              className="rounded bg-zinc-900 px-4 py-2 text-white hover:bg-zinc-800 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
            >
              Continue
            </button>
          </form>
        </div>
      </main>
    );
  }

  if (error && !data) {
    return (
      <main className="flex min-h-screen flex-col items-center justify-center p-8">
        <div className="w-full max-w-md rounded-lg border border-zinc-200 bg-white p-8 dark:border-zinc-800 dark:bg-zinc-900">
          <h1 className="mb-4 text-xl font-semibold">Error</h1>
          <p className="text-red-600 dark:text-red-400">{error}</p>
          <button
            onClick={() => fetchFileList(pin || undefined)}
            className="mt-4 rounded bg-zinc-900 px-4 py-2 text-white hover:bg-zinc-800 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
          >
            Retry
          </button>
        </div>
      </main>
    );
  }

  if (!data) {
    return null;
  }

  const files = Object.entries(data.files);

  return (
    <main className="flex min-h-screen flex-col items-center p-8">
      <div className="w-full max-w-2xl">
        <h1 className="mb-2 text-2xl font-semibold">Download Files</h1>
        <p className="mb-6 text-sm text-zinc-600 dark:text-zinc-400">
          From {data.info.alias}
        </p>

        <ul className="divide-y divide-zinc-200 dark:divide-zinc-700">
          {files.map(([fileId, file]) => (
            <li
              key={fileId}
              className="flex items-center justify-between gap-4 py-4"
            >
              <div className="min-w-0 flex-1">
                <p className="truncate font-medium">{file.fileName}</p>
                <p className="text-sm text-zinc-500 dark:text-zinc-400">
                  {formatFileSize(file.size)}
                  {file.fileType && ` • ${file.fileType}`}
                </p>
              </div>
              <a
                href={getDownloadUrl(fileId)}
                download={file.fileName}
                className="shrink-0 rounded bg-zinc-900 px-4 py-2 text-sm text-white hover:bg-zinc-800 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-200"
              >
                Download
              </a>
            </li>
          ))}
        </ul>
      </div>
    </main>
  );
}

export default function Home() {
  return (
    <Suspense
      fallback={
        <main className="flex min-h-screen flex-col items-center justify-center p-8">
          <div className="text-zinc-600 dark:text-zinc-400">Loading...</div>
        </main>
      }
    >
      <DownloadContent />
    </Suspense>
  );
}
