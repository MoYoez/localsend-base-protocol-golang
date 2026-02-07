export type Locale = "zh" | "en";

export const messages: Record<Locale, Record<string, string>> = {
  zh: {
    "nav.download": "下载",
    "session.title": "下载",
    "session.desc": "请输入发送方分享链接中的会话 ID，以查看并下载文件。",
    "session.placeholder": "会话 ID",
    "session.continue": "继续",
    "loading": "加载中...",
    "pin.title": "输入 PIN",
    "pin.placeholder": "PIN",
    "pin.continue": "继续",
    "error.title": "错误",
    "error.retry": "重试",
    "error.missingSession": "缺少会话 ID，请使用发送方提供的分享链接。",
    "error.pinRequired": "需要 PIN",
    "error.requestFailed": "请求失败",
    "files.title": "下载文件",
    "files.from": "来自",
    "files.download": "下载",
    "files.viewDetail": "查看文件详情",
    "files.fromPath": "来自 {path}",
    "files.searchPlaceholder": "搜索文件名或路径",
    "files.close": "关闭",
    "pagination.prev": "上一页",
    "pagination.next": "下一页",
    "pagination.page": "第 {page} 页，共 {total} 页",
    "pagination.showing": "显示 {from}-{to}，共 {count} 个文件",
    "lang.zh": "中文",
    "lang.en": "English",
  },
  en: {
    "nav.download": "Download",
    "session.title": "Download",
    "session.desc": "Enter the session ID from the sender's share link to view and download files.",
    "session.placeholder": "Session ID",
    "session.continue": "Continue",
    "loading": "Loading...",
    "pin.title": "Enter PIN",
    "pin.placeholder": "PIN",
    "pin.continue": "Continue",
    "error.title": "Error",
    "error.retry": "Retry",
    "error.missingSession": "Missing session ID. Please use the share link provided by the sender.",
    "error.pinRequired": "PIN required",
    "error.requestFailed": "Request failed",
    "files.title": "Download Files",
    "files.from": "From",
    "files.download": "Download",
    "files.viewDetail": "View file details",
    "files.fromPath": "From {path}",
    "files.searchPlaceholder": "Search by file name or path",
    "files.close": "Close",
    "pagination.prev": "Previous",
    "pagination.next": "Next",
    "pagination.page": "Page {page} of {total}",
    "pagination.showing": "Showing {from}-{to} of {count} files",
    "lang.zh": "中文",
    "lang.en": "English",
  },
};

export function getLocaleFromHeader(acceptLanguage: string | null): Locale {
  if (!acceptLanguage) return "en";
  const first = acceptLanguage.split(",")[0]?.toLowerCase() ?? "";
  if (first.startsWith("zh")) return "zh";
  return "en";
}
