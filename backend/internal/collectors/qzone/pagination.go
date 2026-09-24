package qzone

import (
	"encoding/json"
	"strconv"
	"strings"
)

type pageInfo struct {
	ItemCount     int
	HasMore       bool
	HasMoreKnown  bool
	NextCursor    string
	NextPosition  int
	PositionKnown bool
	Partial       bool
	PartialReason string
}

func parsePageInfo(raw json.RawMessage, currentPosition, pageSize int, itemKeys ...string) pageInfo {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return pageInfo{Partial: true, PartialReason: "invalid pagination JSON: " + err.Error()}
	}
	info := pageInfo{ItemCount: len(findMapSlice(value, itemKeys...))}
	if field, ok := nestedField(value, "has_more", "hasMore", "has_more_feeds"); ok {
		if parsed, valid := boolField(field); valid {
			info.HasMore, info.HasMoreKnown = parsed, true
		}
	}
	if field, ok := nestedField(value, "next_cursor", "cursor", "attach_info"); ok {
		info.NextCursor = strings.TrimSpace(textValue(field))
	}
	if field, ok := nestedField(value, "next_pos", "next_position", "next_start", "offset"); ok {
		if parsed, valid := integerField(field); valid {
			info.NextPosition, info.PositionKnown = parsed, true
		}
	}
	if field, ok := nestedField(value, "availability"); ok {
		availability := strings.ToLower(strings.TrimSpace(textValue(field)))
		if (availability == "available_full" || availability == "not_embedded") && !info.HasMoreKnown {
			info.HasMoreKnown = true
			info.HasMore = false
		} else if strings.Contains(availability, "partial") || strings.Contains(availability, "unavailable") {
			info.Partial = true
			info.PartialReason = "upstream availability is " + availability
		}
	}
	if field, ok := nestedField(value, "partial", "incomplete", "pagination_stalled", "truncated_by_stall"); ok {
		if parsed, valid := boolField(field); valid && parsed {
			info.Partial = true
			if info.PartialReason == "" {
				info.PartialReason = "upstream pagination stalled"
			}
		}
	}
	if info.HasMoreKnown {
		if info.HasMore && info.NextCursor == "" && (!info.PositionKnown || info.NextPosition <= currentPosition) {
			if info.ItemCount > 0 {
				info.NextPosition = currentPosition + info.ItemCount
				info.PositionKnown = true
			} else {
				info.Partial = true
				info.PartialReason = "upstream reports more data without a usable cursor"
			}
		}
		return info
	}

	// Some bridge versions omit has_more for comments. A full page is a safe
	// reason to probe the next offset; a short page stops and is complete.
	if pageSize > 0 && info.ItemCount >= pageSize {
		info.HasMore = true
		info.NextPosition = currentPosition + info.ItemCount
		info.PositionKnown = true
		return info
	}
	info.HasMore = false
	info.Partial = false
	return info
}

func nestedField(value any, keys ...string) (any, bool) {
	wanted := make(map[string]bool, len(keys))
	for _, key := range keys {
		wanted[key] = true
	}
	var visit func(any, int) (any, bool)
	visit = func(current any, depth int) (any, bool) {
		if depth > 5 {
			return nil, false
		}
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		for key := range wanted {
			if field, exists := object[key]; exists && field != nil {
				return field, true
			}
		}
		for _, key := range []string{"data", "result", "_meta", "meta", "_page_info", "page_info"} {
			if nested, exists := object[key]; exists {
				if field, found := visit(nested, depth+1); found {
					return field, true
				}
			}
		}
		return nil, false
	}
	return visit(value, 0)
}

func boolField(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed, err == nil
	case float64:
		return typed != 0, true
	default:
		return false, false
	}
}

func integerField(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		return parsed, err == nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return 0, false
	}
}
