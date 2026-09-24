package qzone

import "testing"

func TestParseEventsSupportsBatch(t *testing.T) {
	events := parseEvents([]byte(`[{"post_type":"notice","notice_type":"qzone_like","post_tid":"a","time":10},{"post_type":"message","_tid":"b"}]`))
	if len(events) != 2 || events[0].NoticeType != "qzone_like" || events[1].EventID != "b" {
		t.Fatalf("events = %#v", events)
	}
}
