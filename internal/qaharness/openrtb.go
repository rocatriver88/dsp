package qaharness

import (
	"fmt"
	"time"

	"github.com/prebid/openrtb/v20/openrtb2"
)

// BuildBidRequestOpts parameterizes BuildBidRequest.
type BuildBidRequestOpts struct {
	ID       string
	Geo      string
	OS       string
	IFA      string
	Secure   bool
	BidFloor float64 // in imp-level CNY
	Format   string  // "banner" (default), "video", "native"
	W, H     int64
}

// BuildBidRequest constructs a minimal OpenRTB 2.5 BidRequest suitable for
// hitting Engine.Bid. Defaults target CN + iOS banner 320x50 + secure=0.
func BuildBidRequest(opts BuildBidRequestOpts) *openrtb2.BidRequest {
	if opts.ID == "" {
		opts.ID = fmt.Sprintf("qa-req-%d", time.Now().UnixNano())
	}
	if opts.Geo == "" {
		opts.Geo = "CN"
	}
	if opts.OS == "" {
		opts.OS = "iOS"
	}
	if opts.IFA == "" {
		opts.IFA = "qa-ifa-" + opts.ID
	}
	if opts.Format == "" {
		opts.Format = "banner"
	}
	if opts.W == 0 {
		opts.W = 320
	}
	if opts.H == 0 {
		opts.H = 50
	}

	var secureVal int8
	if opts.Secure {
		secureVal = 1
	}

	imp := openrtb2.Imp{
		ID:       "imp-1",
		BidFloor: opts.BidFloor,
		Secure:   &secureVal,
	}
	switch opts.Format {
	case "video":
		imp.Video = &openrtb2.Video{W: &opts.W, H: &opts.H}
	case "native":
		imp.Native = &openrtb2.Native{Request: `{"ver":"1.2"}`}
	default:
		imp.Banner = &openrtb2.Banner{W: &opts.W, H: &opts.H}
	}

	return &openrtb2.BidRequest{
		ID:  opts.ID,
		Imp: []openrtb2.Imp{imp},
		Device: &openrtb2.Device{
			OS:  opts.OS,
			IFA: opts.IFA,
			Geo: &openrtb2.Geo{Country: opts.Geo},
			IP:  "203.0.113.1",
			UA:  "qa-test-agent/1.0",
		},
	}
}
