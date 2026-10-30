/** Where a published episode can be shared from the publish dialog.
 *
 *  X, Threads and Naver each have a share page that takes the address in its
 *  query, so the dialog opens it in the OS browser. KakaoTalk and Instagram
 *  have none a desktop app can open without a registered SDK key, so for
 *  those the dialog copies the text and the link for the writer to paste. */

export type ShareTarget = "x" | "threads" | "kakao" | "naver" | "instagram";

export const SHARE_TARGETS: readonly ShareTarget[] = ["x", "threads", "kakao", "naver", "instagram"];

export type ShareAction =
  | { kind: "open"; url: string }
  /** Copy `text`, then open `url` when there is somewhere to paste it. */
  | { kind: "copy"; text: string; url?: string };

/** What sharing `url` with `text` to `target` does. */
export function shareAction(target: ShareTarget, url: string, text: string): ShareAction {
  const q = (params: Record<string, string>) => new URLSearchParams(params).toString();
  switch (target) {
    case "x":
      return { kind: "open", url: `https://x.com/intent/post?${q({ text, url })}` };
    case "threads":
      return { kind: "open", url: `https://www.threads.com/intent/post?${q({ text, url })}` };
    case "naver":
      return { kind: "open", url: `https://share.naver.com/web/shareView?${q({ url, title: text })}` };
    case "kakao":
      return { kind: "copy", text: `${text}\n${url}` };
    case "instagram":
      return { kind: "copy", text: `${text}\n${url}`, url: "https://www.instagram.com/" };
  }
}
