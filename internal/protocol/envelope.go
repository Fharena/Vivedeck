package protocol

import (
    "encoding/json"
    "errors"
    "fmt"
    "time"
)

// Envelope is the transport-neutral command envelope for all control path messages.
type Envelope struct {
    SID     string          `json:"sid"`
    RID     string          `json:"rid"`
    Seq     int64           `json:"seq"`
    TS      int64           `json:"ts"`
    Type    MessageType     `json:"type"`
    Payload json.RawMessage `json:"payload"`
}

func NewEnvelope(sid, rid string, seq int64, typ MessageType, payload any) (Envelope, error) {
    bytes, err := json.Marshal(payload)
    if err != nil {
        return Envelope{}, fmt.Errorf("marshal payload: %w", err)
    }

    e := Envelope{
        SID:     sid,
        RID:     rid,
        Seq:     seq,
        TS:      time.Now().UnixMilli(),
        Type:    typ,
        Payload: bytes,
    }

    if err := e.Validate(); err != nil {
        return Envelope{}, err
    }

    return e, nil
}

func (e Envelope) Validate() error {
    if e.SID == "" {
        return errors.New("sid is required")
    }
    if e.RID == "" {
        return errors.New("rid is required")
    }
    if e.Seq < 0 {
        return errors.New("seq must be >= 0")
    }
    if e.TS <= 0 {
        return errors.New("ts must be > 0")
    }
    if e.Type == "" {
        return errors.New("type is required")
    }
    if len(e.Payload) == 0 {
        return errors.New("payload is required")
    }

    return nil
}

func (e Envelope) DecodePayload(target any) error {
    if len(e.Payload) == 0 {
        return errors.New("empty payload")
    }

    if err := json.Unmarshal(e.Payload, target); err != nil {
        return fmt.Errorf("decode payload: %w", err)
    }

    return nil
}
