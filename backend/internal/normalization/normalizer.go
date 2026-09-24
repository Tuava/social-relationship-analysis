package normalization

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Normalizer struct{ DB *pgxpool.Pool }

func (n Normalizer) UpsertProfile(ctx context.Context, personID, nickname, avatar, card, source string, rawID *string) error {
	return n.UpsertProfileData(ctx, personID, "", nickname, avatar, card, source, rawID, map[string]any{
		"nickname":       nickname,
		"avatar_uri":     avatar,
		"card_or_remark": card,
	})
}

func isQZoneAvatarURL(uri string) bool {
	if uri == "" {
		return false
	}
	lower := strings.ToLower(uri)
	return strings.Contains(lower, "store.qq.com") ||
		strings.Contains(lower, "qpic.cn") ||
		strings.Contains(lower, "qzone") ||
		strings.Contains(lower, "figureurl")
}

func (n Normalizer) UpsertProfileData(ctx context.Context, personID, accountID, nickname, avatar, card, source string, rawID *string, data any) error {
	// 无论何时都不要把空间的头像当作用户头像保存到资料库
	if isQZoneAvatarURL(avatar) {
		avatar = ""
	}

	value, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Extract structured fields from data map
	sex, age, area, signature, regTime, loginDays := "", 0, "", "", int64(0), 0
	if m, ok := data.(map[string]any); ok {
		if v, ok := m["sex"].(string); ok {
			sex = v
		}
		if v, ok := toFloat(m["age"]); ok {
			age = int(v)
		}
		if v, ok := m["area"].(string); ok {
			area = v
		}
		if v, ok := m["long_nick"].(string); ok {
			signature = v
		}
		if v, ok := toFloat(m["reg_time"]); ok {
			regTime = int64(v)
		}
		if v, ok := toFloat(m["login_days"]); ok {
			loginDays = int(v)
		}
	}

	snapshotHash := profileSnapshotHash(nickname, avatar, card, value)

	// A profile version belongs to one source stream. Group-member, QZone and
	// account-profile payloads can share a nickname, but their fields are not
	// interchangeable and must never be merged into one row. The raw response
	// is recorded separately below, so identical retries do not create a new
	// visible version.
	var existingID, existingNickname, existingAvatar, existingCard, existingHash string
	var existingPayload []byte
	err = n.DB.QueryRow(ctx, `SELECT id::text,nickname,avatar_uri,card_or_remark,snapshot_hash,profile_data FROM person_profiles
		WHERE person_id=$1 AND source=$2 AND source_account_id IS NOT DISTINCT FROM NULLIF($3,'')::uuid
		  AND valid_to IS NULL ORDER BY valid_from DESC LIMIT 1`, personID, source, accountID).Scan(&existingID, &existingNickname, &existingAvatar, &existingCard, &existingHash, &existingPayload)
	if err == nil && (existingHash == snapshotHash || profileSnapshotsEquivalent(existingNickname, existingAvatar, existingCard, existingPayload, nickname, avatar, card, value)) {
		_, updateErr := n.DB.Exec(ctx, `UPDATE person_profiles SET
			observation_count=observation_count+1,last_observed_at=now(),raw_record_id=COALESCE($2::uuid,raw_record_id),snapshot_hash=$3
			WHERE id=$1::uuid`, existingID, rawID, snapshotHash)
		if updateErr != nil {
			return updateErr
		}
		return n.recordProfileObservation(ctx, existingID, personID, accountID, source, rawID, snapshotHash, value)
	}
	if err == nil && existingID != "" && nickname == "" && avatar == "" && card == "" {
		// A partial response must enrich the current profile, not erase fields
		// obtained from an earlier response.
		_, err = n.DB.Exec(ctx, `UPDATE person_profiles SET
			profile_data=COALESCE(profile_data,'{}'::jsonb) || $2::jsonb,
			avatar_uri=CASE WHEN $3='' THEN avatar_uri ELSE $3 END,
			card_or_remark=CASE WHEN $4='' THEN card_or_remark ELSE $4 END,
			raw_record_id=COALESCE($5::uuid,raw_record_id),
			sex=COALESCE($6,sex), age=COALESCE($7,age), area=COALESCE($8,area),
			signature=COALESCE($9,signature), reg_time=CASE WHEN $10=0 THEN reg_time ELSE $10 END,
			login_days=CASE WHEN $11=0 THEN login_days ELSE $11 END, source_account_id=NULLIF($12,'')::uuid,
			snapshot_hash=$13,observation_count=observation_count+1,last_observed_at=now()
			WHERE id=$1::uuid`,
			existingID, value, avatar, card, rawID, strOrNil(sex), ageOrNil(age), strOrNil(area), strOrNil(signature), regTime, loginDays, accountID, snapshotHash)
		if err != nil {
			return err
		}
		return n.recordProfileObservation(ctx, existingID, personID, accountID, source, rawID, snapshotHash, value)
	}

	// Close only the previous row in this source stream. Other observations
	// remain independently queryable and keep their own provenance.
	if existingID != "" {
		_, _ = n.DB.Exec(ctx, `UPDATE person_profiles SET valid_to=now() WHERE id=$1::uuid`, existingID)
	}

	// Insert new profile row with structured fields. version_number makes the
	// visible history explicit while profile_observations keeps every retry.
	var profileID string
	err = n.DB.QueryRow(ctx, `INSERT INTO person_profiles(person_id,nickname,avatar_uri,card_or_remark,source,raw_record_id,profile_data,source_account_id,sex,age,area,signature,reg_time,login_days,snapshot_hash,version_number,observation_count,first_observed_at,last_observed_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9,$10,$11,$12,$13,$14,$15,
			COALESCE((SELECT max(version_number)+1 FROM person_profiles WHERE person_id=$1 AND source=$5 AND source_account_id IS NOT DISTINCT FROM NULLIF($8,'')::uuid),1),1,now(),now()) RETURNING id::text`,
		personID, nickname, avatar, card, source, rawID, value, accountID, strOrNil(sex), ageOrNil(age), strOrNil(area), strOrNil(signature), regTime, loginDays, snapshotHash).Scan(&profileID)
	if err != nil {
		return err
	}
	return n.recordProfileObservation(ctx, profileID, personID, accountID, source, rawID, snapshotHash, value)
}

func profileSnapshotHash(nickname, avatar, card string, payload []byte) string {
	value := struct {
		Nickname string          `json:"nickname"`
		Avatar   string          `json:"avatar_uri"`
		Card     string          `json:"card_or_remark"`
		Payload  json.RawMessage `json:"payload"`
	}{nickname, avatar, card, payload}
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func profileSnapshotsEquivalent(existingNickname, existingAvatar, existingCard string, existingPayload []byte, nickname, avatar, card string, payload []byte) bool {
	if existingNickname != nickname || existingAvatar != avatar || existingCard != card {
		return false
	}
	var existingValue, incomingValue any
	if json.Unmarshal(existingPayload, &existingValue) != nil || json.Unmarshal(payload, &incomingValue) != nil {
		return bytes.Equal(existingPayload, payload)
	}
	return reflect.DeepEqual(existingValue, incomingValue)
}

func (n Normalizer) recordProfileObservation(ctx context.Context, profileID, personID, accountID, source string, rawID *string, snapshotHash string, payload []byte) error {
	_, err := n.DB.Exec(ctx, `INSERT INTO profile_observations(profile_id,person_id,source,source_account_id,raw_record_id,snapshot_hash,payload)
		VALUES($1,$2,$3,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,$6,$7)
		ON CONFLICT(profile_id,raw_record_id) DO NOTHING`, profileID, personID, source, accountID, rawID, snapshotHash, payload)
	return err
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case int:
		return float64(n), true
	}
	return 0, false
}

func strOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func ageOrNil(a int) any {
	if a == 0 {
		return nil
	}
	return a
}

type oneBotMessage struct {
	MessageID   any             `json:"message_id"`
	Time        int64           `json:"time"`
	MessageType string          `json:"message_type"`
	PostType    string          `json:"post_type"`
	SelfID      any             `json:"self_id"`
	TargetID    any             `json:"target_id"`
	UserID      any             `json:"user_id"`
	GroupID     any             `json:"group_id"`
	Message     json.RawMessage `json:"message"`
	RawMessage  string          `json:"raw_message"`
	Sender      struct {
		UserID   any    `json:"user_id"`
		Nickname string `json:"nickname"`
		Card     string `json:"card"`
	} `json:"sender"`
}

func messageIDString(value any) string {
	if value == nil {
		return ""
	}
	id := fmt.Sprint(value)
	if id == "0" || id == "<nil>" {
		return ""
	}
	return id
}

// A private conversation belongs to the peer, not necessarily the sender.
// History collectors pass the peer from the request, without changing raw evidence.
func messageIdentity(msg oneBotMessage, peer string) (sender, kind, conversation string) {
	sender = messageIDString(msg.Sender.UserID)
	if sender == "" {
		sender = messageIDString(msg.UserID)
	}
	if group := messageIDString(msg.GroupID); group != "" {
		return sender, "group", group
	}
	if peer != "" {
		return sender, "private", peer
	}
	self := messageIDString(msg.SelfID)
	if self != "" && sender == self {
		if target := messageIDString(msg.TargetID); target != "" {
			return sender, "private", target
		}
		if target := messageIDString(msg.UserID); target != "" && target != self {
			return sender, "private", target
		}
		return sender, "private", "" // No peer evidence: retain the raw record only.
	}
	return sender, "private", sender
}

// ProcessRawMessage turns a OneBot message-shaped payload into durable message,
// person, conversation and relation rows. Unknown payloads remain in raw_records.
func (n Normalizer) ProcessRawMessage(ctx context.Context, accountID, rawID string, payload []byte, privatePeer ...string) error {
	var msg oneBotMessage
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&msg); err != nil {
		return err
	}
	if msg.PostType != "message" && msg.MessageType == "" {
		return nil
	}
	peer := ""
	if len(privatePeer) > 0 {
		peer = privatePeer[0]
	}
	senderID, conversationType, conversationID := messageIdentity(msg, peer)
	if senderID == "" || conversationID == "" {
		return nil
	}
	personID, err := n.upsertPerson(ctx, senderID, msg.Sender.Nickname)
	if err != nil {
		return err
	}
	conversationUUID, err := n.upsertConversation(ctx, conversationType, conversationID)
	if err != nil {
		return err
	}
	sentAt := time.Now()
	if msg.Time > 0 {
		sentAt = time.Unix(msg.Time, 0)
	}
	rawText := msg.RawMessage
	if rawText == "" {
		rawText = messageText(msg.Message)
	}
	var sourceMessageID *string
	if msg.MessageID != nil {
		value := fmt.Sprint(msg.MessageID)
		if value != "" && value != "<nil>" {
			sourceMessageID = &value
		}
	}
	var durableMessageID string
	replyToMessageID := ReplyTarget(msg.Message)
	err = n.DB.QueryRow(ctx, `INSERT INTO messages(source_account_id,source_message_id,conversation_id,sender_id,sent_at,raw_text,message_segments,reply_to_message_id,raw_record_id) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9) ON CONFLICT(source_account_id,source_message_id) DO UPDATE SET conversation_id=EXCLUDED.conversation_id, sender_id=EXCLUDED.sender_id, raw_text=EXCLUDED.raw_text, message_segments=EXCLUDED.message_segments, reply_to_message_id=COALESCE(NULLIF(EXCLUDED.reply_to_message_id,''),messages.reply_to_message_id), raw_record_id=EXCLUDED.raw_record_id RETURNING id`, accountID, sourceMessageID, conversationUUID, personID, sentAt, rawText, msg.Message, replyToMessageID, rawID).Scan(&durableMessageID)
	if err != nil {
		return err
	}
	if err := n.QueueMessageMedia(ctx, accountID, durableMessageID, rawID, msg.Message); err != nil {
		return err
	}
	_, err = n.DB.Exec(ctx, `INSERT INTO relation_events(source_account_id,actor_person_id,target_object_id,action_type,context_type,occurred_at,evidence_ids,raw_record_id) VALUES($1,$2,$3,'sent_message',$4,$5,ARRAY[$6]::uuid[],$7) ON CONFLICT DO NOTHING`, accountID, personID, conversationUUID, conversationType, sentAt, rawID, rawID)
	if err != nil {
		return err
	}

	// Create replied_to events for reply segments
	if replyToMessageID != "" {
		var targetPersonID *string
		_ = n.DB.QueryRow(ctx, `SELECT sender_id::text FROM messages WHERE source_message_id=$1 AND source_account_id=$2`, replyToMessageID, accountID).Scan(&targetPersonID)
		if targetPersonID != nil {
			_, _ = n.DB.Exec(ctx, `INSERT INTO relation_events(source_account_id,actor_person_id,target_person_id,action_type,context_type,occurred_at,evidence_ids,raw_record_id) VALUES($1,$2,$3,'replied_to',$4,$5,ARRAY[$6]::uuid[],$7) ON CONFLICT DO NOTHING`, accountID, personID, *targetPersonID, conversationType, sentAt, rawID, rawID)
		} else {
			// Even if we can't resolve the target person yet, record the reply target as object
			_, _ = n.DB.Exec(ctx, `INSERT INTO relation_events(source_account_id,actor_person_id,target_object_id,action_type,context_type,occurred_at,evidence_ids,raw_record_id) VALUES($1,$2,$3,'replied_to',$4,$5,ARRAY[$6]::uuid[],$7) ON CONFLICT DO NOTHING`, accountID, personID, durableMessageID, conversationType, sentAt, rawID, rawID)
		}
	}

	// Create mentioned events for @ segments
	for _, mentionedQQ := range MentionedQQs(msg.Message) {
		mentionedPersonID, err := n.upsertPerson(ctx, mentionedQQ, "")
		if err != nil {
			continue
		}
		_, _ = n.DB.Exec(ctx, `INSERT INTO relation_events(source_account_id,actor_person_id,target_person_id,target_object_id,action_type,context_type,occurred_at,evidence_ids,raw_record_id) VALUES($1,$2,$3,$4,'mentioned',$5,$6,ARRAY[$7]::uuid[],$8) ON CONFLICT DO NOTHING`, accountID, personID, mentionedPersonID, conversationUUID, conversationType, sentAt, rawID, rawID)
	}

	return nil
}

func (n Normalizer) UpsertPerson(ctx context.Context, userID, nickname string) (string, error) {
	var id string
	err := n.DB.QueryRow(ctx, `SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$1`, userID).Scan(&id)
	if err == nil {
		if nickname != "" {
			_, _ = n.DB.Exec(ctx, `UPDATE persons SET display_name=$2,last_seen_at=now() WHERE id=$1`, id, nickname)
		}
		return id, nil
	}
	if err := n.DB.QueryRow(ctx, `INSERT INTO persons(display_name) VALUES($1) RETURNING id`, nickname).Scan(&id); err != nil {
		return "", err
	}
	if _, err := n.DB.Exec(ctx, `INSERT INTO person_identifiers(person_id,platform,platform_user_id) VALUES($1,'qq',$2) ON CONFLICT(platform,platform_user_id) DO NOTHING`, id, userID); err != nil {
		return "", err
	}
	if err := n.DB.QueryRow(ctx, `SELECT person_id FROM person_identifiers WHERE platform='qq' AND platform_user_id=$1`, userID).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func (n Normalizer) upsertPerson(ctx context.Context, userID, nickname string) (string, error) {
	return n.UpsertPerson(ctx, userID, nickname)
}
func (n Normalizer) upsertConversation(ctx context.Context, kind, id string) (string, error) {
	var uuid string
	err := n.DB.QueryRow(ctx, `INSERT INTO conversations(conversation_type,platform_conversation_id) VALUES($1,$2) ON CONFLICT(conversation_type,platform_conversation_id) DO UPDATE SET name=conversations.name RETURNING id`, kind, id).Scan(&uuid)
	return uuid, err
}
func messageText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var arr []struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if json.Unmarshal(raw, &arr) == nil {
		parts := []string{}
		for _, v := range arr {
			if t, ok := v.Data["text"].(string); ok {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "")
	}
	return string(raw)
}
