package asterisk

import (
	"errors"
	"fmt"
	"os/exec"
)

// ConvertToAsteriskWAV transcodes an uploaded audio file (whatever format a
// browser or admin happened to upload — wav/gsm/mp3/ulaw) into the format
// Asterisk's Background()/Playback()/Queue() hold-music apps expect for
// reliable playback: 8kHz, mono, G.711 u-law WAV — the same codec every
// PJSIP endpoint/provider in this system is already configured to use (see
// CreateSIPAccount, writeProviderPJSIP: allow=ulaw,alaw), so no separate
// transcoding step is needed once the file reaches Asterisk.
//
// Requires the `ffmpeg` binary on this backend's host — not related to
// whatever the Asterisk box itself has installed, since conversion happens
// here at upload time, not on the Asterisk server.
func ConvertToAsteriskWAV(srcPath, dstPath string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return errors.New("ffmpeg is not installed on this server — required to convert uploaded audio to Asterisk's format")
	}
	cmd := exec.Command("ffmpeg",
		"-y", // overwrite dstPath if it already exists (re-uploads)
		"-i", srcPath,
		"-ar", "8000", // 8kHz sample rate — standard telephony
		"-ac", "1", // mono
		"-acodec", "pcm_mulaw", // G.711 u-law, 8 bits/sample
		dstPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg conversion failed: %w (%s)", err, out)
	}
	return nil
}
