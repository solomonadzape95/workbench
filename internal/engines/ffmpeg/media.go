package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/solomonadzape95/workbench/internal/core"
)

// Media is the subset of ffprobe metadata Workbench needs to validate inputs
// and compute progress.
type Media struct {
	Duration   time.Duration
	HasAudio   bool
	HasVideo   bool
	VideoCodec string
	AudioCodec string
	Width      int
	Height     int
	SampleRate int
	BitRate    int64
}

type probeJSON struct {
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		CodecType  string `json:"codec_type"`
		CodecName  string `json:"codec_name"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		SampleRate string `json:"sample_rate"`
	} `json:"streams"`
}

// Probe reads format and stream information with ffprobe.
func (c *Client) Probe(parent context.Context, path string) (Media, error) {
	binary, err := c.FFprobeBinary()
	if err != nil {
		return Media{}, err
	}

	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return Media{}, ctx.Err()
		}
		return Media{}, core.Wrap(core.CodeInput, "", path, err,
			"the file may be corrupt or not a media file")
	}

	var raw probeJSON
	if err := json.Unmarshal(out, &raw); err != nil {
		return Media{}, core.Wrap(core.CodeInput, "", path, err,
			"ffprobe returned unexpected output")
	}

	media := Media{}
	if seconds, err := strconv.ParseFloat(raw.Format.Duration, 64); err == nil && seconds > 0 {
		media.Duration = time.Duration(seconds * float64(time.Second))
	}
	if rate, err := strconv.ParseInt(raw.Format.BitRate, 10, 64); err == nil {
		media.BitRate = rate
	}

	for _, stream := range raw.Streams {
		switch stream.CodecType {
		case "audio":
			if !media.HasAudio {
				media.HasAudio = true
				media.AudioCodec = stream.CodecName
				media.SampleRate, _ = strconv.Atoi(stream.SampleRate)
			}
		case "video":
			// Cover art is exposed as a video stream with no dimensions worth
			// treating as a real video track.
			if stream.Width == 0 || stream.Height == 0 {
				continue
			}
			if !media.HasVideo {
				media.HasVideo = true
				media.VideoCodec = stream.CodecName
				media.Width = stream.Width
				media.Height = stream.Height
			}
		}
	}

	if !media.HasAudio && !media.HasVideo {
		return Media{}, core.NewError(core.CodeInput, "", path,
			"no audio or video streams found", "check that this is a media file")
	}
	if media.Duration == 0 {
		media.Duration = 0 // unknown; progress falls back to indeterminate
	}
	return media, nil
}

func (m Media) Summary() string {
	kind := "media"
	if m.HasVideo {
		kind = fmt.Sprintf("%s %dx%d", m.VideoCodec, m.Width, m.Height)
	} else if m.HasAudio {
		kind = m.AudioCodec
	}
	return fmt.Sprintf("%s · %s", kind, m.Duration.Round(time.Millisecond))
}
