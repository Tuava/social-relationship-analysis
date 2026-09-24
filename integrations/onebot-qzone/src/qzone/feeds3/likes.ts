/**
 * feeds3 点赞提取
 */

import { log, htmlUnescape } from '../utils.js';
import { preprocessHtml } from './preprocess.js';
import { collectT1TidRefs } from './tidParams.js';
import { canonicalPostTidFromFeedAttrs } from './feedDataCanonical.js';

/** feeds3 HTML 中解析出来的单条点赞 */
export interface Feeds3Like {
  uin: string;
  nickname: string;
  tid: string;
  ownerUin: string;
  abstime: number;
  customItemId: string;
  _source: 'feeds3_html';
}

function parseSinglePostLikes(
  html: string,
  tid: string,
  ownerUin: string,
): Feeds3Like[] {
  const likes: Feeds3Like[] = [];
  const userListMatch = html.match(/class="user-list"[^>]*>([\s\S]*?)<\/div>/i);
  if (!userListMatch) return likes;

  const userListHtml = userListMatch[1];
  const linkPattern = /<a[^>]*href="http:\/\/user\.qzone\.qq\.com\/(\d+)"[^>]*>([\s\S]*?)<\/a>/g;
  let m: RegExpExecArray | null;

  while ((m = linkPattern.exec(userListHtml)) !== null) {
    const uin = m[1];
    const innerHtml = m[2];
    const nickname = htmlUnescape(innerHtml.replace(/<[^>]+>/g, '').replace(/、/g, '').trim());

    if (uin && nickname) {
      likes.push({
        uin,
        nickname,
        tid,
        ownerUin,
        abstime: 0,
        customItemId: '',
        _source: 'feeds3_html',
      });
    }
  }

  return likes;
}

/**
 * 校验 user-list 所在区块是否确实属于目标 postTid（防串帖）
 */
function likeBlockMatchesPost(fullBlock: string, fixedPostTid: string): boolean {
  const re = /t1_tid=([^&"'<>\s]+)/gi;
  const t1s: string[] = [];
  let m: RegExpExecArray | null;
  while ((m = re.exec(fullBlock)) !== null) {
    const v = m[1]!.trim();
    if (v && !t1s.includes(v)) t1s.push(v);
  }
  if (t1s.length === 0) return true;
  const anchorLooksReal = (t: string) =>
    /^[a-f0-9]{16,}$/i.test(t) || /^d\d+_\d+_/i.test(t);
  const relevant = t1s.filter(anchorLooksReal);
  if (relevant.length === 0) return true;
  const uniq = [...new Set(relevant)];
  const hits = uniq.filter((t) => t === fixedPostTid);
  if (hits.length === 0) return false;
  if (uniq.length > 1 && uniq.some((t) => t !== fixedPostTid)) return false;
  return true;
}

/**
 * 从 feeds3 HTML 中提取点赞详情。
 */
export function parseFeeds3Likes(
  text: string,
  processedText?: string,
): Map<string, Feeds3Like[]> {
  const result = new Map<string, Feeds3Like[]>();
  const html = processedText ?? preprocessHtml(text).text;

  const tidPositions = collectT1TidRefs(html);

  // 1. 针对每一个 user-list 进行独立定位与卡片容器归属判定
  const userListPattern = /<div\s+class="user-list"[^>]*>([\s\S]*?)<\/div>/gi;
  let ulm: RegExpExecArray | null;

  while ((ulm = userListPattern.exec(html)) !== null) {
    const ulIndex = ulm.index;
    const ulInner = ulm[1]!;

    // 找到该 user-list 前方最近的 t1_tid
    const prevTids = tidPositions.filter((r) => r.index < ulIndex);
    if (prevTids.length === 0) continue;
    const closestTid = prevTids[prevTids.length - 1]!;

    // 检查二者距离：如果相隔超过 6000 字符，通常已跨越多个无关卡片
    if (ulIndex - closestTid.index > 6000) continue;

    // 检查中间是否跨越了其它独立的 feed 卡片起始标记（<li class="f-single 或 <div class="f-item）
    const intermediate = html.slice(closestTid.index, ulIndex);
    const feedBoundaryMatch = intermediate.match(/<li\s+class="f-single|<div\s+class="f-item/i);
    // 如果在 t1_tid 与 user-list 之间出现了新的 feed 起始，说明中间换帖了，该 user-list 不属于 closestTid
    if (feedBoundaryMatch && feedBoundaryMatch.index! > 50) continue;

    const block = html.slice(closestTid.index, ulIndex + ulm[0].length + 200);
    if (!likeBlockMatchesPost(block, closestTid.postTid)) continue;

    const ownerMatch = html.slice(Math.max(0, closestTid.index - 200), ulIndex).match(/t1_uin=(\d+)/);
    const ownerUin = ownerMatch?.[1] ?? '';
    const likes = parseSinglePostLikes(ulm[0], closestTid.postTid, ownerUin);
    if (likes.length > 0) {
      if (!result.has(closestTid.postTid)) result.set(closestTid.postTid, []);
      for (const l of likes) {
        if (!result.get(closestTid.postTid)!.some(existing => existing.uin === l.uin)) {
          result.get(closestTid.postTid)!.push(l);
        }
      }
    }
  }

  const feedItemPat = /<div\s+class="f-item[^"]*f-item-passive"\s+id="feed_(\d+)_(\d+)_(\d+)_(\d+)_\d+_\d+"[\s\S]*?name="feed_data"\s*([^>]*)>[\s\S]*?(?=<div\s+class="f-item|<\/ul>|$)/g;
  const dataAttr = (attrs: string, name: string): string => {
    const m = attrs.match(new RegExp(`data-${name}="([^"]*)"`));
    return m?.[1] ?? '';
  };

  let fm: RegExpExecArray | null;
  while ((fm = feedItemPat.exec(html)) !== null) {
    const feedDataAttrs = fm[5]!;
    const feedstype = dataAttr(feedDataAttrs, 'feedstype');
    if (feedstype !== '101') continue;

    const likerUin = fm[1]!;
    const tidRaw = dataAttr(feedDataAttrs, 'tid');
    const ownerUin = dataAttr(feedDataAttrs, 'uin');
    const abstime = parseInt(dataAttr(feedDataAttrs, 'abstime') || '0', 10);
    if (!tidRaw || !likerUin || likerUin === '0') continue;

    const searchBeforeWide = html.substring(Math.max(0, fm.index - 8000), fm.index);
    const searchAfterHead = fm[0].slice(0, 4000);
    const tid =
      canonicalPostTidFromFeedAttrs(feedDataAttrs, searchBeforeWide, searchAfterHead) || tidRaw;

    const blockStart = Math.max(0, fm.index - 600);
    const preceding = html.substring(blockStart, fm.index);
    let nickname = '';
    const nickMatch = preceding.match(/link="nameCard_\d+"[^>]*>([^<]+)<\/a>/);
    if (nickMatch) nickname = htmlUnescape(nickMatch[1]!.trim());

    const blockContent = fm[0];
    const customMatch = blockContent.match(/data-custom_itemid="(\d+)"/);
    const customItemId = customMatch?.[1] ?? '';

    const like: Feeds3Like = {
      uin: likerUin,
      nickname,
      tid,
      ownerUin,
      abstime,
      customItemId,
      _source: 'feeds3_html',
    };

    if (!result.has(tid)) result.set(tid, []);
    const existing = result.get(tid)!.find(l => l.uin === likerUin);
    if (!existing) result.get(tid)!.push(like);
  }

  for (const [tid, likes] of result) {
    const byUin = new Map<string, Feeds3Like>();
    for (const like of likes) {
      const existing = byUin.get(like.uin);
      if (!existing || like.abstime > existing.abstime) byUin.set(like.uin, like);
    }
    result.set(tid, [...byUin.values()]);
  }

  const total = [...result.values()].reduce((s, a) => s + a.length, 0);
  log('DEBUG', `parseFeeds3Likes: found ${total} likes for ${result.size} posts`);
  return result;
}
