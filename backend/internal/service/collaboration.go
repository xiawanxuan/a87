package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"ultrasound-annotation/internal/models"
)

type CollabSession struct {
	UserID      string       `json:"userId"`
	UserName    string       `json:"userName"`
	CursorPos   *models.Point `json:"cursorPos,omitempty"`
	ActiveTool  string       `json:"activeTool,omitempty"`
	JoinedAt    time.Time    `json:"joinedAt"`
	LastBeat    time.Time    `json:"lastBeat"`
}

type CollaborationService interface {
	JoinImage(ctx context.Context, imageID uint64, sess CollabSession) error
	LeaveImage(ctx context.Context, imageID uint64, userID string) error
	Heartbeat(ctx context.Context, imageID uint64, sess CollabSession) error
	ListUsers(ctx context.Context, imageID uint64) ([]CollabSession, error)

	SetDraft(ctx context.Context, imageID uint64, userID string, data []byte) error
	GetDraft(ctx context.Context, imageID uint64, userID string) ([]byte, error)

	PublishOp(ctx context.Context, imageID uint64, op OperationMessage) error
	SubscribeOps(ctx context.Context, imageID uint64) <-chan OperationMessage

	LockAnnotation(ctx context.Context, imageID uint64, annotationID uint64, userID string, ttl time.Duration) (bool, error)
	UnlockAnnotation(ctx context.Context, imageID uint64, annotationID uint64, userID string) error
}

type OperationMessage struct {
	Type       string          `json:"type"`
	UserID     string          `json:"userId"`
	UserName   string          `json:"userName,omitempty"`
	Timestamp  int64           `json:"ts"`
	Payload    json.RawMessage `json:"payload"`
	Sequence   int64           `json:"seq,omitempty"`
}

const (
	OpTypeAdd        = "add"
	OpTypeUpdate     = "update"
	OpTypeDelete     = "delete"
	OpTypeBulkReplace = "bulk_replace"
	OpTypeCursor     = "cursor"
	OpTypeSelection  = "selection"
	OpTypeRollback   = "rollback"
	OpTypeLock       = "lock"
	OpTypeUnlock     = "unlock"
)

type redisCollab struct {
	rdb *redis.Client
}

func NewCollaborationService(rdb *redis.Client) CollaborationService {
	return &redisCollab{rdb: rdb}
}

func sessKey(imageID uint64) string       { return fmt.Sprintf("collab:image:%d:sessions", imageID) }
func lockKey(imageID, annID uint64) string { return fmt.Sprintf("collab:image:%d:lock:%d", imageID, annID) }
func draftKey(imageID uint64, uid string) string {
	return fmt.Sprintf("draft:image:%d:user:%s", imageID, uid)
}
func opChannel(imageID uint64) string { return fmt.Sprintf("op:image:%d", imageID) }

func (r *redisCollab) JoinImage(ctx context.Context, imageID uint64, sess CollabSession) error {
	sess.JoinedAt = time.Now()
	sess.LastBeat = time.Now()
	data, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	if err := r.rdb.HSet(ctx, sessKey(imageID), sess.UserID, data).Err(); err != nil {
		return err
	}
	return r.rdb.Expire(ctx, sessKey(imageID), 24*time.Hour).Err()
}

func (r *redisCollab) LeaveImage(ctx context.Context, imageID uint64, userID string) error {
	return r.rdb.HDel(ctx, sessKey(imageID), userID).Err()
}

func (r *redisCollab) Heartbeat(ctx context.Context, imageID uint64, sess CollabSession) error {
	sess.LastBeat = time.Now()
	data, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	return r.rdb.HSet(ctx, sessKey(imageID), sess.UserID, data).Err()
}

func (r *redisCollab) ListUsers(ctx context.Context, imageID uint64) ([]CollabSession, error) {
	raw, err := r.rdb.HGetAll(ctx, sessKey(imageID)).Result()
	if err != nil {
		return nil, err
	}
	list := make([]CollabSession, 0, len(raw))
	now := time.Now()
	for _, v := range raw {
		var s CollabSession
		if err := json.Unmarshal([]byte(v), &s); err != nil {
			continue
		}
		if now.Sub(s.LastBeat) > 5*time.Minute {
			continue
		}
		list = append(list, s)
	}
	return list, nil
}

func (r *redisCollab) SetDraft(ctx context.Context, imageID uint64, userID string, data []byte) error {
	return r.rdb.Set(ctx, draftKey(imageID, userID), data, 7*24*time.Hour).Err()
}

func (r *redisCollab) GetDraft(ctx context.Context, imageID uint64, userID string) ([]byte, error) {
	b, err := r.rdb.Get(ctx, draftKey(imageID, userID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	return b, err
}

func (r *redisCollab) PublishOp(ctx context.Context, imageID uint64, op OperationMessage) error {
	op.Timestamp = time.Now().UnixMilli()
	data, err := json.Marshal(op)
	if err != nil {
		return err
	}
	return r.rdb.Publish(ctx, opChannel(imageID), data).Err()
}

func (r *redisCollab) SubscribeOps(ctx context.Context, imageID uint64) <-chan OperationMessage {
	ch := make(chan OperationMessage, 256)
	pubsub := r.rdb.Subscribe(ctx, opChannel(imageID))
	go func() {
		defer func() {
			_ = pubsub.Close()
			close(ch)
		}()
		msgCh := pubsub.Channel()
		for msg := range msgCh {
			var op OperationMessage
			if err := json.Unmarshal([]byte(msg.Payload), &op); err != nil {
				continue
			}
			select {
			case ch <- op:
			default:
			}
		}
	}()
	return ch
}

func (r *redisCollab) LockAnnotation(ctx context.Context, imageID uint64, annotationID uint64, userID string, ttl time.Duration) (bool, error) {
	key := lockKey(imageID, annotationID)
	ok, err := r.rdb.SetNX(ctx, key, userID, ttl).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (r *redisCollab) UnlockAnnotation(ctx context.Context, imageID uint64, annotationID uint64, userID string) error {
	key := lockKey(imageID, annotationID)
	owner, err := r.rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil
		}
		return err
	}
	if owner == userID {
		return r.rdb.Del(ctx, key).Err()
	}
	return nil
}
