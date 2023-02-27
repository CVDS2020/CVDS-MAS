package sip

import (
	"fmt"
	"github.com/gofrs/uuid"
	"strings"
)

func createUuid() string {
	return strings.ReplaceAll(uuid.Must(uuid.NewV4()).String(), "-", "")
}

func CreateBranch() string {
	return "z9hG4bK" + createUuid()
}

func CreateTag() string {
	return createUuid()
}

func CreateCallId(host string, port int) string {
	if port > 0 {
		return fmt.Sprintf("%s@%s:%d", createUuid(), host, port)
	}
	return fmt.Sprintf("%s@%s", createUuid(), host)
}

func CreateSipURI(id string, ip string, port int, domain string) string {
	if domain != "" {
		return fmt.Sprintf("sip:%s@%s", id, domain)
	} else if port > 0 {
		return fmt.Sprintf("sip:%s@%s:%d", id, ip, port)
	} else {
		return fmt.Sprintf("sip:%s@%s", id, ip)
	}
}

func CreateDialogId(callId string, fromTag, toTag string) string {
	return callId + ":" + fromTag + ":" + toTag
}
