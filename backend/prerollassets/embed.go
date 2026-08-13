package prerollassets

import _ "embed"

// DefaultVideo is the server-provided preroll available to every profile and
// client. It is intentionally embedded so Docker and standalone builds behave
// identically and never depend on a host filesystem path.
//
//go:embed upscaled-video.mp4
var DefaultVideo []byte
