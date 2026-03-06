package agent

import (
    "encoding/json"
    "errors"
    "fmt"
    "os"
)

type RunProfile struct {
    Label    string `json:"label"`
    Command  string `json:"command"`
    Scope    string `json:"scope"`
    Optional bool   `json:"optional,omitempty"`
}

func DefaultRunProfiles() map[string]RunProfile {
    return map[string]RunProfile{
        "test_last": {
            Label:   "Re-run Last Test",
            Command: "dynamic",
            Scope:   "MEDIUM",
        },
        "test_all": {
            Label:   "Run All Tests",
            Command: "npm test",
            Scope:   "MEDIUM",
        },
        "build": {
            Label:   "Build",
            Command: "npm run build",
            Scope:   "MEDIUM",
        },
        "dev": {
            Label:    "Dev Server",
            Command:  "npm run dev",
            Scope:    "MEDIUM",
            Optional: true,
        },
    }
}

func LoadRunProfiles(path string) (map[string]RunProfile, error) {
    if path == "" {
        return DefaultRunProfiles(), nil
    }

    bytes, err := os.ReadFile(path)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return DefaultRunProfiles(), nil
        }
        return nil, fmt.Errorf("read run profile file: %w", err)
    }

    profiles := make(map[string]RunProfile)
    if err := json.Unmarshal(bytes, &profiles); err != nil {
        return nil, fmt.Errorf("parse run profile json: %w", err)
    }

    if len(profiles) == 0 {
        return nil, errors.New("run profile file is empty")
    }

    return profiles, nil
}
