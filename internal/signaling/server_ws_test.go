package signaling

import (
    "net/http/httptest"
    "strings"
    "testing"
    "time"

    "github.com/Fharena/Vivedeck/internal/protocol"
    "github.com/gorilla/websocket"
)

func TestSignalingQueuesOfferUntilPeerConnects(t *testing.T) {
    store := NewStore(2 * time.Minute)
    pairing, err := store.CreatePairing()
    if err != nil {
        t.Fatalf("create pairing: %v", err)
    }

    claimed, err := store.ClaimPairing(pairing.Code)
    if err != nil {
        t.Fatalf("claim pairing: %v", err)
    }

    server := NewServer(store)
    ts := httptest.NewServer(server.Handler())
    defer ts.Close()

    pcConn, err := dialSessionWS(ts.URL, pairing.SessionID, pairing.PCDeviceKey, RolePC)
    if err != nil {
        t.Fatalf("dial pc ws: %v", err)
    }
    defer pcConn.Close()

    offer, _ := protocol.NewEnvelope(pairing.SessionID, "rid-offer", 1, protocol.TypeSignalOffer, protocol.SignalOfferPayload{SDP: "v=0"})
    if err := pcConn.WriteJSON(offer); err != nil {
        t.Fatalf("send offer: %v", err)
    }

    ack := readEnvelope(t, pcConn, 2*time.Second)
    if ack.Type != protocol.TypeCmdAck {
        t.Fatalf("expected cmd ack, got %s", ack.Type)
    }

    var ackPayload protocol.CmdAckPayload
    if err := ack.DecodePayload(&ackPayload); err != nil {
        t.Fatalf("decode ack payload: %v", err)
    }
    if !ackPayload.Accepted {
        t.Fatalf("offer should be accepted")
    }

    mobileConn, err := dialSessionWS(ts.URL, pairing.SessionID, claimed.MobileDeviceKey, RoleMobile)
    if err != nil {
        t.Fatalf("dial mobile ws: %v", err)
    }
    defer mobileConn.Close()

    // Mobile should receive queued offer after connecting.
    foundOffer := false
    for i := 0; i < 3; i++ {
        env := readEnvelope(t, mobileConn, 2*time.Second)
        if env.Type == protocol.TypeSignalOffer {
            foundOffer = true
            var offerPayload protocol.SignalOfferPayload
            if err := env.DecodePayload(&offerPayload); err != nil {
                t.Fatalf("decode offer payload: %v", err)
            }
            if offerPayload.SDP == "" {
                t.Fatalf("offer sdp should not be empty")
            }
            break
        }
    }

    if !foundOffer {
        t.Fatalf("mobile did not receive queued offer")
    }
}

func TestSignalingRejectsMobileOffer(t *testing.T) {
    store := NewStore(2 * time.Minute)
    pairing, err := store.CreatePairing()
    if err != nil {
        t.Fatalf("create pairing: %v", err)
    }

    claimed, err := store.ClaimPairing(pairing.Code)
    if err != nil {
        t.Fatalf("claim pairing: %v", err)
    }

    server := NewServer(store)
    ts := httptest.NewServer(server.Handler())
    defer ts.Close()

    mobileConn, err := dialSessionWS(ts.URL, pairing.SessionID, claimed.MobileDeviceKey, RoleMobile)
    if err != nil {
        t.Fatalf("dial mobile ws: %v", err)
    }
    defer mobileConn.Close()

    offer, _ := protocol.NewEnvelope(pairing.SessionID, "rid-mobile-offer", 1, protocol.TypeSignalOffer, protocol.SignalOfferPayload{SDP: "v=0"})
    if err := mobileConn.WriteJSON(offer); err != nil {
        t.Fatalf("send invalid offer: %v", err)
    }

    ack := readEnvelope(t, mobileConn, 2*time.Second)
    if ack.Type != protocol.TypeCmdAck {
        t.Fatalf("expected cmd ack for invalid offer, got %s", ack.Type)
    }

    var ackPayload protocol.CmdAckPayload
    if err := ack.DecodePayload(&ackPayload); err != nil {
        t.Fatalf("decode ack payload: %v", err)
    }
    if ackPayload.Accepted {
        t.Fatalf("mobile offer must be rejected")
    }
}

func dialSessionWS(baseURL, sessionID, key string, role Role) (*websocket.Conn, error) {
    wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/v1/sessions/" + sessionID + "/ws?deviceKey=" + key + "&role=" + string(role)
    conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    return conn, err
}

func readEnvelope(t *testing.T, conn *websocket.Conn, timeout time.Duration) protocol.Envelope {
    t.Helper()

    _ = conn.SetReadDeadline(time.Now().Add(timeout))

    var env protocol.Envelope
    if err := conn.ReadJSON(&env); err != nil {
        t.Fatalf("read envelope: %v", err)
    }
    return env
}
