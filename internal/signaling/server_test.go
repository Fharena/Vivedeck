package signaling

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
)

func TestServerCreateAndClaimEndpoints(t *testing.T) {
    srv := NewServer(NewStore(2 * time.Minute))
    handler := srv.Handler()

    createReq := httptest.NewRequest(http.MethodPost, "/v1/pairings", nil)
    createRec := httptest.NewRecorder()
    handler.ServeHTTP(createRec, createReq)

    if createRec.Code != http.StatusCreated {
        t.Fatalf("expected 201, got %d", createRec.Code)
    }

    var created struct {
        Code      string `json:"code"`
        SessionID string `json:"sessionId"`
    }
    if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
        t.Fatalf("decode create response: %v", err)
    }

    if created.Code == "" || created.SessionID == "" {
        t.Fatalf("create response fields must be set")
    }

    claimReq := httptest.NewRequest(http.MethodPost, "/v1/pairings/"+created.Code+"/claim", nil)
    claimRec := httptest.NewRecorder()
    handler.ServeHTTP(claimRec, claimReq)

    if claimRec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", claimRec.Code)
    }

    var claimed struct {
        SessionID string `json:"sessionId"`
        MobileKey string `json:"mobileDeviceKey"`
    }
    if err := json.Unmarshal(claimRec.Body.Bytes(), &claimed); err != nil {
        t.Fatalf("decode claim response: %v", err)
    }

    if claimed.SessionID != created.SessionID {
        t.Fatalf("session id mismatch")
    }
    if claimed.MobileKey == "" {
        t.Fatalf("mobile key is required")
    }
}
