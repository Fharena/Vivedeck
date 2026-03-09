package agent

import (
	"net/http"
)

func (s *HTTPServer) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	response := buildBootstrapResponse(
		s.adapter,
		s.orchestrator,
		s.p2pManager,
		s.config,
		inferAgentBaseURL(r, s.config.PublicAgentBaseURL),
		resolveBootstrapSignalingBaseURL(
			r,
			configuredBootstrapSignalingBaseURL(s.p2pManager),
			s.config.PublicSignalingBaseURL,
		),
	)
	writeJSON(w, http.StatusOK, response)
}
