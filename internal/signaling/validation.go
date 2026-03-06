package signaling

import (
    "errors"
    "fmt"

    "github.com/Fharena/Vivedeck/internal/protocol"
)

func validateSignalEnvelope(sessionID string, role Role, env protocol.Envelope) error {
    if env.SID != sessionID {
        return fmt.Errorf("sid mismatch: expected %s", sessionID)
    }

    if err := validateSignalType(role, env.Type); err != nil {
        return err
    }

    switch env.Type {
    case protocol.TypeSignalOffer:
        var payload protocol.SignalOfferPayload
        if err := env.DecodePayload(&payload); err != nil {
            return fmt.Errorf("invalid SIGNAL_OFFER payload: %w", err)
        }
        if payload.SDP == "" {
            return errors.New("offer sdp is required")
        }

    case protocol.TypeSignalAnswer:
        var payload protocol.SignalAnswerPayload
        if err := env.DecodePayload(&payload); err != nil {
            return fmt.Errorf("invalid SIGNAL_ANSWER payload: %w", err)
        }
        if payload.SDP == "" {
            return errors.New("answer sdp is required")
        }

    case protocol.TypeSignalICE:
        var payload protocol.SignalICEPayload
        if err := env.DecodePayload(&payload); err != nil {
            return fmt.Errorf("invalid SIGNAL_ICE payload: %w", err)
        }
        if payload.Candidate == "" {
            return errors.New("ice candidate is required")
        }

    default:
        return fmt.Errorf("unsupported signaling message type: %s", env.Type)
    }

    return nil
}

func validateSignalType(role Role, typ protocol.MessageType) error {
    switch role {
    case RolePC:
        switch typ {
        case protocol.TypeSignalOffer, protocol.TypeSignalICE:
            return nil
        case protocol.TypeSignalAnswer:
            return errors.New("pc cannot send SIGNAL_ANSWER")
        default:
            return fmt.Errorf("pc cannot send message type: %s", typ)
        }

    case RoleMobile:
        switch typ {
        case protocol.TypeSignalAnswer, protocol.TypeSignalICE:
            return nil
        case protocol.TypeSignalOffer:
            return errors.New("mobile cannot send SIGNAL_OFFER")
        default:
            return fmt.Errorf("mobile cannot send message type: %s", typ)
        }

    default:
        return errors.New("invalid role")
    }
}
