package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type WebSearchResultItem struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type WebSearchResponse struct {
	Query   string                `json:"query"`
	Total   int                   `json:"total"`
	Results []WebSearchResultItem `json:"results"`
}

var (
	tagRe       = regexp.MustCompile(`(?s)<a[^>]+class=['"]result-link['"][^>]*>`)
	linkRe      = regexp.MustCompile(`(?s)<a[^>]+class=['"]result-link['"][^>]*>(.*?)</a>`)
	hrefRe      = regexp.MustCompile(`href=['"]([^'"]+)['"]`)
	snippetRe   = regexp.MustCompile(`(?s)<td[^>]+class=['"]result-snippet['"][^>]*>(.*?)</td>`)
	stripTagsRe = regexp.MustCompile(`<[^>]+>`)
)

func cleanHTML(s string) string {
	s = stripTagsRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(s)
}

// SearchWeb performs a live internet web search for the given query.
func (s *Service) SearchWeb(ctx context.Context, query string, count int) (*WebSearchResponse, error) {
	if count <= 0 {
		count = 5
	}
	if count > 10 {
		count = 10
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return &WebSearchResponse{Query: "", Total: 0, Results: []WebSearchResultItem{}}, nil
	}

	var results []WebSearchResultItem

	// 1. If query is about weather, fetch direct real-time weather telemetry as the first result
	if strings.Contains(query, "天气") || strings.Contains(strings.ToLower(query), "weather") {
		weatherItem := s.searchLiveWeather(ctx, query)
		if weatherItem != nil {
			results = append(results, *weatherItem)
		}
	}

	// 2. Query DuckDuckGo Lite for full web search results
	ddgResults, err := s.searchDDGLite(ctx, query, count)
	if err == nil && len(ddgResults) > 0 {
		results = append(results, ddgResults...)
	}

	// 3. Fallback to Instant Answer if web results are empty
	if len(results) == 0 {
		fallbackResults, fallbackErr := s.searchInstantAnswer(ctx, query)
		if fallbackErr == nil && len(fallbackResults) > 0 {
			results = append(results, fallbackResults...)
		}
	}

	if len(results) > count {
		results = results[:count]
	}

	return &WebSearchResponse{
		Query:   query,
		Total:   len(results),
		Results: results,
	}, nil
}

func (s *Service) searchDDGLite(ctx context.Context, query string, maxCount int) ([]WebSearchResultItem, error) {
	formData := url.Values{}
	formData.Set("q", query)
	formData.Set("kl", "cn-zh")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://lite.duckduckgo.com/lite/", strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	body := string(bodyBytes)

	tags := tagRe.FindAllString(body, -1)
	linkMatches := linkRe.FindAllStringSubmatch(body, -1)
	snippetMatches := snippetRe.FindAllStringSubmatch(body, -1)

	var items []WebSearchResultItem
	for i := 0; i < len(linkMatches) && i < maxCount; i++ {
		title := cleanHTML(linkMatches[i][1])
		rawURL := ""
		if i < len(tags) {
			hrefM := hrefRe.FindStringSubmatch(tags[i])
			if len(hrefM) > 1 {
				rawURL = hrefM[1]
			}
		}
		snippet := ""
		if i < len(snippetMatches) {
			snippet = cleanHTML(snippetMatches[i][1])
		}
		if title != "" {
			items = append(items, WebSearchResultItem{
				Title:   title,
				URL:     rawURL,
				Snippet: snippet,
			})
		}
	}
	return items, nil
}

func (s *Service) searchLiveWeather(ctx context.Context, query string) *WebSearchResultItem {
	city := extractCity(query)
	if city == "" {
		city = "Beijing"
	}
	endpoint := fmt.Sprintf("https://wttr.in/%s?lang=zh&m&format=%%l:+%%C+%%t,+风向%%w,+湿度%%h", url.PathEscape(city))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "curl/8.0.0")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) == 0 {
		return nil
	}
	weatherInfo := strings.TrimSpace(string(data))
	if weatherInfo == "" || strings.Contains(weatherInfo, "Unknown location") || strings.Contains(weatherInfo, "<html>") {
		return nil
	}

	return &WebSearchResultItem{
		Title:   fmt.Sprintf("【实时气象监测】%s 实时天气数据", city),
		URL:     fmt.Sprintf("https://wttr.in/%s", city),
		Snippet: fmt.Sprintf("实时气象站数据：%s (更新于 %s)", weatherInfo, time.Now().Format("2006-01-02 15:04")),
	}
}

func extractCity(query string) string {
	cities := []string{
		"北京", "上海", "广州", "深圳", "杭州", "南京", "成都", "武汉", "重庆", "西安",
		"天津", "苏州", "长沙", "郑州", "青岛", "合肥", "福州", "厦门", "沈阳", "昆明",
		"大连", "济南", "哈尔滨", "长春", "石家庄", "南宁", "贵阳", "南昌", "太原", "海口",
		"三亚", "拉萨", "乌鲁木齐", "呼和浩特", "银川", "西宁", "兰州", "香港", "澳门", "台北",
	}
	for _, c := range cities {
		if strings.Contains(query, c) {
			return c
		}
	}
	return ""
}

func (s *Service) searchInstantAnswer(ctx context.Context, query string) ([]WebSearchResultItem, error) {
	endpoint := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1", url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SRA-Bot/1.0)")
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Heading  string `json:"Heading"`
		Abstract string `json:"Abstract"`
		URL      string `json:"AbstractURL"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	if data.Abstract != "" {
		return []WebSearchResultItem{
			{
				Title:   data.Heading,
				URL:     data.URL,
				Snippet: data.Abstract,
			},
		}, nil
	}
	return nil, nil
}
