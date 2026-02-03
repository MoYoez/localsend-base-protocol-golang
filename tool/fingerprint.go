package tool

func CheckFingerPrintIsSame(fromFingerprint string) bool {
	return fromFingerprint != "" && fromFingerprint == CurrentConfig.Fingerprint
}
