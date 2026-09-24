import { parseFeeds3Likes } from '../../src/qzone/feeds3/likes.js';
import { assert, runSuite, type TestCase } from '../test-helpers.js';

const cases: TestCase[] = [
  {
    name: '不应将后续帖子的点赞泄漏给前面的 0 赞说说',
    fn: () => {
      const multiFeedHtml = `
        <ul id="feed_friend_list">
          <!-- Feed A: mikoto 的无赞动态 -->
          <li class="f-single f-s-s" id="feed_1714610311_87e032666d9ab16935d10a00">
            <div class="f-item">
              <a href="http://user.qzone.qq.com/1714610311/mood/87e032666d9ab16935d10a00" data-param="t1_uin=1714610311&t1_tid=87e032666d9ab16935d10a00">说说A</a>
              <div class="comments-list"></div>
            </div>
          </li>

          <!-- Feed B: 其他好友带 3 个点赞的动态 -->
          <li class="f-single f-s-s" id="feed_999999999_post_with_likes_12345678">
            <div class="f-item">
              <a href="http://user.qzone.qq.com/999999999/mood/post_with_likes_12345678" data-param="t1_uin=999999999&t1_tid=post_with_likes_12345678">说说B</a>
              <div class="user-list">
                <a href="http://user.qzone.qq.com/10000001">群老吴</a>、
                <a href="http://user.qzone.qq.com/3466128351">秋风送落叶</a>、
                <a href="http://user.qzone.qq.com/77632263">余念安</a>
              </div>
            </div>
          </li>
        </ul>
      `;

      const result = parseFeeds3Likes(multiFeedHtml);

      // Feed A 必须为 0 点赞
      const likesA = result.get('87e032666d9ab16935d10a00') ?? [];
      assert(likesA.length === 0, `Feed A 点赞数必须为 0，实际得到: ${likesA.length}`);

      // Feed B 必须完整拿到 3 个点赞
      const likesB = result.get('post_with_likes_12345678') ?? [];
      assert(likesB.length === 3, `Feed B 点赞数必须为 3，实际得到: ${likesB.length}`);
      assert(likesB[0].uin === '10000001', '首个点赞人必须为群老吴');
    },
  },
];

export async function run() {
  return runSuite('feeds3-likes-scope', cases);
}
