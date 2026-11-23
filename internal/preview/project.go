package preview

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const ProjectProofHeader = "X-ForgeOS-Project-Preview-Proof"
const ProjectHealthPath = "/.forgeos-preview/health"

func ProjectProof(secret, instance, direction string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(direction + "\x00" + instance))
	return hex.EncodeToString(mac.Sum(nil))
}
