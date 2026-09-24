import { chromium } from 'playwright';
import urllib from 'node:http';

async function getCookiesFromNapcat(): Promise<string> {
  return new Promise((resolve, reject) => {
    const req = urllib.request('http://127.0.0.1:3000/get_cookies', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': 'Bearer token'
      }
    }, (res) => {
      let data = '';
      res.on('data', chunk => data += chunk);
      res.on('end', () => {
        try {
          const json = JSON.parse(data);
          resolve(json?.data?.cookies || '');
        } catch (e) {
          reject(e);
        }
      });
    });
    req.on('error', reject);
    req.write(JSON.stringify({ domain: 'user.qzone.qq.com' }));
    req.end();
  });
}

async function main() {
  const cookieStr = await getCookiesFromNapcat();
  const browser = await chromium.launch({
    headless: true,
    executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
  });
  const context = await browser.newContext({
    userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36'
  });

  const cookies: any[] = [];
  for (const item of cookieStr.split(';')) {
    const [k, ...v] = item.trim().split('=');
    if (k && v.length) {
      cookies.push({ name: k.trim(), value: v.join('=').trim(), domain: '.qq.com', path: '/' });
      cookies.push({ name: k.trim(), value: v.join('=').trim(), domain: '.qzone.qq.com', path: '/' });
    }
  }
  await context.addCookies(cookies);

  const page = await context.newPage();

  page.on('response', async (response) => {
    const url = response.url();
    if (url.includes('like') || url.includes('cgi')) {
      try {
        const text = await response.text();
        if (text.includes('portrait') || text.includes('fuin') || text.includes('like_uin')) {
          console.log(`[LIKE LIST HIT] ${url.substring(0, 100)}`);
          console.log(text.substring(0, 1000));
        }
      } catch (_) {}
    }
  });

  const targetQQ = process.argv[2];
  if (!targetQQ || !/^\d+$/.test(targetQQ)) throw new Error('Provide a target QQ as the first argument');
  await page.goto(`https://user.qzone.qq.com/${targetQQ}`, { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForTimeout(4000);

  const res = await page.evaluate(async () => {
    return new Promise((resolve) => {
      if ((globalThis as any).QZONE && (globalThis as any).QZONE.FP && (globalThis as any).QZONE.FP.getLikeList) {
        console.log('QZONE.FP.getLikeList found!');
        resolve('QZONE.FP.getLikeList exists: ' + (globalThis as any).QZONE.FP.getLikeList.toString().substring(0, 1000));
      } else {
        resolve('QZONE.FP.getLikeList not found on window');
      }
    });
  });
  console.log('[EVAL RESULT]', res);

  await browser.close();
}

main().catch(console.error);
