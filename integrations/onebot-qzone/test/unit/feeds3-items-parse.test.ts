/**
 * parseFeeds3Items：真实页面中 id="feed_*" 首段 uin 常为内部映射，与 data-uin 不一致。
 */
import { parseFeeds3Items } from '../../src/qzone/feeds3/items.js';
import { assert, runSuite, type TestCase } from '../test-helpers.js';

/** feed_ 用「内部 uin」3628420742；data-uin 为展示 QQ 2492835361 */
const INTERNAL_UIN_MISMATCH_FIXTURE = `<li class="f-single"><div id="feed_3628420742_311_0_1774000000_1_1"><div class="qz_summary wupfeed"><i class="none" name="feed_data" data-tid="1774000000" data-uin="2492835361" data-abstime="1774000000" data-fkey="testfkey123456789012345"></i><a class="f-name" href="#">昵称</a><div class="txt-box-title"><p>昵称：正文测试</p></div></div></div></li>`;

/** app 分享（如 202）：正文内有 `f-name info` 播放量行，发表者只在 feed_data 上方的 f-nick */
const APP_SHARE_F_NICK_FIXTURE = `<li class="f-single"><div class="user-info"><div class="f-nick"><a class="f-name q_namecard" link="nameCard_3264584080">依鸣</a></div></div><div id="feed_3264584080_202_2_1774139725_0_1"><div class="qz_summary wupfeed"><i class="none" name="feed_data" data-tid="1774139725" data-uin="3264584080" data-abstime="1774139725" data-fkey="fchjdbehhb"></i><div class="txt-box"><h4 class="txt-box-title"><a class="c_tx">DECO*27 - テレパシ</a></h4><a class="f-name info state">689万播放 · 点赞</a></div></div></div></li>`;

/** 两人：外层 f-nick 为互动者 A；正文 nameCard + data-uin 为说说主人 B（与 A 不同号，模拟 mergeData 外层 opuin 与正文不一致） */
const TXT_BOX_OWNER_VS_OUTER_INTERACTOR = `<li class="f-single"><div class="f-single-head"><div class="f-nick"><a class="f-name q_namecard" link="nameCard_2336135301">pacebot</a><span>转发了</span></div></div><div id="feed_2336135301_311_2_1774144823_1_1"><div class="qz_summary wupfeed"><i class="none" name="feed_data" data-tid="tidhex001" data-uin="2492835361" data-abstime="1774144823" data-fkey="tidhex001"></i><div class="txt-box one-line"><p class="txt-box-title ellipsis-one"><a class="nickname name c_tx q_namecard" link="nameCard_2492835361">peacebot</a><span>：</span>你是谁？请支持依鸣</p></div><div class="mod-comments"><ul><li class="comments-item bor3" data-type="commentroot"></li><li class="comments-item bor3" data-type="replyroot"></li></ul></div></div></div></li>`;

/** 当前 feed_data 之前的卡片含图时，不能把上一条图带进当前动态。 */
const MEDIA_SCOPE_FIXTURE = `
<li class="f-single"><div class="user-info"><div class="f-nick"><a class="f-name" link="nameCard_1714610311">mikoto</a></div></div>
  <div id="feed_1714610311_311_0_1784451685_0_1"><div class="qz_summary"><i name="feed_data" data-tid="mediaone001" data-uin="1714610311" data-abstime="1784451685" data-fkey="mediaone001"></i>
    <div class="f-ct"><a class="img-item" data-pickey="mediaone001,https://a1.qpic.cn/first.jpg"></a></div>
  </div></div>
</li>
<li class="f-single"><div class="user-info"><div class="f-nick"><a class="f-name" link="nameCard_1714610311">mikoto</a></div></div>
  <div id="feed_1714610311_311_0_1783783553_0_1"><div class="qz_summary"><i name="feed_data" data-tid="mediatwo002" data-uin="1714610311" data-abstime="1783783553" data-fkey="mediatwo002"></i>
    <div class="f-ct"><a class="img-item" data-pickey="mediatwo002,https://photo.store.qq.com/second.jpg"></a></div>
  </div></div>
</li>
<li class="f-single"><div class="user-info"><div class="f-nick"><a class="f-name" link="nameCard_1714610311">mikoto</a></div></div>
  <div id="feed_1714610311_311_0_1782153983_0_1"><div class="qz_summary"><i name="feed_data" data-tid="mediathree003" data-uin="1714610311" data-abstime="1782153983" data-fkey="mediathree003"></i>
    <div class="f-ct"><img src="http://qzonestyle.gtimg.cn/qzone_v6/img/feed/gift-default.jpg"></div>
  </div></div>
</li>`;

const cases: TestCase[] = [
  {
    name: 'parseFeeds3Items: feed_ 内部 uin ≠ data-uin 时仍按 data-uin 过滤成功',
    fn: () => {
      const items = parseFeeds3Items(INTERNAL_UIN_MISMATCH_FIXTURE, '2492835361', undefined, 10, false);
      assert(items.length === 1, `应解析 1 条，实际 ${items.length}`);
      assert(String(items[0]!.uin) === '2492835361', 'item.uin 应为 data-uin');
      assert(String(items[0]!.content ?? '').includes('正文测试'), '应提取正文');
      assert(String(items[0]!.tid) === 'testfkey123456789012345', '数字 tid 应归一为 fkey');
    },
  },
  {
    name: 'parseFeeds3Items: app 分享优先 f-nick 发表者，不被 f-name info 播放量行带偏',
    fn: () => {
      const items = parseFeeds3Items(APP_SHARE_F_NICK_FIXTURE, undefined, undefined, 10, false);
      assert(items.length === 1, `应解析 1 条，实际 ${items.length}`);
      assert(String(items[0]!.uin) === '3264584080', 'uin 应为 f-nick nameCard');
      assert(String(items[0]!.opuin) === '3264584080', 'opuin 应与 uin 一致');
      assert(String(items[0]!.nickname) === '依鸣', `nickname 应为 f-nick，实际 ${items[0]!.nickname}`);
      assert(!String(items[0]!.nickname ?? '').includes('播放'), '昵称不应是播放量文案');
    },
  },
  {
    name: 'parseFeeds3Items: 正文 nameCard 与 data-uin 一致时以说说主人为准（外层 f-nick 可能是他人）；cmtnum 从 mod-comments 统计',
    fn: () => {
      const items = parseFeeds3Items(TXT_BOX_OWNER_VS_OUTER_INTERACTOR, undefined, undefined, 10, false);
      assert(items.length === 1, `应解析 1 条，实际 ${items.length}`);
      assert(String(items[0]!.uin) === '2492835361', 'item.uin 应为 data-uin / 正文发表者 B');
      assert(String(items[0]!.nickname) === 'peacebot', `nickname 应为正文发表者 B（peacebot），实际 ${items[0]!.nickname}`);
      assert(Number(items[0]!.cmtnum) === 2, `cmtnum 应为 2（2 条 comments-item），实际 ${items[0]!.cmtnum}`);
    },
  },
  {
    name: 'parseFeeds3Items: 媒体只属于当前 feed，且装饰占位图不计入动态资源',
    fn: () => {
      const items = parseFeeds3Items(MEDIA_SCOPE_FIXTURE, '1714610311', undefined, 10, false);
      assert(items.length === 3, `应解析 3 条，实际 ${items.length}`);
      const first = items.find((item) => item.tid === 'mediaone001');
      const second = items.find((item) => item.tid === 'mediatwo002');
      const third = items.find((item) => item.tid === 'mediathree003');
      assert(Array.isArray(first?.pic) && first.pic.length === 1 && String(first.pic[0].url).includes('first.jpg'), '第一条应只包含第一张图片');
      assert(Array.isArray(second?.pic) && second.pic.length === 1 && String(second.pic[0].url).includes('second.jpg'), '第二条不应继承第一条图片');
      assert(Array.isArray(third?.pic) && third.pic.length === 0, '占位图不应作为动态图片');
    },
  },
];

export async function run() {
  return runSuite('feeds3/items-parse', cases);
}
