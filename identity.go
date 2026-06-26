package wharf

import (
	"crypto/sha256"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// User is a verified, anonymous identity derived from an SSH public key.
//
// The SSH handshake cryptographically proves the client holds the private key,
// so Fingerprint is a stable, verified, zero-signup account id. ShortID and
// Color are deterministic, friendly projections of that id for display.
type User struct {
	Fingerprint string         // SHA256 fingerprint of the public key (the real id)
	ShortID     string         // friendly handle, e.g. "brave-otter"
	Color       lipgloss.Color // deterministic per-user color
}

// userFromKey derives a User from the session's public key. fallback is used
// only when no key is present (which shouldn't happen under public-key auth).
func userFromKey(key ssh.PublicKey, fallback string) User {
	var fp string
	if key != nil {
		fp = gossh.FingerprintSHA256(key)
	} else {
		fp = "anon:" + fallback
	}
	sum := sha256.Sum256([]byte(fp))
	return User{
		Fingerprint: fp,
		ShortID:     adjectives[int(sum[0])%len(adjectives)] + "-" + animals[int(sum[1])%len(animals)],
		Color:       palette[int(sum[2])%len(palette)],
	}
}

// Style returns a lipgloss style coloured with the user's colour, using the
// session's renderer so it respects the client's terminal capabilities.
func (u User) Style(r *lipgloss.Renderer) lipgloss.Style {
	return r.NewStyle().Foreground(u.Color)
}

var adjectives = []string{
	"brave", "quiet", "lucky", "swift", "calm", "bold", "wise", "keen",
	"merry", "noble", "proud", "sly", "warm", "wild", "young", "amber",
	"misty", "rusty", "silent", "sunny", "frosty", "velvet", "crisp", "lone",
	"giddy", "spry", "fuzzy", "humble", "eager", "jolly", "nimble", "fierce",
}

var animals = []string{
	"otter", "finch", "lynx", "heron", "fox", "wren", "moth", "newt",
	"crane", "vole", "shrew", "stoat", "raven", "ibis", "tapir", "gecko",
	"quail", "marten", "badger", "puffin", "civet", "dingo", "egret", "koi",
	"mole", "swan", "hare", "lemur", "skink", "toad", "yak", "owl",
}

// palette is a set of visually distinct ANSI-256 colours (works on basically
// every terminal). One is assigned deterministically per user.
var palette = []lipgloss.Color{
	"39", "208", "46", "201", "51", "220", "129", "196",
	"45", "118", "213", "171", "82", "214", "75", "199",
}
