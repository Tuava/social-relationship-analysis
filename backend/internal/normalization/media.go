package normalization

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type MediaReference struct {
	SegmentIndex int
	SegmentType  string
	Kind         string
	SourceRef    string
	SourceURL    string
	Resolver     string
	Filename     string
	Metadata     json.RawMessage
}

func (n Normalizer) QueueMessageMedia(ctx context.Context, accountID, messageID, rawID string, segments json.RawMessage) error {
	for _, ref := range ExtractMediaReferences(segments) {
		var referenceID string
		err := n.DB.QueryRow(ctx, `INSERT INTO media_references(
			source_account_id,message_id,raw_record_id,segment_index,segment_type,media_kind,
			source_ref,source_url,resolver_endpoint,original_filename,metadata
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT(message_id,segment_index,segment_type) WHERE message_id IS NOT NULL
		DO UPDATE SET source_ref=EXCLUDED.source_ref,source_url=EXCLUDED.source_url,
			resolver_endpoint=EXCLUDED.resolver_endpoint,original_filename=EXCLUDED.original_filename,metadata=EXCLUDED.metadata,
			status=CASE WHEN media_references.status='completed' THEN 'completed' ELSE 'pending' END
		RETURNING id::text`, accountID, messageID, rawID, ref.SegmentIndex, ref.SegmentType, ref.Kind,
			ref.SourceRef, ref.SourceURL, ref.Resolver, ref.Filename, ref.Metadata).Scan(&referenceID)
		if err != nil {
			return err
		}
		_, err = n.DB.Exec(ctx, `INSERT INTO message_media(message_id,media_reference_id,segment_index,segment_type,metadata)
			VALUES($1,$2,$3,$4,$5) ON CONFLICT(message_id,media_reference_id) DO UPDATE SET metadata=EXCLUDED.metadata`,
			messageID, referenceID, ref.SegmentIndex, ref.SegmentType, ref.Metadata)
		if err != nil {
			return err
		}
	}
	return nil
}

func (n Normalizer) QueueAvatar(ctx context.Context, accountID, personID, rawID, qq, avatarURL string) error {
	// 无论何时都不要把空间的头像 (store.qq.com/qzone, qpic.cn 等) 作为用户头像！
	if isQZoneAvatarURL(avatarURL) {
		avatarURL = ""
	}
	canonicalURL := ""
	if qq != "" {
		canonicalURL = "https://q1.qlogo.cn/g?b=qq&nk=" + qq + "&s=640"
	}
	urls := make([]string, 0, 1)
	if canonicalURL != "" {
		urls = append(urls, canonicalURL)
	} else if avatarURL != "" {
		urls = append(urls, avatarURL)
	}
	if len(urls) == 0 {
		return nil
	}
	for _, sourceURL := range urls {
		_, err := n.DB.Exec(ctx, `INSERT INTO media_references(source_account_id,person_id,raw_record_id,segment_type,media_kind,source_url,metadata)
		VALUES($1,$2,NULLIF($3::text,'')::uuid,'avatar','avatar',$4,jsonb_build_object('qq',$5::text,'raw_record_id',$3::text))
		ON CONFLICT(source_account_id,person_id,source_url) WHERE person_id IS NOT NULL AND media_kind='avatar'
		DO UPDATE SET raw_record_id=EXCLUDED.raw_record_id,
            status=CASE WHEN media_references.status='completed' THEN 'completed' ELSE 'pending' END,
            attempt_count=CASE WHEN media_references.status='completed' THEN media_references.attempt_count ELSE 0 END,
            last_error=CASE WHEN media_references.status='completed' THEN media_references.last_error ELSE NULL END,
			next_attempt_at=CASE WHEN media_references.status='completed' THEN media_references.next_attempt_at ELSE now() END`, accountID, personID, rawID, sourceURL, qq)
		if err != nil {
			return err
		}
	}
	return nil
}

func (n Normalizer) QueueGroupAvatar(ctx context.Context, accountID, groupID, rawID, platformGroupID, avatarURL string) error {
	canonicalURL := ""
	if platformGroupID != "" {
		canonicalURL = "https://p.qlogo.cn/gh/" + platformGroupID + "/" + platformGroupID + "/640/"
	}
	urls := make([]string, 0, 2)
	if avatarURL != "" {
		urls = append(urls, avatarURL)
	}
	if canonicalURL != "" && canonicalURL != avatarURL {
		urls = append(urls, canonicalURL)
	}
	for _, sourceURL := range urls {
		_, err := n.DB.Exec(ctx, `INSERT INTO media_references(
			source_account_id,group_id,raw_record_id,segment_type,media_kind,source_url,metadata,relation_depth,priority_reason
		) VALUES($1,$2,NULLIF($3::text,'')::uuid,'group_avatar','avatar',$4,
			jsonb_build_object('group_id',$5::text,'raw_record_id',$3::text),0,'avatar')
		ON CONFLICT(source_account_id,group_id,source_url) WHERE group_id IS NOT NULL AND media_kind='avatar'
		DO UPDATE SET raw_record_id=EXCLUDED.raw_record_id,
			status=CASE WHEN media_references.status='completed' THEN 'completed' ELSE 'pending' END,
			attempt_count=CASE WHEN media_references.status='completed' THEN media_references.attempt_count ELSE 0 END,
			last_error=CASE WHEN media_references.status='completed' THEN media_references.last_error ELSE NULL END,
			next_attempt_at=CASE WHEN media_references.status='completed' THEN media_references.next_attempt_at ELSE now() END`,
			accountID, groupID, rawID, sourceURL, platformGroupID)
		if err != nil {
			return err
		}
	}
	return nil
}

func ExtractMediaReferences(raw json.RawMessage) []MediaReference {
	var segments []struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if json.Unmarshal(raw, &segments) != nil {
		return nil
	}
	result := make([]MediaReference, 0)
	for index, segment := range segments {
		kind, resolver := mediaKind(segment.Type, segment.Data)
		if kind == "" {
			continue
		}
		urlValue := firstString(segment.Data, "url", "src")
		fileValue := firstString(segment.Data, "file_id", "file", "path")
		if strings.HasPrefix(fileValue, "http://") || strings.HasPrefix(fileValue, "https://") {
			if urlValue == "" {
				urlValue = fileValue
			}
			fileValue = ""
		}
		if urlValue == "" && fileValue == "" {
			continue
		}
		metadata, _ := json.Marshal(segment.Data)
		filenameKeys := []string{"file_name", "filename", "name"}
		if strings.EqualFold(segment.Type, "file") {
			filenameKeys = append(filenameKeys, "file")
		}
		result = append(result, MediaReference{SegmentIndex: index, SegmentType: segment.Type, Kind: kind,
			SourceRef: fileValue, SourceURL: urlValue, Resolver: resolver,
			Filename: firstString(segment.Data, filenameKeys...), Metadata: metadata})
	}
	return result
}

func mediaKind(segmentType string, data map[string]any) (string, string) {
	switch strings.ToLower(segmentType) {
	case "image":
		if isStickerImage(data) {
			return "sticker", "/get_image"
		}
		return "image", "/get_image"
	case "mface":
		return "sticker", "/get_image"
	case "record":
		return "audio", "/get_record"
	case "video":
		return "video", "/get_file"
	case "file":
		return "file", "/get_file"
	default:
		return "", ""
	}
}

func isStickerImage(data map[string]any) bool {
	urlValue := strings.ToLower(firstString(data, "url", "src"))
	if strings.Contains(urlValue, "gxh.vip.qq.com") {
		return true
	}
	subType := firstString(data, "sub_type")
	if subType != "" && subType != "0" {
		return true
	}
	summary := strings.ToLower(firstString(data, "summary"))
	return strings.Contains(summary, "动画表情") || strings.Contains(summary, "自定义表情")
}

func firstString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key]; ok && value != nil {
			text := fmt.Sprint(value)
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}
