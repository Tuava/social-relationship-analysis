package qzone

import (
	"encoding/json"
	"testing"
)

func TestParsePageInfoKeepsCursorAndOffsetIndependent(t *testing.T) {
	cursorPage := parsePageInfo(json.RawMessage(`{
		"msglist":[{"tid":"1"}],"has_more":true,"next_cursor":"opaque-token"
	}`), 0, 100, "msglist")
	if !cursorPage.HasMore || cursorPage.NextCursor != "opaque-token" || cursorPage.PositionKnown {
		t.Fatalf("cursor page = %#v", cursorPage)
	}

	offsetPage := parsePageInfo(json.RawMessage(`{
		"data":{"comments":[{"id":"1"}],"has_more":true,"next_pos":21}
	}`), 20, 20, "comments")
	if !offsetPage.HasMore || !offsetPage.PositionKnown || offsetPage.NextPosition != 21 || offsetPage.NextCursor != "" {
		t.Fatalf("offset page = %#v", offsetPage)
	}
}

func TestParsePageInfoShortPageIsComplete(t *testing.T) {
	page := parsePageInfo(json.RawMessage(`{"commentlist":[{"id":"1"}]}`), 0, 20, "commentlist")
	if page.HasMore || page.Partial {
		t.Fatalf("page = %#v", page)
	}
}

func TestParsePageInfoContinuesFullPageWithoutMetadata(t *testing.T) {
	page := parsePageInfo(json.RawMessage(`{"commentlist":[{"id":"1"},{"id":"2"}]}`), 4, 2, "commentlist")
	if !page.HasMore || !page.PositionKnown || page.NextPosition != 6 {
		t.Fatalf("page = %#v", page)
	}
}

func TestParsePageInfoPreservesUpstreamPartialAvailability(t *testing.T) {
	page := parsePageInfo(json.RawMessage(`{
		"commentlist":[],"has_more":false,"availability":"available_partial"
	}`), 0, 20, "commentlist")
	if !page.Partial || page.PartialReason != "upstream availability is available_partial" {
		t.Fatalf("page = %#v", page)
	}
}

func TestParsePageInfoAcceptsExplicitFullAvailability(t *testing.T) {
	page := parsePageInfo(json.RawMessage(`{
		"commentlist":[{"id":"1"}],"availability":"available_full"
	}`), 0, 20, "commentlist")
	if page.HasMore || !page.HasMoreKnown || page.Partial {
		t.Fatalf("page = %#v", page)
	}
}

func TestParsePageInfoMarksStalledPaginationPartial(t *testing.T) {
	page := parsePageInfo(json.RawMessage(`{"msglist":[{"tid":"1"}],"has_more":false,"_page_info":{"pagination_stalled":true}}`), 0, 20, "msglist")
	if !page.Partial || page.PartialReason != "upstream pagination stalled" {
		t.Fatalf("page = %#v", page)
	}
}
