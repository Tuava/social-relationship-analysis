package napcat

import "testing"

func TestParseEventsSupportsObjectAndArray(t *testing.T) {
	object := parseEvents([]byte(`{"post_type":"message","message_type":"group","message_id":42}`))
	if len(object) != 1 || object[0].PostType != "message" || object[0].EventID != "42" {
		t.Fatalf("unexpected object event: %#v", object)
	}
	batch := parseEvents([]byte(`[{"post_type":"notice"},{"post_type":"meta_event","time":12}]`))
	if len(batch) != 2 || batch[1].PostType != "meta_event" || batch[1].Time != 12 {
		t.Fatalf("unexpected batch: %#v", batch)
	}
}
