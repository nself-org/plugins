package internal

import (
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

func parseAlassConfidence(output string) float64 {
	if m := reConfidence.FindStringSubmatch(output); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return math.Min(1, math.Max(0, v))
		}
	}
	if strings.TrimSpace(output) != "" {
		return 0.7
	}
	return 0
}

func parseAlassOffset(output string) float64 {
	if m := reAlassOffset.FindStringSubmatch(output); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return v
		}
	}
	return 0
}

func parseAlassFramerate(output string) bool {
	return reAlassFramerate.MatchString(output)
}

func parseFfsubsyncConfidence(output string) float64 {
	if m := reFfsubsyncConf.FindStringSubmatch(output); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return math.Min(1, math.Max(0, v))
		}
	}
	if m := reFfsubsyncRatio.FindStringSubmatch(output); m != nil {
		if ratio, err := strconv.ParseFloat(m[1], 64); err == nil {
			return math.Max(0, 1-math.Abs(1-ratio))
		}
	}
	if strings.TrimSpace(output) != "" {
		return 0.7
	}
	return 0
}

func parseFfsubsyncOffset(output string) float64 {
	if m := reFfsubsyncOffMs.FindStringSubmatch(output); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return v
		}
	}
	if m := reFfsubsyncOffSec.FindStringSubmatch(output); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return v * 1000
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// Aggregate confidence
// ---------------------------------------------------------------------------

func computeAggregateConfidence(alass *AlassSyncResult, ffsub *FfsubsyncResult) float64 {
	if alass != nil && ffsub != nil {
		return alass.Confidence*0.4 + ffsub.Confidence*0.6
	}
	if alass != nil {
		return alass.Confidence
	}
	if ffsub != nil {
		return ffsub.Confidence
	}
	return 0
}

// ---------------------------------------------------------------------------
// File copy helper
// ---------------------------------------------------------------------------

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
