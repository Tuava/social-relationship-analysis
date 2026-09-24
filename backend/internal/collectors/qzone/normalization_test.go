package qzone

import (
	"testing"
	"time"
)

func TestFindMapSliceHandlesNestedData(t *testing.T) {
	value := map[string]any{"data": map[string]any{"commentlist": []any{map[string]any{"uin": "123"}}}}
	items := findMapSlice(value, "commentlist", "comments")
	if len(items) != 1 || firstText(items[0], "uin") != "123" {
		t.Fatalf("items = %#v", items)
	}
}

func TestContentMediaExtractsAndDeduplicates(t *testing.T) {
	item := map[string]any{
		"pic":    []any{map[string]any{"url3": "https://example.qpic.cn/a.jpg"}, "https://example.qpic.cn/a.jpg"},
		"videos": []any{map[string]any{"video_url": "https://example.qpic.cn/a.mp4"}},
	}
	refs := contentMedia(item, false)
	if len(refs) != 2 || refs[0].kind != "image" || refs[0].resolver != "qzone:/fetch_image" || refs[1].kind != "video" {
		t.Fatalf("refs = %#v", refs)
	}
}

func TestContentMediaCommentPicOnlyForComments(t *testing.T) {
	base := map[string]any{
		"pic":          []any{map[string]any{"url3": "https://example.qpic.cn/post.jpg"}},
		"_comment_pic": []any{map[string]any{"url3": "https://example.qpic.cn/comment.jpg"}},
	}
	postRefs := contentMedia(base, false)
	for _, ref := range postRefs {
		if ref.url != "https://example.qpic.cn/post.jpg" {
			t.Fatalf("post refs must not include comment picture, got %#v", postRefs)
		}
	}
	commentRefs := contentMedia(base, true)
	var found bool
	for _, ref := range commentRefs {
		if ref.url == "https://example.qpic.cn/comment.jpg" {
			found = true
		}
	}
	if !found {
		t.Fatalf("comment refs should include _comment_pic, got %#v", commentRefs)
	}
}

func TestMediaIdentityIgnoresQZoneSignatureRefresh(t *testing.T) {
	first := "https://a1.qpic.cn/psc?/stable-key&dis_t=1&dis_k=old"
	second := "https://a1.qpic.cn/psc?/stable-key&dis_t=2&dis_k=new"
	if mediaIdentity(first) != mediaIdentity(second) {
		t.Fatalf("identities differ: %q vs %q", mediaIdentity(first), mediaIdentity(second))
	}
}

func TestInteractionContextPreservesParentPost(t *testing.T) {
	post := normalizedPost{ContentID: "content-id", TID: "tid-1", AuthorQQ: "123"}
	item := withInteractionContext(map[string]any{"commentid": "c1"}, post)
	if item["_post_content_id"] != post.ContentID || item["_post_tid"] != post.TID || item["commentid"] != "c1" {
		t.Fatalf("context = %#v", item)
	}
}

func TestEventTimeSupportsMilliseconds(t *testing.T) {
	got := eventTimeFrom(map[string]any{"created_time": float64(1_700_000_000_000)})
	want := time.Unix(1_700_000_000, 0)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestEventTimeSupportsCreateTimeKeyAndFormats(t *testing.T) {
	// Lowercase createtime (used in Qzone comments)
	gotComment := eventTimeFrom(map[string]any{"createtime": int64(1786901220)})
	if gotComment.Unix() != 1786901220 {
		t.Fatalf("got %v, want unix 1786901220", gotComment.Unix())
	}

	// String formatted createTime
	gotStr := eventTimeFrom(map[string]any{"createTime": "2024-07-19 01:16"})
	if gotStr.Year() != 2024 || gotStr.Month() != 7 || gotStr.Day() != 19 || gotStr.Hour() != 1 || gotStr.Minute() != 16 {
		t.Fatalf("got %v, want 2024-07-19 01:16", gotStr)
	}

	// Upstream identity abstime fallback
	gotUpstream := eventTimeFrom(map[string]any{
		"_upstream_identity": map[string]any{"abstime": float64(1712330618)},
	})
	if gotUpstream.Unix() != 1712330618 {
		t.Fatalf("got %v, want unix 1712330618", gotUpstream.Unix())
	}
}
