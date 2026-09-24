/**
 * Comprehensive text unescaper and sanitizer for QQ/QZone messages, posts, and metadata.
 */

export function unescapeHtml(text: string): string {
  if (!text) return "";
  let s = String(text);

  // Numeric entities
  s = s.replace(/&#(\d+);/g, (_, dec) => {
    try {
      return String.fromCodePoint(Number(dec));
    } catch {
      return "";
    }
  });
  s = s.replace(/&#x([0-9a-fA-F]+);/g, (_, hex) => {
    try {
      return String.fromCodePoint(parseInt(hex, 16));
    } catch {
      return "";
    }
  });

  // Named entities
  const map: Record<string, string> = {
    "&quot;": '"',
    "&amp;": "&",
    "&apos;": "'",
    "&#39;": "'",
    "&#x27;": "'",
    "&#x2f;": "/",
    "&lt;": "<",
    "&gt;": ">",
    "&nbsp;": " ",
    "&ensp;": " ",
    "&emsp;": "　",
    "&thinsp;": " ",
    "&middot;": "·",
    "&mdash;": "—",
    "&copy;": "©",
    "&reg;": "®",
  };

  s = s.replace(/&(?:quot|amp|apos|lt|gt|nbsp|ensp|emsp|thinsp|middot|mdash|copy|reg|#39|#x27|#x2f);/gi, (match) => map[match.toLowerCase()] ?? match);

  return s;
}

export function cleanDisplayText(raw?: string): string {
  if (!raw && raw !== "0") return "";
  let text = String(raw);

  // Unescape HTML entities
  text = unescapeHtml(text);

  // Unescape literal JSON/C escapes: \\t, \\r, \\n, \\", \\\\
  text = text.replace(/\\t/g, " ");
  text = text.replace(/\\r/g, "");
  text = text.replace(/\\n/g, "\n");
  text = text.replace(/\\"/g, '"');
  text = text.replace(/\\\\/g, "\\");

  // QZone mentions: @{uin:123456,nick:张三,who:1} or @{uin:123456,nick:张三}
  text = text.replace(/@\{uin:\d+,\s*nick:([^,}]+)[^}]*\}/gi, "@$1 ");

  // QZone emotion codes: [em]e100[/em], [em]e10324[/em]
  text = text.replace(/\[em\]e\d+\[\/em\]/gi, "[表情]");

  // CQ codes (OneBot / NapCat)
  text = text
    .replace(/\[CQ:reply,id=\d+[^\]]*\]/gi, "[回复] ")
    .replace(/\[CQ:at,qq=all[^\]]*\]/gi, "@全体成员 ")
    .replace(/\[CQ:at,qq=(\d+)(?:,name=([^,\]]+))?[^\]]*\]/gi, (_, qq, name) => name ? `@${name} ` : `@${qq} `)
    .replace(/\[CQ:image,[^\]]*\]/gi, "[图片] ")
    .replace(/\[CQ:face,id=\d+[^\]]*\]/gi, "[表情] ")
    .replace(/\[CQ:record,[^\]]*\]/gi, "[语音] ")
    .replace(/\[CQ:video,[^\]]*\]/gi, "[视频] ")
    .replace(/\[CQ:share,[^\]]*title=([^,\]]+)[^\]]*\]/gi, "[分享: $1] ")
    .replace(/\[CQ:forward,[^\]]*\]/gi, "[合并转发] ")
    .replace(/\[CQ:json,[^\]]*\]/gi, "[卡片消息] ")
    .replace(/\[CQ:xml,[^\]]*\]/gi, "[卡片消息] ")
    .replace(/\[CQ:poke,qq=(\d+)[^\]]*\]/gi, "[戳一戳] ")
    .replace(/\[CQ:[^\]]+\]/gi, "[多媒体] ");

  // Normalize whitespace: replace tabs with space, remove line-end spaces
  text = text.replace(/\t+/g, " ");
  text = text.replace(/[ \f\v]+/g, " ");
  text = text.replace(/ *\n */g, "\n");
  text = text.replace(/\n{3,}/g, "\n\n");

  return text.trim();
}
