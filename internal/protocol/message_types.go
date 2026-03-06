package protocol

type MessageType string

const (
    TypePromptSubmit MessageType = "PROMPT_SUBMIT"
    TypePromptAck    MessageType = "PROMPT_ACK"

    TypePatchReady  MessageType = "PATCH_READY"
    TypePatchApply  MessageType = "PATCH_APPLY"
    TypePatchResult MessageType = "PATCH_RESULT"

    TypeRunProfile MessageType = "RUN_PROFILE"
    TypeRunResult  MessageType = "RUN_RESULT"

    TypeOpenLocation MessageType = "OPEN_LOCATION"

    TypeTerm        MessageType = "TERM"
    TypeTermAck     MessageType = "TERM_ACK"
    TypeTermSummary MessageType = "TERM_SUMMARY"

    TypeSignalOffer  MessageType = "SIGNAL_OFFER"
    TypeSignalAnswer MessageType = "SIGNAL_ANSWER"
    TypeSignalICE    MessageType = "SIGNAL_ICE"
    TypeSignalReady  MessageType = "SIGNAL_READY"

    TypeCmdAck MessageType = "CMD_ACK"
)
