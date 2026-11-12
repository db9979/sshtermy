package relay

import (
	"context"

	"github.com/db9979/sshtermy/internal/sharewire"
)

// Einladungscode, PAKE und Verschlüsselung stehen in internal/sharewire
// (läuft auch im Browser). Hier nur die Namen, unter denen relay sie kennt.

type (
	Invite    = sharewire.Invite
	PakeKeys  = sharewire.PakeKeys
	GuestPake = sharewire.GuestPake
	Hello     = sharewire.Hello
	ShareInfo = sharewire.ShareInfo
	Ctl       = sharewire.Ctl
	Watch2    = sharewire.Watch2
)

const (
	MsgPakeA    = sharewire.MsgPakeA
	MsgPakeB    = sharewire.MsgPakeB
	MsgHello    = sharewire.MsgHello
	MsgCtl      = sharewire.MsgCtl
	MsgFrame    = sharewire.MsgFrame
	MsgNo       = sharewire.MsgNo
	FramePause  = sharewire.FramePause
	FrameResume = sharewire.FrameResume
	FrameInfo   = sharewire.FrameInfo
	FrameData   = sharewire.FrameData
	FrameSize   = sharewire.FrameSize
	FrameEnd    = sharewire.FrameEnd
)

var (
	NewWords     = sharewire.NewWords
	IsInvite     = sharewire.IsInvite
	ParseInvite  = sharewire.ParseInvite
	StartGuest   = sharewire.StartGuest
	HostPake     = sharewire.HostPake
	SealFrame    = sharewire.SealFrame
	OpenFrame    = sharewire.OpenFrame
	PakeB        = sharewire.PakeB
	SplitPakeB   = sharewire.SplitPakeB
	ErrDenied    = sharewire.ErrDenied
	errWrongCode = sharewire.ErrWrongCode
	randN        = sharewire.RandN
	seal         = sharewire.Seal
	open         = sharewire.Open
)

// Join: anmelden, Code prüfen, auf Zulassung warten (siehe sharewire.Join).
func Join(ctx context.Context, g *Guest2, inv Invite, hello Hello, checked func([3]string)) (*Watch2, error) {
	return sharewire.Join(ctx, g, inv, hello, checked)
}
